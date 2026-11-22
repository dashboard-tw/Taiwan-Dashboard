package population

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseVitalPage(t *testing.T) {
	b, err := os.ReadFile("testdata/vital.json")
	if err != nil {
		t.Fatal(err)
	}
	rows, pages, err := ParseVitalPage(b)
	if err != nil || pages != 1 || len(rows) != 5 {
		t.Fatalf("rows %+v, pages %d, %v", rows, pages, err)
	}
	if rows[0].Code != "65000" || rows[0].Births != 2 {
		t.Errorf("first %+v", rows[0])
	}
	if rows, _, err := ParseVitalPage([]byte(`{"responseCode":"OD-0102-S","responseMessage":"查無資料"}`)); err != nil || len(rows) != 0 {
		t.Errorf("unpublished month: %v, %v", rows, err)
	}
}

func TestVitalRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/vital/11509": "testdata/vital.json"})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &VitalPipeline{URL: srv.URL + "/vital/%d?page=%d"}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 2 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if !res.LastObservedAt.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("observed %v", res.LastObservedAt)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT sum(births) FROM data.vital_month WHERE month = '2026-09-01'`); n == 0 {
		t.Error("no births stored")
	}
}
