package weather

import (
	"context"
	"os"
	"strings"
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

func at(h, m int) time.Time { return time.Date(2026, 10, 22, h, m, 0, 0, pipeline.Taipei) }

func TestParseTyphoons(t *testing.T) {
	cyclones, warnings, err := ParseTyphoons(read(t, "testdata/typhoon.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cyclones) != 1 || len(warnings) != 0 {
		t.Fatalf("cyclones=%d warnings=%v", len(cyclones), warnings)
	}
	c := cyclones[0]
	if c.ID() != "2026-33" || *c.TyNo != 29 || c.NameEn != "CHAMPI" || c.NameZh != "薔琵" || len(c.Fixes) != 36+8 {
		t.Errorf("cyclone = %+v (%d fixes)", c, len(c.Fixes))
	}
	var last, first Fix
	for _, f := range c.Fixes {
		if !f.Forecast {
			last = f
		} else if first.ValidAt.IsZero() {
			first = f
		}
	}
	if !last.ValidAt.Equal(at(11, 0)) || *last.Pressure != 925 || *last.MaxWind != 51 || *last.Radius7 != 320 || *last.Radius10 != 160 {
		t.Errorf("latest analysis = %+v", last)
	}
	if first.ForecastHour != 6 || !first.ValidAt.Equal(at(17, 0)) || *first.Radius70 != 30 {
		t.Errorf("first forecast = %+v", first)
	}
	if lastFc := c.Fixes[len(c.Fixes)-1]; lastFc.Note != "變性為溫帶氣旋" || lastFc.MaxWind != nil {
		t.Errorf("last forecast = %+v", lastFc)
	}

	none, _, err := ParseTyphoons(read(t, "testdata/typhoon_none.json"))
	if err != nil || len(none) != 0 {
		t.Errorf("no cyclone: %v %v", none, err)
	}
}

func TestParseTyphoonWarnings(t *testing.T) {
	list, warnings, err := ParseTyphoonWarnings(read(t, "testdata/typhoon_warning.json"))
	if err != nil || len(list) != 1 || len(warnings) != 0 {
		t.Fatalf("list=%+v warnings=%v err=%v", list, warnings, err)
	}
	w := list[0]
	if w.TyNo != 29 || *w.WarningNo != 5 || w.Alert != "海上陸上颱風警報" || w.NameZh != "薔琵" ||
		!w.EffectiveAt.Equal(at(11, 30)) || len(w.Sections) != 7 || len(w.Areas) != 10 {
		t.Errorf("warning = %+v", w)
	}
	if w.Sections[0].Title != "命名與位置" || !strings.Contains(w.Sections[0].Value, "鵝鑾鼻") {
		t.Errorf("first section = %+v", w.Sections[0])
	}
	none, _, err := ParseTyphoonWarnings(read(t, "testdata/typhoon_warning_none.json"))
	if err != nil || len(none) != 0 {
		t.Errorf("no warning: %v %v", none, err)
	}
}

func TestParseWarnings(t *testing.T) {
	list, warnings, err := ParseWarnings(read(t, "testdata/warning.json"))
	if err != nil || len(warnings) != 0 || len(list) != 26 {
		t.Fatalf("list=%d warnings=%v err=%v", len(list), warnings, err)
	}
	w := list[0]
	if w.County != "宜蘭縣" || w.Label() != "海上陸上颱風警報" || !w.StartsAt.Equal(at(11, 30)) || !w.EndsAt.Equal(at(15, 30)) {
		t.Errorf("first = %+v", w)
	}
}

func TestParseRain(t *testing.T) {
	obs, warnings, err := ParseRain(read(t, "testdata/rain.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 9 || len(warnings) != 1 {
		t.Fatalf("obs=%d warnings=%v", len(obs), warnings)
	}
	o := obs[0]
	if o.StationID != "01AG10" || o.CountyCode != "65000" || *o.Values["Past24hr"] != 64 || *o.Values["Past1hr"] != 1 || o.Lat == nil {
		t.Errorf("first = %+v", o)
	}
	odd := obs[8]
	if odd.Values["Past24hr"] != nil || odd.Values["Past1hr"] != nil || odd.Values["Now"] == nil {
		t.Errorf("odd = %+v", odd.Values)
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/ty":   "testdata/typhoon.json",
		"/tyw":  "testdata/typhoon_warning.json",
		"/w":    "testdata/warning.json",
		"/rain": "testdata/rain.json",
	})
	ctx := context.Background()
	env := srv.Env(db, at(11, 50))
	ty := &TyphoonPipeline{URL: srv.URL + "/ty", APIKey: "k"}
	tyw := &TyphoonWarningPipeline{URL: srv.URL + "/tyw", APIKey: "k"}
	warn := &WarningPipeline{URL: srv.URL + "/w", APIKey: "k"}
	rain := &RainPipeline{URL: srv.URL + "/rain", APIKey: "k"}

	if res, err := ty.Run(ctx, env); err != nil || res.RowsInserted != 44 {
		t.Fatalf("typhoon: %+v, %v", res, err)
	}
	if res, err := ty.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("typhoon again: %+v, %v", res, err)
	}
	if res, err := tyw.Run(ctx, env); err != nil || res.RowsInserted != 1 {
		t.Fatalf("typhoon warning: %+v, %v", res, err)
	}
	if res, err := warn.Run(ctx, env); err != nil || res.RowsInserted != 26 {
		t.Fatalf("warnings: %+v, %v", res, err)
	}
	if res, err := warn.Run(ctx, srv.Env(db, at(12, 0))); err != nil || res.RowsInserted != 0 {
		t.Errorf("warnings again: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_warning WHERE last_seen_at = '2026-10-22 12:00+08'`); n != 26 {
		t.Errorf("last_seen_at not moved on: %d", n)
	}
	if res, err := rain.Run(ctx, env); err != nil || res.RowsInserted != 9 {
		t.Fatalf("rain: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.rain_obs_current`); n != 9 {
		t.Errorf("current rain = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.rain_station WHERE geom IS NOT NULL AND county_code IS NOT NULL`); n != 9 {
		t.Errorf("located stations = %d", n)
	}

	if n := pipelinetest.Count(t, db, `
		SELECT count(*) FROM data.typhoon_fix
		 WHERE kind = 'forecast' AND radius70_km IS NOT NULL
		   AND ST_IsValid(ST_Buffer(geom::geography, radius70_km * 1000)::geometry)`); n != 8 {
		t.Errorf("valid forecast circles = %d", n)
	}
}
