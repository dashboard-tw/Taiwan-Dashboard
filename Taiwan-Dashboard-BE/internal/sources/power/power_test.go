package power

import (
	"context"
	"math"
	"os"
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

func TestParseGen(t *testing.T) {
	snap, warnings, err := ParseGen(read(t, "testdata/gen.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if want := time.Date(2026, 10, 20, 16, 50, 0, 0, pipeline.Taipei); !snap.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %v", snap.ObservedAt)
	}
	if len(snap.Units) != 202 {
		t.Errorf("units = %d, want 202", len(snap.Units))
	}
	if len(snap.Subtotals) != 11 {
		t.Errorf("subtotals = %d, want 11", len(snap.Subtotals))
	}
	if w := CheckSubtotals(snap); len(w) != 0 {
		t.Errorf("our sums disagree with Taipower's subtotals: %v", w)
	}

	seen := map[string]bool{}
	for _, u := range snap.Units {
		seen[u.FuelType] = true
		if u.FuelType == "other" {
			t.Errorf("unit %s has unknown type %q", u.Name, u.RawType)
		}
		if u.Name == "大潭CC#1" && (u.Plant != "大潭" || u.FuelType != "gas" || u.IsPrivate) {
			t.Errorf("大潭CC#1 = %+v", u)
		}
	}
	for _, f := range []string{"gas", "coal", "solar", "wind", "hydro", "storage", "storage_load"} {
		if !seen[f] {
			t.Errorf("no units of fuel type %s", f)
		}
	}
}

func TestAggregateSharesSumTo100(t *testing.T) {
	snap, _, err := ParseGen(read(t, "testdata/gen.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, ft := range Aggregate(snap.Units) {
		if ft.SharePct != nil {
			sum += *ft.SharePct
		}
		if want := snap.Subtotals["燃氣"] + snap.Subtotals["民營電廠-燃氣"]; ft.FuelType == "gas" && math.Abs(ft.NetMW-want) > 0.5 {
			t.Errorf("gas = %.1f MW, want %.1f", ft.NetMW, want)
		}
	}
	if math.Abs(sum-100) > 0.05 {
		t.Errorf("shares sum to %.3f, want 100", sum)
	}
}

func TestPlantName(t *testing.T) {
	cases := map[string]string{
		"大潭CC#1":  "大潭",
		"台中#10":   "台中",
		"通霄GT#9":  "通霄",
		"電池(註16)": "電池",
		"彰工光":     "彰工光",
	}
	for in, want := range cases {
		if got := PlantName(in); got != want {
			t.Errorf("PlantName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDailyCSV(t *testing.T) {
	rows, warnings, err := ParseDailyCSV(read(t, "testdata/daily.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(rows) != 3 {
		t.Fatalf("rows=%d warnings=%v", len(rows), warnings)
	}
	r := rows[0]
	if *r.NetPeakCapMW != 31064 || *r.PeakLoadMW != 27994 || *r.ReserveMW != 3070 || *r.ReservePct != 10.97 {
		t.Errorf("first row = %+v", r)
	}
	if *r.IndustrialGWh != 327.26 {
		t.Errorf("industrial = %v", *r.IndustrialGWh)
	}
}

func TestParseForecastCSV(t *testing.T) {
	rows, _, err := ParseForecastCSV(read(t, "testdata/forecast.csv"))
	if err != nil || len(rows) != 6 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if *rows[0].ReservePct != 18.51 || *rows[0].NetPeakCapMW != 42911 {
		t.Errorf("first row = %+v", rows[0])
	}
}

func TestParseCSVRejectsMissingColumn(t *testing.T) {
	if _, _, err := ParseForecastCSV([]byte("日期,其他\n20261021,1\n")); err == nil {
		t.Error("expected an error for a missing column")
	}
}

func TestGenRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/gen": "testdata/gen.json"})
	p := &GenPipeline{URL: srv.URL + "/gen"}
	env := srv.Env(db, time.Now())
	ctx := context.Background()

	res, err := p.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 202 {
		t.Errorf("inserted %d, want 202", res.RowsInserted)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.power_unit`); n != 202 {
		t.Errorf("units = %d, want 202 (storage and storage-load units share names)", n)
	}
	fuels := pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_gen_by_fuel_current`)
	if fuels == 0 || fuels != pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_gen_by_fuel_history`) {
		t.Errorf("current fuel rows = %d, should match the single history snapshot", fuels)
	}

	if res, err = p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: inserted %d, err %v", res.RowsInserted, err)
	}
}

func TestDailyRunUpdatesOnlyChangedRows(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/daily": "testdata/daily.csv"})
	p := &DailyPipeline{URL: srv.URL + "/daily"}
	env := srv.Env(db, time.Now())

	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("first run: %+v, %v", res, err)
	}
	if res, err := p.Run(context.Background(), env); err != nil || res.RowsInserted != 0 {
		t.Errorf("unchanged file should write nothing: %+v, %v", res, err)
	}
}

func TestForecastStoresEachEditionOnce(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/fc": "testdata/forecast.csv"})
	p := &ForecastPipeline{URL: srv.URL + "/fc"}
	ctx := context.Background()
	day1 := time.Date(2026, 10, 20, 6, 10, 0, 0, pipeline.Taipei)

	if res, err := p.Run(ctx, srv.Env(db, day1)); err != nil || res.RowsInserted != 6 {
		t.Fatalf("first edition: %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, srv.Env(db, day1.AddDate(0, 0, 1))); err != nil || res.RowsInserted != 0 {
		t.Errorf("same edition stored again: %+v, %v", res, err)
	}

	revised := t.TempDir() + "/forecast.csv"
	if err := os.WriteFile(revised, []byte("日期,預估淨尖峰供電能力(萬瓩),預估瞬時尖峰負載(萬瓩),預估尖峰備轉容量(萬瓩),預估尖峰備轉容量率(%)\n20261101,4000,3500,500,14.29\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.Set("/fc", revised)
	if res, err := p.Run(ctx, srv.Env(db, day1.AddDate(0, 0, 2))); err != nil || res.RowsInserted != 1 {
		t.Fatalf("revised edition: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(DISTINCT published_on) FROM data.power_supply_forecast`); n != 2 {
		t.Errorf("editions = %d, want 2", n)
	}
}
