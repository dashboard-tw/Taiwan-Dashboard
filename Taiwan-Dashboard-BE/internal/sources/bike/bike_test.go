package bike

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
	list, err := ParseStations(read(t, "testdata/station.json"))
	if err != nil || len(list) != 3 {
		t.Fatalf("stations %+v, %v", list, err)
	}
	if s := list[0]; s.Name == "" || s.Name[0] == 'Y' || s.Capacity == nil || s.Lat == nil || s.ServiceType != 2 {
		t.Errorf("first %+v", s)
	}
	av, err := ParseAvailability(read(t, "testdata/availability.json"))
	if err != nil || len(av) != 4 {
		t.Fatalf("availability %+v, %v", av, err)
	}
	if a := av[0]; a.Rent != 5 || a.Return != 23 || a.EBikes != 2 || a.UpdatedAt == nil {
		t.Errorf("first %+v", a)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":                                "testdata/token.json",
		"/api/v2/Bike/Station/City/Taipei":      "testdata/station.json",
		"/api/v2/Bike/Availability/City/Taipei": "testdata/availability.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 22, 20, 0, 0, pipeline.Taipei))
	c := &tdx.Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: tdx.NewLimiter(100, time.Minute)}
	taipei := []City{{"Taipei", "63000"}}
	if res, err := (&StationPipeline{TDX: c, Cities: taipei}).Run(ctx, env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("stations %+v, %v", res, err)
	}
	res, err := (&AvailabilityPipeline{TDX: c, Cities: taipei}).Run(ctx, env)
	if err != nil || res.RowsInserted != 3 || len(res.Warnings) != 0 {
		t.Fatalf("availability %+v, %v", res, err)
	}
	var stations, bikes, empty, full int
	if err := db.QueryRow(`SELECT stations, bikes, empty_stations, full_stations FROM data.bike_county_history`).
		Scan(&stations, &bikes, &empty, &full); err != nil {
		t.Fatal(err)
	}
	if stations != 3 || bikes != 17 || empty != 1 || full != 1 {
		t.Errorf("summary stations %d bikes %d empty %d full %d", stations, bikes, empty, full)
	}
	srv.Fail("/api/v2/Bike/Availability/City/Taipei", 500)
	if _, err := (&AvailabilityPipeline{TDX: c, Cities: taipei}).Run(ctx, env); err == nil {
		t.Error("every city failing is an error")
	}
}
