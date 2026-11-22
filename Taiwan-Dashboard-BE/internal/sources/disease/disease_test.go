package disease

import (
	"context"
	"os"
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

func TestParse(t *testing.T) {
	cases, err := ParseDengue(read(t, "testdata/dengue.csv"))
	if err != nil || len(cases) != 7 {
		t.Fatalf("dengue %v, %v", cases, err)
	}
	total, imported := 0, 0
	for k, n := range cases {
		total += n
		if k.Imported {
			imported += n
		}
	}
	if total != 8 || imported != 4 {
		t.Errorf("total %d, imported %d", total, imported)
	}
	visits, err := ParseVisits(read(t, "testdata/nhi_ili.csv"), 2020)
	if err != nil || len(visits) != 8 {
		t.Fatalf("visits %+v, %v", visits, err)
	}
	v := visits[0]
	if v.Year != 2025 || v.Week != 19 || v.Visits <= 0 || v.Total < v.Visits || v.CountyCode == "" {
		t.Errorf("first %+v", v)
	}
	if got := WeekStart(2025, 19); got.Format("2006-01-02") != "2025-05-05" {
		t.Errorf("week 19 of 2025 starts %s", got)
	}
	if got := WeekStart(2026, 1); got.Format("2006-01-02") != "2025-12-29" {
		t.Errorf("week 1 of 2026 starts %s", got)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/dengue.csv": "testdata/dengue.csv", "/ili.csv": "testdata/nhi_ili.csv"})
	env := srv.Env(db, time.Date(2025, 5, 25, 9, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	dengue := &DenguePipeline{URL: srv.URL + "/dengue.csv"}
	for i := 0; i < 2; i++ {
		res, err := dengue.Run(ctx, env)
		if err != nil || res.RowsInserted != 6 || len(res.Warnings) != 1 {
			t.Fatalf("dengue run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT sum(cases) FROM data.dengue_daily`); n != 7 {
		t.Errorf("dengue cases = %d", n)
	}
	p := &VisitPipeline{URLs: map[string]string{"ili": srv.URL + "/ili.csv", "ev": srv.URL + "/ev.csv"}}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 8 || len(res.Warnings) != 1 {
		t.Fatalf("visits %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT county_code) FROM data.nhi_visit_week WHERE disease = 'ili'`); n != 2 {
		t.Errorf("counties = %d", n)
	}
}
