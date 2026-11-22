package aqi

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParse(t *testing.T) {
	body, err := os.ReadFile("testdata/aqi.json")
	if err != nil {
		t.Fatal(err)
	}
	obs, warnings, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(obs) != 3 {
		t.Fatalf("got %d observations, want 3", len(obs))
	}

	guting := obs[0]
	if guting.SiteID != 13 || guting.SiteName != "古亭" || *guting.AQI != 48 {
		t.Errorf("古亭 = %+v", guting)
	}
	if want := time.Date(2026, 10, 20, 15, 0, 0, 0, pipeline.Taipei); !guting.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %v, want %v", guting.ObservedAt, want)
	}
	if v := guting.Values["pm25_ug_m3"]; v == nil || *v != 12 {
		t.Errorf("pm25 = %v", v)
	}

	matsu := obs[2]
	if matsu.AQI != nil || matsu.Values["pm25_ug_m3"] != nil || matsu.Values["pm10_ug_m3"] != nil {
		t.Errorf("馬祖 should have no readings: %+v", matsu)
	}
}

func TestParseAcceptsEveryResponseShape(t *testing.T) {
	rec := `{"siteid":"1","sitename":"基隆","county":"基隆市","aqi":"30","publishtime":"2026/10/20 15:00:00"}`
	for name, body := range map[string]string{
		"top-level list": `[` + rec + `]`,
		"records":        `{"records":[` + rec + `]}`,
		"result.records": `{"result":{"records":[` + rec + `]}}`,
		"numeric values": `[{"siteid":1,"sitename":"基隆","county":"基隆市","aqi":30,"publishtime":"2026/10/20 15:00:00"}]`,
	} {
		obs, _, err := Parse([]byte(body))
		if err != nil || len(obs) != 1 || obs[0].SiteID != 1 {
			t.Errorf("%s: obs=%v err=%v", name, obs, err)
		}
	}
	if _, _, err := Parse([]byte(`{"message":"invalid api key"}`)); err == nil {
		t.Error("expected an error for a response without records")
	}
}

func TestParseSkipsBadRecordsWithWarning(t *testing.T) {
	body := `[{"siteid":"x","sitename":"壞資料","publishtime":"2026/10/20 15:00:00"},
	          {"siteid":"2","sitename":"汐止","county":"新北市","aqi":"40","publishtime":"2026/10/20 15:00:00"}]`
	obs, warnings, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || len(warnings) != 1 {
		t.Errorf("obs=%d warnings=%v", len(obs), warnings)
	}
}

func TestRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/aqi": "testdata/aqi.json"})
	p := &Pipeline{URL: srv.URL + "/aqi", APIKey: "test-key"}
	env := srv.Env(db, time.Now())
	ctx := context.Background()

	res, err := p.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsFetched != 3 || res.RowsInserted != 3 {
		t.Errorf("first run: %+v", res)
	}

	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.aqi_site WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 3 {
		t.Errorf("sites with county and geom = %d, want 3", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.aqi_site WHERE site_id = 45 AND county_code = '67000'`); n != 1 {
		t.Error("安南 should be in 臺南市 (67000)")
	}

	res, err = p.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 0 {
		t.Errorf("second run inserted %d rows, want 0", res.RowsInserted)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.aqi_obs_history`); n != 3 {
		t.Errorf("history rows = %d, want 3", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.aqi_obs_current`); n != 3 {
		t.Errorf("current rows = %d, want 3", n)
	}
}

func TestRunRequiresAPIKey(t *testing.T) {
	_, err := (&Pipeline{URL: "http://unused"}).Run(context.Background(), pipeline.Env{})
	if !errors.Is(err, pipeline.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRunNeverLeaksAPIKey(t *testing.T) {
	srv := pipelinetest.NewServer(t, nil)
	srv.Fail("/aqi", 403)
	p := &Pipeline{URL: srv.URL + "/aqi", APIKey: "super-secret-key"}

	_, err := p.Run(context.Background(), srv.Env(nil, time.Now()))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatalf("HTTP error leaks the API key: %v", err)
	}

	p.URL = "http://127.0.0.1:1/aqi"
	_, err = p.Run(context.Background(), srv.Env(nil, time.Now()))
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatalf("network error leaks the API key: %v", err)
	}
}
