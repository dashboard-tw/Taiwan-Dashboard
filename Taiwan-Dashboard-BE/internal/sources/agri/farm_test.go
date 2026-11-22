package agri

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
	b, err := os.ReadFile("testdata/farm.json")
	if err != nil {
		t.Fatal(err)
	}
	list, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("prices %+v", list)
	}
	c := list[0]
	if c.Crop != "甘藍" || c.Category != "vegetable" || c.Volume != 56594 || c.Markets != 1 ||
		!c.Date.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("cabbage %+v", c)
	}
	if round2(c.AvgPrice) != 38.16 {
		t.Errorf("cabbage price %v", c.AvgPrice)
	}
	if rocDate(c.Date) != "115.10.20" {
		t.Errorf("roc date %s", rocDate(c.Date))
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/farm": "testdata/farm.json"})
	env := srv.Env(db, time.Date(2026, 10, 21, 17, 0, 0, 0, pipeline.Taipei))
	res, err := (&Pipeline{URL: srv.URL + "/farm"}).Run(context.Background(), env)
	if err != nil || res.RowsInserted != 3 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.farm_price_daily WHERE trade_date = '2026-10-20'`); n != 3 {
		t.Errorf("rows = %d", n)
	}
}
