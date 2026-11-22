package opendata_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/csvout"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/testdb"
)

var taipei = time.FixedZone("Asia/Taipei", 8*60*60)

func readGz(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	sc := bufio.NewScanner(gz)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func readIndex(t *testing.T, dir string) opendata.Index {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "aqi_obs", opendata.IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	var idx opendata.Index
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestExportMonthlyFiles(t *testing.T) {
	db := testdb.New(t)
	testdb.RegisterDatasets(t, db)
	if _, err := db.Exec(`
		INSERT INTO ref.aqi_site (site_id, name, county_code) VALUES (1, '=中山', '63000');
		INSERT INTO data.aqi_obs_history (site_id, observed_at, aqi) VALUES
		  (1, '2026-08-15 12:00+08', 40),
		  (1, '2026-09-01 00:00+08', 41),
		  (1, '2026-09-30 23:30+08', 42),
		  (1, '2026-10-01 00:10+08', 43)`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	e := &opendata.Exporter{
		Service: &opendata.Service{DB: db}, Dir: dir,
		Now:    func() time.Time { return time.Date(2026, 10, 22, 3, 30, 0, 0, taipei) },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx := context.Background()

	res, err := e.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 2 {
		t.Fatalf("written %d files, want August and September", res.Written)
	}
	if _, err := os.Stat(filepath.Join(dir, "aqi_obs", "2026-10.csv.gz")); err == nil {
		t.Error("the unfinished month was exported")
	}
	sep := readGz(t, filepath.Join(dir, "aqi_obs", "2026-09.csv.gz"))
	if len(sep) != 3 || !strings.HasPrefix(sep[0], csvout.BOM+"site_id,observed_at,aqi,") {
		t.Fatalf("September file = %q", sep)
	}
	if !strings.HasPrefix(sep[1], "1,2026-09-01T00:00:00+08:00,41,") || !strings.HasPrefix(sep[2], "1,2026-09-30T23:30:00+08:00,42,") {
		t.Errorf("September rows = %q", sep[1:])
	}

	idx := readIndex(t, dir)
	if idx.Dataset != "aqi_obs" || len(idx.Files) != 2 || idx.Files[0].Month != "2026-08" || idx.Files[1].Rows != 2 ||
		len(idx.Fields) == 0 || idx.License == "" {
		t.Fatalf("index = %+v", idx)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "aqi_obs", "2026-09.csv.gz"))
	sum := sha256.Sum256(b)
	if idx.Files[1].SHA256 != hex.EncodeToString(sum[:]) || idx.Files[1].Bytes != int64(len(b)) {
		t.Errorf("index sha256/bytes do not match the file")
	}

	if res, err := e.Run(ctx); err != nil || res.Written != 0 || res.Current != 2 {
		t.Errorf("second run: %+v %v", res, err)
	}

	if _, err := db.Exec(`INSERT INTO data.aqi_obs_history (site_id, observed_at, aqi) VALUES (1, '2026-09-10 08:00+08', 50)`); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "aqi_obs", "2026-08.csv.gz")); err != nil {
		t.Fatal(err)
	}
	if res, err := e.Run(ctx); err != nil || res.Written != 2 {
		t.Errorf("after late data and a lost file: %+v %v", res, err)
	}
	if idx := readIndex(t, dir); idx.Files[1].Rows != 3 {
		t.Errorf("September rows in index = %d, want 3", idx.Files[1].Rows)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "aqi_obs"))
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), ".") {
			t.Errorf("temporary file left behind: %s", en.Name())
		}
	}
}
