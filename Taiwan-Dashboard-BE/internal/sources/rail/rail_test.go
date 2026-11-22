package rail

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
	alerts, err := ParseTRAAlerts(read(t, "testdata/tra_alert.json"))
	if err != nil || len(alerts) != 2 {
		t.Fatalf("TRA alerts %+v, %v", alerts, err)
	}
	a := alerts[0]
	if a.Operator != "TRA" || a.Title == "" || len(a.Lines) == 0 || len(a.Stations) == 0 || a.StartsAt == nil || a.EndsAt == nil {
		t.Errorf("first alert %+v", a)
	}
	if thsr, err := ParseTHSRAlerts(read(t, "testdata/thsr_normal.json")); err != nil || len(thsr) != 0 {
		t.Errorf("THSR normal %+v, %v", thsr, err)
	}
	trains, updated, err := ParseLive(read(t, "testdata/live.json"))
	if err != nil || len(trains) != 4 || updated == nil {
		t.Fatalf("live %+v, %v", trains, err)
	}
	if tr := trains[2]; tr.DelayMin != 5 || tr.Type == "" || tr.StationName == "" {
		t.Errorf("delayed train %+v", tr)
	}
	stations, err := ParseStations(read(t, "testdata/station.json"))
	if err != nil || len(stations) == 0 {
		t.Fatalf("stations %+v, %v", stations, err)
	}
	if s := stations[0]; s.Lat == nil || s.Address == "" || s.Address[0] >= '0' && s.Address[0] <= '9' {
		t.Errorf("station %+v", s)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":                 "testdata/token.json",
		"/api/" + TRAAlertPath:   "testdata/tra_alert.json",
		"/api/" + THSRAlertPath:  "testdata/thsr_normal.json",
		"/api/" + TRALivePath:    "testdata/live.json",
		"/api/" + TRAStationPath: "testdata/station.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 22, 20, 0, 0, pipeline.Taipei))
	c := &tdx.Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: tdx.NewLimiter(100, time.Minute)}
	if res, err := (&StationPipeline{TDX: c}).Run(ctx, env); err != nil || res.RowsInserted == 0 {
		t.Fatalf("stations %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.tra_station WHERE county_code IS NULL`); n != 0 {
		t.Errorf("stations without a county = %d", n)
	}
	if res, err := (&AlertPipeline{TDX: c}).Run(ctx, env); err != nil || res.RowsInserted != 2 {
		t.Fatalf("alerts %+v, %v", res, err)
	}
	live := &LivePipeline{TDX: c}
	for i := 0; i < 2; i++ {
		if res, err := live.Run(ctx, env); err != nil || res.RowsInserted != 4 {
			t.Fatalf("live run %d: %+v, %v", i, res, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.tra_train_live`); n != 4 {
		t.Errorf("live trains = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT delayed FROM data.tra_delay_history`); n != 2 {
		t.Errorf("delayed trains = %d", n)
	}
}
