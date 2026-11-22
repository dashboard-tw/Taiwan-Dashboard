package power

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseRenewable(t *testing.T) {
	rows, err := ParseRenewable(read(t, "testdata/renewable.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 32 {
		t.Fatalf("%d rows", len(rows))
	}
	if r := rows[0]; !r.Month.Equal(time.Date(2007, 1, 1, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("first %+v", r)
	}
	if _, err := ParseRenewable([]byte("日期(年/月),單位,太陽光電(數值)\n")); err == nil {
		t.Error("missing columns must fail")
	}
}

func TestRenewableRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/re.csv": "testdata/renewable.csv"})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &RenewablePipeline{URL: srv.URL + "/re.csv"}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 32 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if !res.LastObservedAt.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("observed %v", res.LastObservedAt)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Fatalf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT round(sum(gen_mwh)) FROM data.renewable_month WHERE month = '2026-08-01'`); n != 3274740 {
		t.Errorf("August total = %d", n)
	}
}
