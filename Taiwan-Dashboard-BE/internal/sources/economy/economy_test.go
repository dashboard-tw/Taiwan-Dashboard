package economy

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

func series(points []Point) map[string]int {
	m := map[string]int{}
	for _, p := range points {
		m[p.Series]++
	}
	return m
}

func TestParse(t *testing.T) {
	since := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	cpi, err := ParseCPI(read(t, "testdata/cpi.xml"), since)
	if err != nil {
		t.Fatal(err)
	}
	s := series(cpi)
	if len(s) != 8 || s["cpi:總指數"] != 2 || s["cpi_yoy:食物類"] != 2 || s["cpi:穀類及其製品"] != 2 || s["cpi:外食費"] != 2 {
		t.Errorf("CPI series %v", s)
	}
	u, err := ParseUnemployment(read(t, "testdata/unemployment.xml"), since)
	if err != nil {
		t.Fatal(err)
	}
	s = series(u)
	if s["unemployment:總計"] != 2 || s["unemployment:15-19歲"] != 2 || s["unemployment:65歲以上"] != 2 || s["unemployment:大學及以上"] != 2 {
		t.Errorf("unemployment series %v", s)
	}
	for _, p := range u {
		if p.Series == "unemployment:總計" && p.Month.Month() == time.August && p.Value != 3.41 {
			t.Errorf("August %+v", p)
		}
	}
	cycle, err := ParseCycle(read(t, "testdata/cycle.zip"), since)
	if err != nil {
		t.Fatal(err)
	}
	s = series(cycle)
	if s["signal_score"] != 8 || s["leading"] != 8 || s["coincident"] != 8 || s["lagging"] != 8 {
		t.Errorf("cycle series %v", s)
	}
	towns, err := ParseIncome(read(t, "testdata/income.csv"))
	if err != nil || len(towns) != 3 {
		t.Fatalf("towns %+v, %v", towns, err)
	}
	if tw := towns[0]; tw.Area != "臺北市松山區" || tw.Households != 69735 || tw.Median != 758 {
		t.Errorf("first town %+v", tw)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/cpi.xml": "testdata/cpi.xml", "/u.xml": "testdata/unemployment.xml", "/cycle.zip": "testdata/cycle.zip",
		"/ias112/112_165-9.csv": "testdata/income.csv",
	})
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	p := &IndicatorPipeline{CPIURL: srv.URL + "/cpi.xml", UnemploymentURL: srv.URL + "/u.xml", CycleURL: srv.URL + "/cycle.zip"}
	res, err := p.Run(ctx, env)
	if err != nil || res.RowsInserted == 0 || len(res.Warnings) != 0 {
		t.Fatalf("indicators %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Fatalf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.econ_month WHERE series = 'signal_score'`); n != 8 {
		t.Errorf("signal scores = %d", n)
	}
	ip := &IncomePipeline{URL: srv.URL + "/ias%d/%d_165-9.csv"}
	if res, err := ip.Run(ctx, env); err != nil || res.RowsInserted != 2 {
		t.Fatalf("income %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.income_town WHERE year = 2023 AND county_code = '63000' AND town = '松山區'`); n != 1 {
		t.Errorf("松山區 = %d", n)
	}
}
