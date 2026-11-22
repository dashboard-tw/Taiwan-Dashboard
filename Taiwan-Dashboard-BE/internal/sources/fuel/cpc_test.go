package fuel

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseHistory(t *testing.T) {
	b, err := os.ReadFile("testdata/history.xml")
	if err != nil {
		t.Fatal(err)
	}
	list, err := ParseHistory(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("prices %+v", list)
	}
	last := list[2]
	if last.Price != 31.2 || last.Unit != "元/公升" ||
		!last.EffectiveAt.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("last %+v", last)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	routes := map[string]string{}
	for _, p := range Products {
		routes["/h"+string(rune('0'+p.ID))] = "testdata/history.xml"
	}
	srv := pipelinetest.NewServer(t, routes)
	p := &Pipeline{URL: srv.URL + "/h%d"}
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei)
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsInserted != 3*len(Products) {
		t.Fatalf("run %+v, %v", res, err)
	}
	if !res.LastObservedAt.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("observed %v", res.LastObservedAt)
	}
	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}
}
