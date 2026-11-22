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

func TestParseLivestock(t *testing.T) {
	b, err := os.ReadFile("testdata/poultry.json")
	if err != nil {
		t.Fatal(err)
	}
	days, err := ParsePoultry(b)
	if err != nil || len(days) != 4 {
		t.Fatalf("poultry %+v, %v", days, err)
	}
	if d := days[3]; d.Date.Format("2006-01-02") != "2026-10-21" || d.EggFarm == nil || *d.EggFarm != 35.5 || d.BroilerLarge == nil {
		t.Errorf("first day %+v", d)
	}
	if d := days[0]; d.EggFarm != nil || d.EggTransport == nil {
		t.Errorf("2010 %+v", d)
	}
	spiky := []Poultry{{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EggTransport: ptr(38.5)},
		{Date: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), EggTransport: ptr(67)},
		{Date: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), EggTransport: ptr(39)}}
	dropSpikes(spiky)
	if spiky[1].EggTransport != nil || spiky[2].EggTransport == nil {
		t.Errorf("spikes %+v", spiky)
	}
	b, err = os.ReadFile("testdata/hog.json")
	if err != nil {
		t.Fatal(err)
	}
	hogs, err := ParseHogs(b)
	if err != nil || len(hogs) != 4 {
		t.Fatalf("hogs %+v, %v", hogs, err)
	}
	if h := hogs[0]; h.Date.Format("2006-01-02") != "2026-10-22" || h.Head <= 0 || h.Price == nil || h.Weight == nil {
		t.Errorf("first hog market %+v", h)
	}
}

func TestLivestockRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/p": "testdata/poultry.json", "/h": "testdata/hog.json"})
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	p := &PoultryPipeline{URL: srv.URL + "/p"}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 4 {
		t.Fatalf("poultry %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Fatalf("poultry again %+v, %v", res, err)
	}
	if res, err := (&HogPipeline{URL: srv.URL + "/h"}).Run(ctx, env); err != nil || res.RowsInserted != 4 {
		t.Fatalf("hogs %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.hog_trade WHERE price IS NOT NULL`); n != 4 {
		t.Errorf("hog prices = %d", n)
	}
}

func ptr(v float64) *float64 { return &v }
