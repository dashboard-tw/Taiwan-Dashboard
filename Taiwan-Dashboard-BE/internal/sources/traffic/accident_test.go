package traffic

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseAccidents(t *testing.T) {
	list, err := ParseAccidents(read(t, "testdata/a1.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("accidents %+v", list)
	}
	a := list[0]
	if a.OccurredAt != time.Date(2026, 1, 1, 5, 37, 0, 0, pipeline.Taipei) || a.Deaths != 1 || a.Injuries != 0 ||
		a.Lat == nil || a.Vehicle == "" || a.Cause == "" || a.Type == "" {
		t.Errorf("first %+v", a)
	}
	if list[3].Deaths != 2 {
		t.Errorf("the freeway accident %+v", list[3])
	}
}

func TestAccidentRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/a1.csv": "testdata/a1.csv"})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &AccidentPipeline{URL: srv.URL + "/a1.csv"}
	for i := 0; i < 2; i++ {
		res, err := p.Run(ctx, env)
		if err != nil || res.RowsInserted != 4 || len(res.Warnings) != 0 {
			t.Fatalf("run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.traffic_accident WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 4 {
		t.Errorf("located accidents = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT sum(deaths) FROM data.traffic_accident`); n != 5 {
		t.Errorf("deaths = %d", n)
	}
}
