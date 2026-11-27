package weather

import (
	"context"
	"testing"

	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseAdvisories(t *testing.T) {
	list, err := ParseAdvisories(read(t, "testdata/advisory_cold.json"))
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	a := list[0]
	if a.Event != "低溫" || !a.Effective.Equal(nov(22, 6, 35)) || !a.Expires.Equal(nov(22, 9, 0)) ||
		a.Level != "低溫黃色燈號" || a.Color != "黃色" || len(a.Areas) != 5 || a.Areas[0] != "新北市平溪區" ||
		a.Web == "" || a.Instruction == "" {
		t.Errorf("cold = %+v", a)
	}
	fog, err := ParseAdvisories(read(t, "testdata/advisory_fog.json"))
	if err != nil || len(fog) != 1 || fog[0].Event != "濃霧" || fog[0].Level != "黃色燈號" {
		t.Fatalf("fog = %+v, %v", fog, err)
	}
}

func TestParseHealth(t *testing.T) {
	towns, heat, warnings, err := ParseHealth(read(t, "testdata/health_heat.json"), "heat")
	if err != nil || len(warnings) != 0 || len(towns) != 2 || len(heat) != 6 {
		t.Fatalf("towns=%+v heat=%d warnings=%v err=%v", towns, len(heat), warnings, err)
	}
	if towns[0].County != "嘉義市" || towns[0].Name != "東區" || towns[0].Geocode != "10020010" {
		t.Errorf("town = %+v", towns[0])
	}
	h := heat[0]
	if !h.StartsAt.Equal(nov(28, 18, 0)) || !h.EndsAt.Equal(nov(28, 21, 0)) || *h.Index != 16 || h.Warning != "" {
		t.Errorf("heat = %+v", h)
	}
	_, cold, _, err := ParseHealth(read(t, "testdata/health_cold.json"), "cold")
	if err != nil || len(cold) != 6 || *cold[1].Index != 10 || cold[1].Warning != "注意" ||
		!cold[1].EndsAt.Equal(nov(30, 6, 0)) {
		t.Fatalf("cold = %+v, %v", cold, err)
	}
	_, diff, _, err := ParseHealth(read(t, "testdata/health_diff.json"), "temp_diff")
	if err != nil || len(diff) != 6 || diff[0].Index != nil || *diff[0].MaxTemp != 24 || *diff[0].MinTemp != 15 ||
		diff[0].Warning != "警戒" {
		t.Fatalf("temp diff = %+v, %v", diff, err)
	}
	if _, _, _, err := ParseHealth(read(t, "testdata/health_diff.json"), "nope"); err == nil {
		t.Error("unknown kind should fail")
	}
}

func TestParseClimate(t *testing.T) {
	normals, err := ParseNormals(read(t, "testdata/climate_normal.json"))
	if err != nil || len(normals) != 24 {
		t.Fatalf("normals=%d err=%v", len(normals), err)
	}
	n := normals[10]
	if n.StationID != "466880" || n.Name != "板橋" || n.Month != 11 || *n.TempMean != 21.9 || *n.TempMax != 25.1 ||
		*n.TempMin != 19.4 || *n.RainMM != 101.8 || *n.RainDays != 12.4 || *n.StartYear != 2002 {
		t.Errorf("November = %+v", n)
	}
	rain, err := ParseDailyRain(read(t, "testdata/rain_daily.json"))
	if err != nil || len(rain) != 10 {
		t.Fatalf("rain=%d err=%v", len(rain), err)
	}
	if *rain[0].MM != 20.5 || *rain[5].MM != 0 || rain[6].MM != nil || rain[5].StationID != "466900" {
		t.Errorf("rain = %+v %+v %+v", rain[0], rain[5], rain[6])
	}
}

func TestCWAHealthRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/cold": "testdata/advisory_cold.json",
		"/fog":  "testdata/advisory_fog.json",
		"/heat": "testdata/health_heat.json",
		"/hc":   "testdata/health_cold.json",
		"/diff": "testdata/health_diff.json",
		"/norm": "testdata/climate_normal.json",
		"/rain": "testdata/rain_daily.json",
	})
	ctx := context.Background()
	env := srv.Env(db, nov(27, 7, 0))

	adv := &AdvisoryPipeline{URLs: []string{srv.URL + "/cold", srv.URL + "/fog"}, APIKey: "k"}
	res, err := adv.Run(ctx, env)
	if err != nil || res.RowsInserted != 2 || len(res.Warnings) != 1 {
		t.Fatalf("advisory: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_advisory_area WHERE event = '低溫'`); n != 4 {
		t.Errorf("cold areas = %d", n)
	}
	if n := pipelinetest.Count(t, db, `
		SELECT count(*) FROM data.weather_advisory_area WHERE event = '低溫' AND county_code = '65000' AND town = '平溪區'`); n != 1 {
		t.Errorf("township not split from county name")
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_advisory_area WHERE event = '濃霧' AND town = ''`); n == 0 {
		t.Error("county-wide fog areas missing")
	}
	if res, err := adv.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("advisory again: %+v, %v", res, err)
	}

	health := &HealthPipeline{URLs: map[string]string{"heat": srv.URL + "/heat", "cold": srv.URL + "/hc",
		"temp_diff": srv.URL + "/diff"}, APIKey: "k"}
	if res, err := health.Run(ctx, env); err != nil || res.RowsInserted != 18 {
		t.Fatalf("health: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.forecast_town`); n != 6 {
		t.Errorf("towns = %d", n)
	}
	if res, err := health.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("health again: %+v, %v", res, err)
	}

	climate := &ClimatePipeline{NormalURL: srv.URL + "/norm", RainURL: srv.URL + "/rain", APIKey: "k"}
	if res, err := climate.Run(ctx, env); err != nil || res.RowsInserted != 34 {
		t.Fatalf("climate: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.rain_daily WHERE rain_mm IS NULL`); n != 1 {
		t.Errorf("missing rain values = %d", n)
	}
}
