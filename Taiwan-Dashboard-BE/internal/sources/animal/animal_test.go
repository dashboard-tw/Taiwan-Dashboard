package animal

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
	b, err := os.ReadFile("testdata/adopt.json")
	if err != nil {
		t.Fatal(err)
	}
	list, err := ParseAnimals(b)
	if err != nil || len(list) != 5 {
		t.Fatalf("animals %+v, %v", list, err)
	}
	if a := list[0]; a.Kind != "狗" || a.Shelter == "" || a.Opened == nil || a.Sterilized == nil || a.Variety != "混種犬" {
		t.Errorf("first %+v", a)
	}
	b, err = os.ReadFile("testdata/stats.json")
	if err != nil {
		t.Fatal(err)
	}
	months, err := ParseStats(b)
	if err != nil || len(months) != 5 {
		t.Fatalf("months %+v, %v", months, err)
	}
	if m := months[0]; m.Month.Year() != 2026 || m.Accepted == 0 {
		t.Errorf("first month %+v", m)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/adopt": "testdata/adopt.json", "/stats": "testdata/stats.json"})
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if res, err := (&AdoptPipeline{URL: srv.URL + "/adopt"}).Run(ctx, env); err != nil || res.RowsInserted != 4 || len(res.Warnings) != 0 {
			t.Fatalf("adopt run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.shelter_animal WHERE county_code IS NULL`); n != 0 {
		t.Errorf("animals without a county = %d", n)
	}
	if res, err := (&StatsPipeline{URL: srv.URL + "/stats"}).Run(ctx, env); err != nil || res.RowsInserted != 4 {
		t.Fatalf("stats %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.shelter_month WHERE county_code = '09007'`); n != 2 {
		t.Errorf("連江縣 months = %d", n)
	}
}
