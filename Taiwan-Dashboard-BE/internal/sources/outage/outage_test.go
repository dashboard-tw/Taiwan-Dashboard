package outage

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

func taipei(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, pipeline.Taipei)
}

func TestParseTWC(t *testing.T) {
	outages, warnings, err := ParseTWC(read(t, "testdata/twc.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outages) != 5 || len(warnings) != 3 {
		t.Fatalf("outages=%d warnings=%v", len(outages), warnings)
	}
	o := outages[1]
	if o.CaseID != "202610220001" || o.County != "南投縣" || o.District != "南投市" || o.Kind != "非計畫性" ||
		*o.Households != 226 || !o.StartsAt.Equal(taipei(2026, 10, 22, 9, 30)) || !o.EndsAt.Equal(taipei(2026, 10, 22, 16, 0)) {
		t.Errorf("case = %+v", o)
	}
	if o.Reason != "南投市中興路146巷17弄口漏水搶修。" || o.LowPressureArea != "" {
		t.Errorf("reason %q, low pressure %q", o.Reason, o.LowPressureArea)
	}
	if outages[2].LowPressureArea == "" || outages[2].LowPressureReason == "" {
		t.Errorf("low pressure missing: %+v", outages[2])
	}
	if n := outages[4]; n.Households != nil || n.Area != n.District {
		t.Errorf("no households: %+v", n)
	}
}

func TestParseTaipei(t *testing.T) {
	outages, warnings, err := ParseTaipei(read(t, "testdata/taipei.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outages) != 5 || len(warnings) != 2 {
		t.Fatalf("outages=%d warnings=%v", len(outages), warnings)
	}
	o := outages[0]
	if o.CaseID != "11538035B13S0006" || o.DailyFrom != "10:30" || o.DailyTo != "16:30" ||
		!o.StartsAt.Equal(taipei(2026, 10, 26, 10, 30)) || !o.EndsAt.Equal(taipei(2026, 10, 31, 16, 30)) {
		t.Errorf("first = %+v", o)
	}
	if o.PublishedOn == nil || !o.PublishedOn.Equal(taipei(2026, 10, 22, 0, 0)) || !strings.HasPrefix(o.URL, "https://") {
		t.Errorf("published %v url %q", o.PublishedOn, o.URL)
	}
	if one := outages[3]; !one.StartsAt.Equal(taipei(2026, 10, 23, 9, 0)) || !one.EndsAt.Equal(taipei(2026, 10, 23, 17, 0)) {
		t.Errorf("one day = %+v", one)
	}
	if all := outages[4]; all.DailyFrom != "" || !all.StartsAt.Equal(taipei(2026, 10, 24, 0, 0)) || !all.EndsAt.Equal(taipei(2026, 10, 26, 0, 0)) {
		t.Errorf("whole days = %+v", all)
	}
}

func TestParsePowerKML(t *testing.T) {
	outages, warnings, err := ParsePowerKML(read(t, "testdata/power.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outages) != 2 || len(warnings) != 2 {
		t.Fatalf("outages=%+v warnings=%v", outages, warnings)
	}
	o := outages[0]
	if o.District != "宜蘭縣蘇澳鎮" || *o.Households != 1250 || o.ExpectedRestore != "今日18時前" || o.UpdatedAt != "2026/10/22 09:00" {
		t.Errorf("first = %+v", o)
	}
	if o := outages[1]; o.District != "花蓮縣秀林鄉" || *o.Households != 80 {
		t.Errorf("second = %+v", o)
	}
	for _, o := range outages {
		if strings.Contains(o.Info+o.District+o.Area+o.ExpectedRestore, "00-0000000") ||
			strings.Contains(o.Info, "測試聯絡人") {
			t.Errorf("contact details leaked into %+v", o)
		}
	}

	empty, warnings, err := ParsePowerKML(read(t, "testdata/power_empty.xml"))
	if err != nil || len(empty) != 0 || len(warnings) != 0 {
		t.Errorf("empty file: %v %v %v", empty, warnings, err)
	}
	if _, _, err := ParsePowerKML([]byte("<html><body>404</body></html>")); err == nil {
		t.Error("an HTML error page should fail")
	}
}

func TestWaterRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/twc":    "testdata/twc.json",
		"/taipei": "testdata/taipei.json",
	})
	ctx := context.Background()
	now := taipei(2026, 10, 22, 10, 30)
	twc := &WaterPipeline{Source: "twc", URL: srv.URL + "/twc"}
	tp := &WaterPipeline{Source: "taipei", URL: srv.URL + "/taipei"}

	if res, err := twc.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 5 {
		t.Fatalf("twc: %+v, %v", res, err)
	}
	if res, err := tp.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 5 {
		t.Fatalf("taipei: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.water_outage WHERE county_code IS NULL`); n != 0 {
		t.Errorf("%d outages without a county", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.water_outage WHERE source = 'taipei' AND county_code = '65000'`); n != 5 {
		t.Errorf("Taipei Water outages in 新北市 = %d, want 5", n)
	}

	later := now.Add(30 * time.Minute)
	if res, err := twc.Run(ctx, srv.Env(db, later)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
	var first, last time.Time
	if err := db.QueryRow(`SELECT first_seen_at, last_seen_at FROM data.water_outage WHERE case_id = '202610220001'`).Scan(&first, &last); err != nil {
		t.Fatal(err)
	}
	if !first.Equal(now) || !last.Equal(later) {
		t.Errorf("first %v last %v", first, last)
	}
}

func TestPowerRunReplacesCurrent(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/power": "testdata/power.xml"})
	ctx := context.Background()
	p := &PowerPipeline{URL: srv.URL + "/power"}
	now := taipei(2026, 10, 22, 9, 20)

	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 2 {
		t.Fatalf("first run: %+v, %v", res, err)
	}
	var observed time.Time
	var county string
	if err := db.QueryRow(`SELECT observed_at, county_code FROM data.power_outage_current WHERE seq = 1`).Scan(&observed, &county); err != nil {
		t.Fatal(err)
	}
	if !observed.Equal(taipei(2026, 10, 22, 9, 0)) || county != "10002" {
		t.Errorf("observed %v county %s", observed, county)
	}

	srv.Set("/power", "testdata/power_empty.xml")
	if res, err := p.Run(ctx, srv.Env(db, now.Add(time.Hour))); err != nil || res.RowsInserted != 0 {
		t.Fatalf("empty run: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_outage_current`); n != 0 {
		t.Errorf("current still has %d outages", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.power_outage_history`); n != 2 {
		t.Errorf("history = %d, want 2", n)
	}
}
