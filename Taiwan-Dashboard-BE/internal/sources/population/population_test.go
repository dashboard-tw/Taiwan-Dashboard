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

func TestParsePage(t *testing.T) {
	for _, f := range []string{"testdata/114.json", "testdata/113.json"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParsePage(b)
		if err != nil {
			t.Fatal(f, err)
		}
		if p.TotalPages != 1 || len(p.Rows) != 5 {
			t.Fatalf("%s: %+v", f, p)
		}
		r := p.Rows[0]
		if r.Code != "65000" || r.Households == 0 || r.Population != r.Male+r.Female || r.Male == 0 {
			t.Errorf("%s first %+v", f, r)
		}
	}
	p, err := ParsePage([]byte(`{"responseCode":"OD-0102-S","responseMessage":"查無資料"}`))
	if err != nil || len(p.Rows) != 0 {
		t.Errorf("no data: %+v, %v", p, err)
	}
	if _, err := ParsePage([]byte(`{"responseCode":"OD-0201-E","responseMessage":"參數錯誤"}`)); err == nil {
		t.Error("an error response must fail")
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/ris/114": "testdata/114.json",
		"/ris/113": "testdata/113.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2027, 3, 1, 9, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{URL: srv.URL + "/ris/%d?page=%d"}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 4 || res.RowsFetched != 10 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if !res.LastObservedAt.Equal(time.Date(2025, 12, 31, 0, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("observed %v", res.LastObservedAt)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT year) FROM data.population_year`); n != 2 {
		t.Errorf("years = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.population_year WHERE population <> male + female`); n != 0 {
		t.Errorf("rows where population != male + female: %d", n)
	}
}
