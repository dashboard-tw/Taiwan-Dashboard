package welfare

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseChildcare(t *testing.T) {
	b, err := os.ReadFile("testdata/welfare.csv")
	if err != nil {
		t.Fatal(err)
	}
	list, err := ParseChildcare(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("centres %+v", list)
	}
	if c := list[0]; c.Lat != nil || c.Address == "" {
		t.Errorf("the centre without coordinates %+v", c)
	}
	if c := list[1]; c.Lat == nil {
		t.Errorf("second %+v", c)
	}
}

func TestChildcareRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/w.csv": "testdata/welfare.csv"})
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	res, err := (&ChildcarePipeline{URL: srv.URL + "/w.csv"}).Run(context.Background(), env)
	if err != nil || res.RowsInserted != 2 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.childcare_center WHERE county_code IS NOT NULL`); n != 2 {
		t.Errorf("centres with a county = %d", n)
	}
}
