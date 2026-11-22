package debris

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParse(t *testing.T) {
	b, err := os.ReadFile("testdata/alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	alerts, err := Parse(b)
	if err != nil || len(alerts) != 4 {
		t.Fatalf("alerts %+v, %v", alerts, err)
	}
	if a := alerts[0]; a.Type != "D" || a.ID == "" || a.Level != "yellow" || a.Village == "" || a.Updated == nil {
		t.Errorf("debris flow %+v", a)
	}
	if a := alerts[3]; a.Type != "L" || a.Name == "" || a.ID == "" {
		t.Errorf("landslide %+v", a)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/a": "testdata/alerts.json"})
	p := &Pipeline{URL: srv.URL + "/a"}
	ctx := context.Background()
	at := func(m int) pipeline.Env {
		return srv.Env(db, time.Date(2026, 10, 24, 1, 0, 0, 0, pipeline.Taipei).Add(time.Duration(m)*time.Minute))
	}
	if res, err := p.Run(ctx, at(0)); err != nil || res.RowsInserted != 4 {
		t.Fatalf("first run %+v, %v", res, err)
	}
	srv.Set("/a", "testdata/alerts_red.json")
	if res, err := p.Run(ctx, at(10)); err != nil || res.RowsInserted != 0 {
		t.Fatalf("second run %+v, %v", res, err)
	}
	srv.Set("/a", "testdata/alerts.json")
	if _, err := p.Run(ctx, at(20)); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.debris_alert WHERE level = 'red'`); n != 1 {
		t.Errorf("red alerts = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.debris_alert WHERE county_code IS NULL`); n != 0 {
		t.Errorf("alerts without a county = %d", n)
	}
	srv.Set("/a", "testdata/none.json")
	if res, err := p.Run(ctx, at(30)); err != nil || res.RowsFetched != 0 {
		t.Fatalf("no alerts %+v, %v", res, err)
	}
	srv.Set("/a", "testdata/alerts.json")
	if res, err := p.Run(ctx, at(150)); err != nil || res.RowsInserted != 4 {
		t.Fatalf("issued again %+v, %v", res, err)
	}
}
