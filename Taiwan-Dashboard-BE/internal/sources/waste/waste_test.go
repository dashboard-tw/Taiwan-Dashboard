package waste

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
	b, err := os.ReadFile("testdata/waste.json")
	if err != nil {
		t.Fatal(err)
	}
	rows, n, err := Parse(b)
	if err != nil || n != 4 || len(rows) != 4 {
		t.Fatalf("rows %+v, %d, %v", rows, n, err)
	}
	for _, r := range rows {
		if r.Year != 2023 || r.Total <= 0 || r.PerCapita == nil {
			t.Errorf("row %+v", r)
		}
		if d := r.Total - (r.Garbage + r.Recycled + r.Food); d > 1 || d < -1 {
			t.Errorf("parts of %+v do not add up", r)
		}
	}
	if rows[0].Month != 0 || rows[2].Month != 12 {
		t.Errorf("months %d %d", rows[0].Month, rows[2].Month)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/w": "testdata/waste.json"})
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{URL: srv.URL + "/w", APIKey: "test"}
	res, err := p.Run(context.Background(), env)
	if err != nil || res.RowsInserted != 4 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.waste_county WHERE period_type = 'year'`); n != 2 {
		t.Errorf("year rows = %d", n)
	}
	if _, err := (&Pipeline{URL: srv.URL + "/w"}).Run(context.Background(), env); err == nil {
		t.Error("a missing key must fail")
	}
}
