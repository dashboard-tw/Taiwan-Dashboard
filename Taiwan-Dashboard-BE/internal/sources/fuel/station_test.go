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

func TestParseStations(t *testing.T) {
	b, err := os.ReadFile("testdata/stations.json")
	if err != nil {
		t.Fatal(err)
	}
	list, err := ParseStations(b)
	if err != nil || len(list) != 4 {
		t.Fatalf("stations %+v, %v", list, err)
	}
	s := list[0]
	if s.ID == "" || s.Lat == nil || !s.Operating || !s.Diesel || s.Address[:9] != s.County {
		t.Errorf("first %+v", s)
	}
	if !list[3].Freeway {
		t.Errorf("the freeway station %+v", list[3])
	}
}

func TestStationRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/s": "testdata/stations.json"})
	env := srv.Env(db, time.Date(2026, 10, 24, 9, 0, 0, 0, pipeline.Taipei))
	res, err := (&StationPipeline{URL: srv.URL + "/s"}).Run(context.Background(), env)
	if err != nil || res.RowsInserted != 4 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.fuel_station WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 4 {
		t.Errorf("located stations = %d", n)
	}
}
