package traffic

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

func TestParseSections(t *testing.T) {
	list, err := ParseSections(read(t, "testdata/section.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("sections %+v", list)
	}
	s := list[0]
	if s.ID != "0001" || s.Road != "國道1號" || s.Direction != "S" || s.LengthKm == nil || *s.LengthKm != 1.1 ||
		s.SpeedLimit == nil || *s.SpeedLimit != 100 {
		t.Errorf("first section %+v", s)
	}
	shapes, err := ParseShapes(read(t, "testdata/shape.xml"))
	if err != nil || len(shapes) != 3 || shapes["0001"][:11] != "LINESTRING(" {
		t.Errorf("shapes %v, %v", len(shapes), err)
	}
}

func TestParseLive(t *testing.T) {
	updated, list, err := ParseLive(read(t, "testdata/live.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.IsZero() || len(list) != 4 {
		t.Fatalf("live %v %+v", updated, list)
	}
	l := list[0]
	if l.SectionID != "0001" || l.SpeedKmh == nil || l.Level == nil || *l.Level < 1 || *l.Level > 5 {
		t.Errorf("first %+v", l)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/section.xml": "testdata/section.xml",
		"/shape.xml":   "testdata/shape.xml",
		"/live.xml":    "testdata/live.xml",
	})
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei)
	live := &LivePipeline{URL: srv.URL + "/live.xml"}
	if _, err := live.Run(ctx, srv.Env(db, now)); err == nil {
		t.Error("live traffic without sections should fail")
	}
	sec := &SectionPipeline{URL: srv.URL + "/section.xml", ShapeURL: srv.URL + "/shape.xml"}
	if res, err := sec.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 3 {
		t.Fatalf("sections %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.freeway_section WHERE county_code = '10017' AND geom IS NOT NULL`); n != 3 {
		t.Errorf("sections in Keelung = %d", n)
	}
	if res, err := sec.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second section run %+v, %v", res, err)
	}
	res, err := live.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsFetched != 4 || res.RowsInserted != 3 {
		t.Fatalf("live %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.freeway_traffic_current`); n != 3 {
		t.Errorf("current rows = %d", n)
	}
	if res, err := live.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second live run %+v, %v", res, err)
	}
}
