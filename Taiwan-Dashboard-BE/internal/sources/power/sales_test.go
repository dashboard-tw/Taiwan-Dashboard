package power

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseSales(t *testing.T) {
	list, err := ParseSales(read(t, "testdata/sales.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 8 {
		t.Fatalf("%d rows", len(list))
	}
	if s := list[1]; s.Sector != "service" || s.KWh != 636471432 || !s.Month.Equal(time.Date(2025+1, 8, 1, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("second %+v", s)
	}
}

func TestSalesRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/s.csv": "testdata/sales.csv"})
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &SalesPipeline{URL: srv.URL + "/s.csv"}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 8 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_sales_month WHERE county_code = '63000'`); n != 4 {
		t.Errorf("Taipei rows = %d", n)
	}
}
