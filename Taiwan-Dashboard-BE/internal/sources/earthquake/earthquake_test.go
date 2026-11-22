package earthquake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIntensityRank(t *testing.T) {
	for in, want := range map[string]int{"1級": 1, "4級": 4, "5弱": 5, "5強": 6, "6強": 8, "7級": 9} {
		if got, ok := IntensityRank(in); !ok || got != want {
			t.Errorf("IntensityRank(%q) = %d, %v", in, got, ok)
		}
	}
	if _, ok := IntensityRank("8級"); ok {
		t.Error("8級 is not on the scale")
	}
}

func TestParseSignificant(t *testing.T) {
	reports, warnings, err := Parse(read(t, "testdata/significant.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 4 || len(warnings) != 2 {
		t.Fatalf("reports=%d warnings=%v", len(reports), warnings)
	}
	r := reports[0]
	if r.ID != "2026069" || *r.EarthquakeNo != 115069 || *r.Magnitude != 4.3 || *r.DepthKm != 33.8 ||
		*r.Lat != 23.56 || *r.Lon != 121.45 || r.Color != "綠色" {
		t.Errorf("first = %+v", r)
	}
	if !r.OriginTime.Equal(time.Date(2026, 10, 13, 17, 30, 10, 0, pipeline.Taipei)) {
		t.Errorf("origin = %v", r.OriginTime)
	}
	if len(r.Counties) != 11 || r.Counties[0].County != "花蓮縣" || r.Counties[0].Intensity != "3級" {
		t.Errorf("counties = %+v", r.Counties)
	}
	if label, rank := r.MaxIntensity(); label != "3級" || rank != 3 {
		t.Errorf("max = %s %d", label, rank)
	}
	if r.Location != "花蓮縣政府南南西方 50.8 公里 (位於花蓮縣豐濱鄉)" {
		t.Errorf("location = %q", r.Location)
	}
	if label, _ := reports[2].MaxIntensity(); reports[2].ID != "2026057" || label != "4級" || reports[2].Color != "黃色" {
		t.Errorf("yellow report = %+v", reports[2])
	}
	if odd := reports[3]; len(odd.Counties) != 10 {
		t.Errorf("unknown intensity should drop one county: %d", len(odd.Counties))
	}
}

func TestParseLocalAndDenied(t *testing.T) {
	reports, warnings, err := Parse(read(t, "testdata/local.json"))
	if err != nil || len(reports) != 3 || len(warnings) != 0 {
		t.Fatalf("reports=%d warnings=%v err=%v", len(reports), warnings, err)
	}
	if reports[0].ID != "2026102114512039" || len(reports[0].Counties) != 2 {
		t.Errorf("first = %+v", reports[0])
	}
	if _, _, err := Parse(read(t, "testdata/denied.json")); err == nil {
		t.Error("success=false should fail")
	}
}

func TestRunKeepsTheKeyOutOfErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Authorization") != "secret-key" {
			t.Errorf("key not sent: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := &Pipeline{Kind: "significant", URL: srv.URL, APIKey: "secret-key"}
	f := pipeline.NewFetcher("test")
	f.Retries = 0
	_, err := p.Run(context.Background(), pipeline.Env{Fetch: f, Now: time.Now})
	if err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Errorf("err = %v", err)
	}
	if _, err := (&Pipeline{Kind: "local", URL: srv.URL}).Run(context.Background(), pipeline.Env{}); err == nil {
		t.Error("a missing key should fail before fetching")
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/sig":   "testdata/significant.json",
		"/local": "testdata/local.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 22, 11, 30, 0, 0, pipeline.Taipei))
	sig := &Pipeline{Kind: "significant", URL: srv.URL + "/sig", APIKey: "k"}
	local := &Pipeline{Kind: "local", URL: srv.URL + "/local", APIKey: "k"}

	if res, err := sig.Run(ctx, env); err != nil || res.RowsInserted != 4 {
		t.Fatalf("significant: %+v, %v", res, err)
	}
	if res, err := local.Run(ctx, env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("local: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.earthquake_intensity`); n != 11+6+19+10+2+2+9 {
		t.Errorf("county intensities = %d", n)
	}
	var label string
	var rank int
	if err := db.QueryRow(`SELECT max_intensity, max_intensity_rank FROM data.earthquake WHERE report_id = '2026057'`).Scan(&label, &rank); err != nil || label != "4級" || rank != 4 {
		t.Errorf("2026057 max = %s %d (%v)", label, rank, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.earthquake WHERE kind = 'local' AND geom IS NOT NULL`); n != 3 {
		t.Errorf("local epicenters = %d", n)
	}
	if res, err := sig.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
}
