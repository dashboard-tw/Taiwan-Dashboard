package fire

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestCause(t *testing.T) {
	for in, want := range map[string]string{
		"電氣因素(不含車輛)":  "電氣因素",
		"其他-因燃燒雜草、垃圾": "因燃燒雜草、垃圾",
		"人為縱火":        "縱火",
		"":            "其他",
	} {
		if got := Cause(in); got != want {
			t.Errorf("Cause(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	for file, want := range map[string]int{"testdata/2024.csv": 5, "testdata/2022.csv": 3} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		list, err := Parse(b)
		if err != nil || len(list) != want {
			t.Fatalf("%s: %+v, %v", file, list, err)
		}
		x := list[0]
		if x.ID == "" || x.County == "" || x.Cause == "" || x.Arrived == nil || x.Reported.Location() != pipeline.Taipei {
			t.Errorf("%s first %+v", file, x)
		}
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/2024.csv": "testdata/2024.csv", "/2022.csv": "testdata/2022.csv"})
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	p := &Pipeline{Files: map[int]string{2024: srv.URL + "/2024.csv", 2022: srv.URL + "/2022.csv"}}
	res, err := p.Run(context.Background(), env)
	if err != nil || res.RowsInserted != 8 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.fire_incident WHERE county_code IS NULL`); n != 0 {
		t.Errorf("fires without a county = %d", n)
	}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}
}
