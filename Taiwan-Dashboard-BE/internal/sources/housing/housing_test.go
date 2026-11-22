package housing

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseSeason(t *testing.T) {
	b, err := os.ReadFile("testdata/115S3.zip")
	if err != nil {
		t.Fatal(err)
	}
	deals, err := ParseSeason(b, "115S3")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, d := range deals {
		kinds[d.Kind]++
	}
	if kinds["sale"] != 6 || kinds["presale"] != 2 || kinds["rent"] != 3 {
		t.Fatalf("kinds %v", kinds)
	}
	d := deals[0]
	if d.Serial == "" || d.TotalPrice == nil || d.UnitPrice == nil || d.Area == nil || d.Date.Year() != 2026 || d.Letter != "a" {
		t.Errorf("first %+v", d)
	}
	if !d.Special {
		t.Errorf("special deal %+v", d)
	}
	if deals, err := ParseSeason([]byte("<html>not yet</html>"), "115S4"); err != nil || deals != nil {
		t.Errorf("an unpublished season: %v, %v", deals, err)
	}
	if Season(time.Date(2026, 10, 24, 0, 0, 0, 0, pipeline.Taipei)) != "115S4" {
		t.Error("season of October")
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/lvr/115S3": "testdata/115S3.zip"})
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{URL: srv.URL + "/lvr/%s"}
	res, err := p.Run(context.Background(), env)
	if err != nil || res.RowsInserted != 11 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.real_estate_deal WHERE county_code IS NULL`); n != 0 {
		t.Errorf("deals without a county = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.real_estate_deal WHERE county_code = '65000'`); n != 2 {
		t.Errorf("New Taipei deals = %d", n)
	}
	if _, err := p.Run(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.real_estate_deal`); n != 11 {
		t.Errorf("rows after a second run = %d", n)
	}
}
