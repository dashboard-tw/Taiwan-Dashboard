package ev

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
	stations, err := ParseStations(read(t, "testdata/station.json"))
	if err != nil || len(stations) != 3 {
		t.Fatalf("stations %+v, %v", stations, err)
	}
	if s := stations[0]; s.DC != 4 || s.AC != 0 || len(s.Types) != 1 || s.Types[0] != "J1772" || s.Lat == nil {
		t.Errorf("DC station %+v", s)
	}
	if s := stations[1]; s.AC != 3 || len(s.Types) != 2 || s.ChargingRate == "" {
		t.Errorf("AC station %+v", s)
	}
	live, err := ParseLive(read(t, "testdata/live.json"))
	if err != nil || len(live) != 3 {
		t.Fatalf("live %+v, %v", live, err)
	}
	if l := live[0]; l.Free != 1 || l.InUse != 1 || l.Broken != 1 || l.Updated == nil {
		t.Errorf("first station %+v", l)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":                         "testdata/token.json",
		"/api/v1/EV/Station/City/Taipei": "testdata/station.json",
		"/api/v1/EV/ConnectorLiveStatus/City/Taipei": "testdata/live.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 24, 5, 0, 0, 0, pipeline.Taipei))
	c := &tdx.Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: tdx.NewLimiter(100, time.Minute)}
	two := []City{{"Taipei", "63000"}, {"Keelung", "10017"}}
	res, err := (&StationPipeline{TDX: c, Cities: two}).Run(ctx, env)
	if err != nil || res.RowsInserted != 3 || len(res.Warnings) != 1 {
		t.Fatalf("stations %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.ev_station WHERE geom IS NOT NULL AND cardinality(connector_types) > 0`); n != 3 {
		t.Errorf("stations with a place and connectors = %d", n)
	}
	live := &LivePipeline{TDX: c, Cities: two}
	for i := 0; i < 2; i++ {
		if res, err := live.Run(ctx, env); err != nil || res.RowsInserted != 3 || len(res.Warnings) != 0 {
			t.Fatalf("live run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT free FROM data.ev_county_history WHERE county_code = '63000'`); n != 8 {
		t.Errorf("free connectors in Taipei = %d", n)
	}
}
