package aviation

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/sources/tdx"
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
	dep, err := ParseFlights(read(t, "testdata/departure.json"), "dep")
	if err != nil || len(dep) != 6 {
		t.Fatalf("departures %+v, %v", dep, err)
	}
	want := []string{"cancelled", "on_time", "done", "changed", "delayed", "cancelled"}
	for i, f := range dep {
		if f.Status != want[i] {
			t.Errorf("departure %d: %s (%s), want %s", i, f.Status, f.Remark, want[i])
		}
	}
	if f := dep[2]; f.Actual == nil || f.Airport == "" || f.Other == "" || f.Scheduled.Location() != pipeline.Taipei {
		t.Errorf("departed %+v", f)
	}
	arr, err := ParseFlights(read(t, "testdata/arrival.json"), "arr")
	if err != nil || len(arr) != 5 || arr[2].Status != "done" {
		t.Fatalf("arrivals %+v, %v", arr, err)
	}
	airports, err := ParseAirports(read(t, "testdata/airport.json"))
	if err != nil || len(airports) != 6 {
		t.Fatalf("airports %+v, %v", airports, err)
	}
	airlines, err := ParseAirlines(read(t, "testdata/airline.json"))
	if err != nil || len(airlines) != 4 {
		t.Fatalf("airlines %+v, %v", airlines, err)
	}
	for _, a := range airlines {
		if a.ID == "B7" && a.Name != "立榮航空" {
			t.Errorf("UNI Air %+v", a)
		}
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":                "testdata/token.json",
		"/api/" + DeparturePath: "testdata/departure.json",
		"/api/" + ArrivalPath:   "testdata/arrival.json",
		"/api/" + AirportPath:   "testdata/airport.json",
		"/api/" + AirlinePath:   "testdata/airline.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	c := &tdx.Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: tdx.NewLimiter(100, time.Minute)}
	if res, err := (&RefPipeline{TDX: c}).Run(ctx, env); err != nil || res.RowsInserted != 10 {
		t.Fatalf("airports %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.airport WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 4 {
		t.Errorf("airports in Taiwan = %d", n)
	}
	p := &FlightPipeline{TDX: c}
	for i := 0; i < 2; i++ {
		if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 10 {
			t.Fatalf("flights run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.flight WHERE status = 'cancelled'`); n != 2 {
		t.Errorf("cancelled flights = %d", n)
	}
	later := srv.Env(db, time.Date(2026, 12, 24, 9, 0, 0, 0, pipeline.Taipei))
	if _, err := p.Run(ctx, later); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.flight`); n != 0 {
		t.Errorf("flights after two months = %d", n)
	}
}
