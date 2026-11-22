package weather

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseObservations(t *testing.T) {
	obs, _, err := ParseObservations(read(t, "testdata/obs_hourly.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 5 {
		t.Fatalf("stations = %d", len(obs))
	}
	byID := map[string]WeatherObservation{}
	for _, o := range obs {
		byID[o.StationID] = o
	}
	m := byID["466881"]
	if m.Kind() != "manned" || m.TempC == nil || m.Lat == nil || m.CountyCode == "" {
		t.Errorf("manned station %+v", m)
	}
	if byID["12J990"].Kind() != "auto" {
		t.Error("12J990 is an automatic station")
	}
	if v := byID["C0TC00"].TempC; v != nil {
		t.Errorf("missing temperature = %v", *v)
	}
	if k := byID["C0W240"]; k.Lon == nil || k.CountyCode != "09020" {
		t.Errorf("Kinmen station %+v", k)
	}
	if !m.ObservedAt.Equal(time.Date(2026, 10, 23, 12, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("time %v", m.ObservedAt)
	}
}

func TestObsRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/hourly": "testdata/obs_hourly.json",
		"/tenmin": "testdata/obs_10min.json",
	})
	p := &ObsPipeline{HourlyURL: srv.URL + "/hourly", TenMinURL: srv.URL + "/tenmin", APIKey: "k"}
	now := time.Date(2026, 10, 23, 12, 5, 0, 0, pipeline.Taipei)
	ctx := context.Background()
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsFetched != 5 || res.RowsInserted != 5 {
		t.Errorf("result %+v", res)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_obs_current WHERE observed_at = $1`,
		time.Date(2026, 10, 23, 12, 0, 0, 0, pipeline.Taipei)); n != 5 {
		t.Errorf("current at 12:00 = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.weather_station WHERE kind = 'manned'`); n != 1 {
		t.Errorf("manned stations = %d", n)
	}

	srv.Set("/hourly", "testdata/obs_10min.json")
	if _, err := p.Run(ctx, srv.Env(db, now)); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_obs_history`); n != 5 {
		t.Errorf("history rows = %d", n)
	}
}

func TestParseForecast(t *testing.T) {
	list, err := ParseForecast(read(t, "testdata/forecast.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 9 {
		t.Fatalf("periods = %d", len(list))
	}
	f := list[0]
	if f.Weather == "" || f.WeatherCode == nil || f.RainChance == nil || f.MinTemp == nil || f.MaxTemp == nil || f.Comfort == "" {
		t.Errorf("first period %+v", f)
	}
	if !f.StartsAt.Equal(time.Date(2026, 10, 23, 12, 0, 0, 0, pipeline.Taipei)) || !f.EndsAt.After(f.StartsAt) {
		t.Errorf("period %v–%v", f.StartsAt, f.EndsAt)
	}
}

func TestForecastRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/f": "testdata/forecast.json"})
	p := &ForecastPipeline{URL: srv.URL + "/f", APIKey: "k"}
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 11, 10, 0, 0, pipeline.Taipei)
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsInserted != 9 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if !res.LastObservedAt.Equal(now) {
		t.Errorf("observed %v", res.LastObservedAt)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT county_code) FROM data.county_forecast`); n != 3 {
		t.Errorf("counties = %d", n)
	}
	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}
}
