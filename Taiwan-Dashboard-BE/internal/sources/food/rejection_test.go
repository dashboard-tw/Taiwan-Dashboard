package food

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
	b, err := os.ReadFile("testdata/rejection.csv")
	if err != nil {
		t.Fatal(err)
	}
	list, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("rejections %+v", list)
	}
	x := list[0]
	if x.Origin == "" || x.Product == "" || x.Reason == "" || x.Published == nil || x.ID == "" {
		t.Errorf("first %+v", x)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/r.csv": "testdata/rejection.csv"})
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{URL: srv.URL + "/r.csv"}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 3 || res.RowsFetched != 3 {
		t.Fatalf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.food_rejection`); n != 3 {
		t.Errorf("rows = %d", n)
	}
}
