package weather

import (
	"context"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func nov(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, pipeline.Taipei) }

func TestTownForecastIDs(t *testing.T) {
	short, week := TownForecastIDs(false), TownForecastIDs(true)
	if len(short) != 5 || short[0] != "F-D0047-001,F-D0047-005,F-D0047-009,F-D0047-013,F-D0047-017" ||
		short[4] != "F-D0047-081,F-D0047-085" {
		t.Errorf("3-day ids = %q", short)
	}
	all := strings.Join(week, ",")
	if !strings.HasPrefix(all, "F-D0047-003,") || !strings.HasSuffix(all, ",F-D0047-087") || strings.Count(all, ",") != 21 {
		t.Errorf("week ids = %q", week)
	}
}

func TestParseTownForecast(t *testing.T) {
	towns, periods, warnings, err := ParseTownForecast(read(t, "testdata/town_3day.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(towns) != 2 || len(warnings) != 1 || len(periods) != 8 {
		t.Fatalf("towns=%+v periods=%d warnings=%v", towns, len(periods), warnings)
	}
	if towns[0].Geocode != "09007000" || towns[0].County != "台灣" || towns[0].Name != "連江縣" {
		t.Errorf("first town = %+v", towns[0])
	}
	p := periods[0]
	if !p.StartsAt.Equal(nov(27, 6, 0)) || !p.EndsAt.Equal(nov(27, 9, 0)) || p.Weather != "晴" || *p.WeatherCode != 1 ||
		*p.RainChance != 10 || *p.Temp != 16 || *p.Apparent != 14 || *p.Humidity != 95 || *p.WindMS != 5 ||
		*p.Beaufort != 3 || p.WindDir != "偏北風" || p.Comfort != "稍有寒意" {
		t.Errorf("first period = %+v", p)
	}
	if periods[1].Temp == nil || periods[2].Temp != nil {
		t.Errorf("temperature only where an hourly value exists: %v %v", periods[1].Temp, periods[2].Temp)
	}

	_, week, _, err := ParseTownForecast(read(t, "testdata/town_week.json"))
	if err != nil || len(week) != 8 {
		t.Fatalf("week periods=%d err=%v", len(week), err)
	}
	w := week[0]
	if !w.EndsAt.Equal(nov(27, 18, 0)) || *w.MaxTemp != 20 || *w.MinTemp != 16 || *w.MaxApparent != 20 ||
		*w.MinApparent != 14 || *w.Humidity != 88 || *w.UVI != 4 || w.UVLevel != "中量級" || *w.RainChance != 10 ||
		!strings.HasPrefix(w.Description, "晴時多雲") {
		t.Errorf("first week period = %+v", w)
	}
	if week[1].UVI != nil {
		t.Errorf("night period has a UV index: %v", *week[1].UVI)
	}
}

func TestParseTides(t *testing.T) {
	stations, tides, warnings, err := ParseTides(read(t, "testdata/tide.json"))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if len(stations) != 2 || len(tides) != 16 || stations[1].ID != "N00900" {
		t.Fatalf("stations=%+v tides=%d", stations, len(tides))
	}
	f := tides[0]
	if f.StationID != "10015050" || !f.At.Equal(nov(27, 1, 45)) || f.Tide != "乾潮" || *f.HeightCM != -84 || f.Range != "大" {
		t.Errorf("first tide = %+v", f)
	}
}

func TestParseSunMoon(t *testing.T) {
	sun, err := ParseSunMoon(read(t, "testdata/sun.json"), false)
	if err != nil || len(sun) != 4 {
		t.Fatalf("sun=%d err=%v", len(sun), err)
	}
	if sun[0].County != "嘉義縣" || sun[0].Values["sunrise"] != "06:41" || sun[0].Values["sunrise_az"] != "115" ||
		sun[0].Values["sun_transit_alt"] != "44S" || sun[0].Values["civil_dusk"] != "17:49" {
		t.Errorf("sun = %+v", sun[0])
	}
	moon, err := ParseSunMoon(read(t, "testdata/moon.json"), true)
	if err != nil || len(moon) != 4 || moon[0].Values["moonset"] != "18:40" || moon[0].Values["sunrise"] != nil {
		t.Fatalf("moon=%+v err=%v", moon, err)
	}
	if sunValue("moonrise", "") != nil || sunValue("moonrise", "--") != nil || sunValue("moonrise_az", "400") != nil {
		t.Error("blank or invalid values should be NULL")
	}
}

func TestCWAMoreRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/town": "testdata/town_3day.json",
		"/week": "testdata/town_week.json",
		"/tide": "testdata/tide.json",
		"/sun":  "testdata/sun.json",
		"/moon": "testdata/moon.json",
	})
	minTowns = 1
	t.Cleanup(func() { minTowns = 300 })
	ctx := context.Background()
	env := srv.Env(db, nov(27, 7, 0))

	// 測試資料的 LocationsName 是「台灣」，改成縣市讓代碼對得上
	if _, err := db.Exec(`UPDATE ref.county SET aliases = aliases || '{台灣}' WHERE code = '09007'`); err != nil {
		t.Fatal(err)
	}
	town := &TownForecastPipeline{URL: srv.URL + "/town", APIKey: "k"}
	if res, err := town.Run(ctx, env); err != nil || res.RowsInserted != 8 {
		t.Fatalf("town: %+v, %v", res, err)
	}
	if res, err := town.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("town again: %+v, %v", res, err)
	}
	week := &TownForecastPipeline{Week: true, URL: srv.URL + "/week", APIKey: "k"}
	if res, err := week.Run(ctx, env); err != nil || res.RowsInserted != 8 {
		t.Fatalf("week: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.forecast_town WHERE county_code = '09007'`); n != 2 {
		t.Errorf("towns = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.town_forecast_week WHERE uvi = 4 AND uv_level = '中量級'`); n == 0 {
		t.Error("UV index not stored")
	}

	tide := &TidePipeline{URL: srv.URL + "/tide", APIKey: "k"}
	if res, err := tide.Run(ctx, env); err != nil || res.RowsInserted != 16 {
		t.Fatalf("tide: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.tide_station WHERE county_code IS NOT NULL`); n != 2 {
		t.Errorf("tide stations with a county = %d", n)
	}
	// 兩天以前的潮汐會被清掉
	if res, err := tide.Run(ctx, srv.Env(db, nov(30, 3, 0))); err != nil || res.RowsInserted != 0 {
		t.Fatalf("tide later: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.tide_forecast`); n != 7 {
		t.Errorf("tides kept = %d", n)
	}

	sun := &SunMoonPipeline{SunURL: srv.URL + "/sun", MoonURL: srv.URL + "/moon", APIKey: "k"}
	if res, err := sun.Run(ctx, env); err != nil || res.RowsInserted != 8 {
		t.Fatalf("sun/moon: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `
		SELECT count(*) FROM data.sun_moon
		 WHERE sunrise IS NOT NULL AND moonrise IS NOT NULL AND sunset_az = 245 AND date = '2025-01-01'`); n != 2 {
		t.Errorf("merged sun and moon rows = %d", n)
	}
	if res, err := sun.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("sun/moon again: %+v, %v", res, err)
	}
}
