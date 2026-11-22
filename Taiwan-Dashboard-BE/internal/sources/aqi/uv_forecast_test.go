package aqi

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseCoordinate(t *testing.T) {
	for in, want := range map[string]float64{
		"121,30,53":    121.514722,
		"25,07,60":     25.133333, // the source writes 60 seconds
		"120.85985238": 120.859852,
	} {
		lo, hi := 10.0, 123.0
		got, err := parseCoordinate(in, lo, hi)
		if err != nil || got == nil || math.Abs(*got-want) > 1e-6 {
			t.Errorf("%q = %v, %v; want %v", in, got, err, want)
		}
	}
	if v, err := parseCoordinate("", 10, 123); v != nil || err != nil {
		t.Errorf("empty: %v %v", v, err)
	}
	for _, bad := range []string{"130,00,00", "1,2", "a,b,c", "121,-5,0"} {
		if _, err := parseCoordinate(bad, 114, 123); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestParseUV(t *testing.T) {
	body, err := os.ReadFile("testdata/uv.json")
	if err != nil {
		t.Fatal(err)
	}
	obs, warnings, err := ParseUV(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 9 {
		t.Fatalf("got %d observations, want 9", len(obs))
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want the nameless record and the faulty reading", warnings)
	}
	byName := map[string]UVObservation{}
	for _, o := range obs {
		byName[o.SiteName] = o
	}
	if o := byName["日月潭"]; o.UVI != nil {
		t.Errorf("-999 should be no value, got %v", *o.UVI)
	}
	if o := byName["故障站"]; o.UVI != nil {
		t.Errorf("31 is impossible and should be no value, got %v", *o.UVI)
	}
	lulin := byName["鹿林山"]
	if lulin.Agency != "環境部" || lulin.Lng == nil || math.Abs(*lulin.Lng-120.859852) > 1e-6 {
		t.Errorf("鹿林山 = %+v", lulin)
	}
	if want := time.Date(2026, 10, 22, 2, 0, 0, 0, pipeline.Taipei); !lulin.ObservedAt.Equal(want) {
		t.Errorf("a time with seconds: %v, want %v", lulin.ObservedAt, want)
	}
}

func TestRunUV(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/uv": "testdata/uv.json"})
	p := &UVPipeline{URL: srv.URL + "/uv", APIKey: "test-key"}
	env := srv.Env(db, time.Now())
	ctx := context.Background()

	res, err := p.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 9 {
		t.Errorf("inserted %d, want 9", res.RowsInserted)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.uv_site WHERE geom IS NOT NULL`); n != 6 {
		t.Errorf("sites with a point = %d, want 6", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.uv_site WHERE name = '測試站' AND county_code IS NULL`); n != 1 {
		t.Error("an unknown county should leave county_code empty")
	}
	if !strings.Contains(strings.Join(res.Warnings, "；"), "火星市") {
		t.Errorf("warnings = %v, want the unknown county", res.Warnings)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.uv_site WHERE name = '臺北' AND county_code = '63000' AND agency = '中央氣象署'`); n != 1 {
		t.Error("臺北 should be in 63000, run by 中央氣象署")
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.uv_obs_current c JOIN ref.uv_site s USING (site_id)
		WHERE s.name = '臺北' AND c.observed_at = '2026-10-22 02:00+08'`); n != 1 {
		t.Error("臺北 current should be 02:00")
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: inserted %d, err %v", res.RowsInserted, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.uv_site`); n != 6 {
		t.Errorf("sites = %d after a second run, want 6", n)
	}
}

func TestRunForecast(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/f": "testdata/forecast.json"})
	p := &ForecastPipeline{URL: srv.URL + "/f", APIKey: "test-key"}
	env := srv.Env(db, time.Now())
	ctx := context.Background()

	res, err := p.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 18 {
		t.Errorf("inserted %d, want 18", res.RowsInserted)
	}
	if !strings.Contains(strings.Join(res.Warnings, "；"), "火星") {
		t.Errorf("warnings = %v, want the unknown area", res.Warnings)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT published_at) FROM data.aqi_forecast`); n != 2 {
		t.Errorf("publications = %d, want 2", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.aqi_forecast_text`); n != 2 {
		t.Errorf("texts = %d, want one per publication", n)
	}
	if !res.LastObservedAt.Equal(time.Date(2026, 10, 21, 22, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("last observed = %v, want the 22:00 publication", res.LastObservedAt)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: inserted %d, err %v", res.RowsInserted, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.county c
		WHERE NOT EXISTS (SELECT 1 FROM ref.aqi_area_county a WHERE a.county_code = c.code)`); n != 0 {
		t.Errorf("%d counties have no air quality area", n)
	}
}

func TestUVAndForecastNeedAPIKey(t *testing.T) {
	for _, p := range []pipeline.Pipeline{&UVPipeline{URL: "http://unused"}, &ForecastPipeline{URL: "http://unused"}} {
		if _, err := p.Run(context.Background(), pipeline.Env{}); !errors.Is(err, pipeline.ErrNotConfigured) {
			t.Errorf("%s: err = %v, want ErrNotConfigured", p.Dataset().ID, err)
		}
	}
}
