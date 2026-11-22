package power

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

func TestParseRegionCSV(t *testing.T) {
	rows, warnings, err := ParseRegionCSV(read(t, "testdata/region.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(rows) != 4 {
		t.Fatalf("rows=%d warnings=%v", len(rows), warnings)
	}
	want := time.Date(2026, 10, 22, 10, 10, 0, 0, pipeline.Taipei)
	var load float64
	for i, r := range rows {
		if r.Region != Regions[i] || !r.ObservedAt.Equal(want) {
			t.Errorf("row %d = %+v", i, r)
		}
		load += *r.LoadMW
	}
	if *rows[0].GenMW != 10136 || *rows[0].LoadMW != 12264 {
		t.Errorf("北部 = %v / %v MW", *rows[0].GenMW, *rows[0].LoadMW)
	}
	if load != 34267 {
		t.Errorf("total load = %v MW, want 34267", load)
	}
}

func TestParseRegionCSVWarnings(t *testing.T) {
	body := "時間,區域,發電(萬瓩),用電(萬瓩)\n" +
		"20261022T101000,北部,1013.6,-\n" +
		"20261022T101000,離島,1,1\n" +
		"2026-10-22,中部,1,1\n"
	rows, warnings, err := ParseRegionCSV([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LoadMW != nil || len(warnings) != 2 {
		t.Errorf("rows=%+v warnings=%v", rows, warnings)
	}
	if _, _, err := ParseRegionCSV([]byte("時間,區域,發電(萬瓩)\n")); err == nil {
		t.Error("expected an error for a missing column")
	}
}

func TestParseROCTime(t *testing.T) {
	cases := map[string]time.Time{
		"115.10.22(四)10:10": time.Date(2026, 10, 22, 10, 10, 0, 0, pipeline.Taipei),
		"115.10.21":         time.Date(2026, 10, 21, 0, 0, 0, 0, pipeline.Taipei),
		"99.1.5 (二) 9:05":   time.Date(2010, 1, 5, 9, 5, 0, 0, pipeline.Taipei),
	}
	for in, want := range cases {
		if got, err := parseROCTime(in); err != nil || !got.Equal(want) {
			t.Errorf("parseROCTime(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "2026.10.22 02:22", "115.02.30", "115.10.22(四)25:00"} {
		if _, err := parseROCTime(bad); err == nil {
			t.Errorf("parseROCTime(%q) should fail", bad)
		}
	}
}

func TestParseLoadJSON(t *testing.T) {
	s, warnings, err := ParseLoadJSON(read(t, "testdata/load.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if !s.ObservedAt.Equal(time.Date(2026, 10, 22, 10, 10, 0, 0, pipeline.Taipei)) {
		t.Errorf("ObservedAt = %v", s.ObservedAt)
	}
	if *s.LoadMW != 34267 || *s.UtilPct != 79 || *s.ForeSupplyMW != 44970 || *s.ForeReservePct != 23.21 {
		t.Errorf("snapshot = %+v", s)
	}
	if s.ForeIndicator != "G" || s.ForePeakHours != "13:00-16:00" {
		t.Errorf("indicator %q hours %q", s.ForeIndicator, s.ForePeakHours)
	}
	y := s.Yesterday
	if y == nil || !y.Date.Equal(time.Date(2026, 10, 21, 0, 0, 0, 0, pipeline.Taipei)) || *y.PeakLoadMW != 38309 {
		t.Errorf("yesterday = %+v", y)
	}
}

func TestParseLoadJSONUnknownIndicator(t *testing.T) {
	body := `{"records":[{"curr_load":"3000"},{"publish_time":"115.10.22(四)10:20","fore_peak_resv_indicator":"Z"}]}`
	s, warnings, err := ParseLoadJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if s.ForeIndicator != "" || s.Yesterday != nil || len(warnings) != 1 {
		t.Errorf("snapshot = %+v, warnings = %v", s, warnings)
	}
	if _, _, err := ParseLoadJSON([]byte(`{"records":[{"curr_load":"3000"}]}`)); err == nil {
		t.Error("expected an error without publish_time")
	}
}

func TestRegionLoadAndPeakRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/region": "testdata/region.csv",
		"/load":   "testdata/load.json",
	})
	env := srv.Env(db, time.Now())
	ctx := context.Background()
	region := &RegionPipeline{URL: srv.URL + "/region"}
	load := &LoadPipeline{URL: srv.URL + "/load"}
	peak := &PeakPipeline{URL: srv.URL + "/load"}

	if res, err := region.Run(ctx, env); err != nil || res.RowsInserted != 4 {
		t.Fatalf("region: %+v, %v", res, err)
	}
	if res, err := load.Run(ctx, env); err != nil || res.RowsInserted != 1 {
		t.Fatalf("load: %+v, %v", res, err)
	}
	if res, err := peak.Run(ctx, env); err != nil || res.RowsInserted != 1 {
		t.Fatalf("peak: %+v, %v", res, err)
	}
	for _, p := range []pipeline.Pipeline{region, load, peak} {
		if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
			t.Errorf("%s again: %+v, %v", p.Dataset().ID, res, err)
		}
	}

	revised := t.TempDir() + "/load.json"
	b := strings.Replace(string(read(t, "testdata/load.json")), `"3830.9"`, `"3831.0"`, 1)
	if err := os.WriteFile(revised, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.Set("/load", revised)
	if res, err := peak.Run(ctx, env); err != nil || res.RowsInserted != 1 {
		t.Errorf("revised: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_peak_daily WHERE peak_load_mw = 38310`); n != 1 {
		t.Errorf("revised peak not stored")
	}
}
