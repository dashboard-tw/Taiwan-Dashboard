package school

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

func TestParse(t *testing.T) {
	list, err := Parse(read(t, "testdata/e1.csv"), "elementary")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("schools %+v", list)
	}
	s := list[0]
	if s.County != "新北市" || s.Public || s.Year != 115 || s.Address == "" || s.Address[0] == '[' {
		t.Errorf("first %+v", s)
	}
	list, err = Parse(read(t, "testdata/j1.csv"), "junior")
	if err != nil || len(list) != 2 || list[0].Year != 115 {
		t.Fatalf("junior %+v, %v", list, err)
	}
	list, err = Parse(read(t, "testdata/high.csv"), "senior")
	if err != nil || len(list) != 2 {
		t.Fatalf("senior %+v, %v", list, err)
	}
	if schoolYear(time.Date(2026, 7, 31, 0, 0, 0, 0, pipeline.Taipei)) != 114 ||
		schoolYear(time.Date(2026, 8, 1, 0, 0, 0, 0, pipeline.Taipei)) != 115 {
		t.Error("school year starts in August")
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/115/e1.csv":   "testdata/e1.csv",
		"/115/high.csv": "testdata/high.csv",
		"/j1.csv":       "testdata/j1.csv",
		"/u1.csv":       "testdata/u1.csv",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2027, 9, 1, 9, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{URLs: map[string]string{
		"elementary": srv.URL + "/%d/e1.csv",
		"senior":     srv.URL + "/%d/high.csv",
		"junior":     srv.URL + "/j1.csv",
		"university": srv.URL + "/u1.csv",
	}}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted != 9 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT level) FROM ref.school WHERE county_code IS NOT NULL`); n != 4 {
		t.Errorf("levels = %d", n)
	}
	srv.Fail("/j1.csv", 500)
	if _, err := p.Run(ctx, env); err == nil {
		t.Error("a failed directory must fail the run")
	}
}
