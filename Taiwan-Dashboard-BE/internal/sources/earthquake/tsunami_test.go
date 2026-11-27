package earthquake

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseTsunami(t *testing.T) {
	reports, warnings, err := ParseTsunami(read(t, "testdata/tsunami.json"))
	if err != nil || len(warnings) != 0 || len(reports) != 2 {
		t.Fatalf("reports=%+v warnings=%v err=%v", reports, warnings, err)
	}
	r := reports[0]
	issued := time.Date(2026, 10, 24, 11, 36, 0, 0, pipeline.Taipei)
	if r.TsunamiNo != 115007 || r.Seq != 2 || !r.IssuedAt.Equal(issued) || r.Color != "綠色" || r.Type != "海嘯消息" ||
		r.Location != "大洋洲 萬那杜群島" || *r.Magnitude != 6.9 || *r.Lat != -13.8 || r.ValidUntil == nil ||
		r.URL == "" {
		t.Errorf("first = %+v", r)
	}
	if n, ok := reportSeq("第12報"); !ok || n != 12 {
		t.Errorf("reportSeq = %d %v", n, ok)
	}
}

func TestTsunamiRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/t": "testdata/tsunami.json"})
	p := &TsunamiPipeline{URL: srv.URL + "/t", APIKey: "k"}
	env := srv.Env(db, time.Date(2026, 11, 27, 9, 0, 0, 0, pipeline.Taipei))
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 2 {
		t.Fatalf("first run: %+v, %v", res, err)
	}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.tsunami WHERE geom IS NOT NULL AND magnitude > 6`); n != 2 {
		t.Errorf("stored = %d", n)
	}
}
