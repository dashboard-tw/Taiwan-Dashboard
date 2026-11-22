package river

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseQuality(t *testing.T) {
	list, n, err := ParseQuality(read(t, "testdata/rpi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 || len(list) != 4 {
		t.Fatalf("%d rows, samples %+v", n, list)
	}
	s := list[0]
	if s.SiteID != "1003" || s.RPI != 4 || s.Lat == nil || s.River == "" || s.SampledAt.Location() != pipeline.Taipei {
		t.Errorf("first %+v", s)
	}
	if list, _, err := ParseQuality([]byte("WQX_P_01查無資料")); err != nil || len(list) != 0 {
		t.Errorf("no data: %v, %v", list, err)
	}
}

func TestQualityRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/wqx": "testdata/rpi.json"})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &QualityPipeline{URL: srv.URL + "/wqx", APIKey: "test"}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 4 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Fatalf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.river_quality_latest`); n != 2 {
		t.Errorf("latest per site = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.river_quality_site WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 2 {
		t.Errorf("located sites = %d", n)
	}
	if _, err := (&QualityPipeline{URL: srv.URL + "/wqx"}).Run(ctx, env); err == nil {
		t.Error("a missing key must fail")
	}
}
