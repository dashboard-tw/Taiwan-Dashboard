package flood

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParsePage(t *testing.T) {
	b, err := os.ReadFile("testdata/page.json")
	if err != nil {
		t.Fatal(err)
	}
	list, next, err := ParsePage(b)
	if err != nil || len(list) != 5 || next != "" {
		t.Fatalf("page %+v, %q, %v", list, next, err)
	}
	if r := list[0]; r.Code == "" || r.Name == "" || r.Lat == nil || r.Depth == nil || r.Observed.IsZero() {
		t.Errorf("first %+v", r)
	}
	b, err = os.ReadFile("testdata/page_next.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, next, err := ParsePage(b); err != nil || next != "https://example.test/next" {
		t.Errorf("next link %q, %v", next, err)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/p": "testdata/page.json"})
	env := srv.Env(db, time.Date(2026, 10, 24, 5, 0, 0, 0, pipeline.Taipei))
	res, err := (&Pipeline{URL: srv.URL + "/p"}).Run(context.Background(), env)
	if err != nil || res.RowsInserted != 3 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.flood_sensor WHERE county_code IS NOT NULL`); n != 4 {
		t.Errorf("sensors with a county = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.flood_obs`); n != 1 {
		t.Errorf("flooded readings = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT sum(flooded) FROM data.flood_county_history`); n != 1 {
		t.Errorf("flooded sensors in the county summary = %d", n)
	}
}
