package alerts

import (
	"context"
	"os"
	"path/filepath"
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

const (
	debrisCAP  = "testdata/ardswc.gov.tw_debrisFlowLandSlide_202610231530.cap"
	sensorCAP  = "testdata/WRA_floodSensor_20261023162700_0000.cap"
	cancelCAP  = "testdata/WRA_floodSensor_20261023165700_0000.cap"
	roadCAP    = "testdata/THB-Bobe_633_23792_42700_20261023150127.cap"
	closureDoc = "testdata/closure.html"
)

func TestParseCAP(t *testing.T) {
	d, err := ParseCAP(read(t, debrisCAP), "debris_flow")
	if err != nil {
		t.Fatal(err)
	}
	if d.MsgType != "Alert" || d.Level != "黃色警戒" || d.Color != "黃色" || d.Expires == nil || len(d.Areas) != 3 {
		t.Errorf("debris %+v", d)
	}
	if got := countyCodes(d.Areas, map[string]bool{"10002": true}); len(got) != 1 || got[0] != "10002" {
		t.Errorf("counties %v", got)
	}

	s, err := ParseCAP(read(t, sensorCAP), "flood_sensor")
	if err != nil {
		t.Fatal(err)
	}
	if s.Lat == nil || *s.Lat < 24 || *s.Lon < 121 || s.Color != "黃色" || strings.Contains(s.Description, "​") {
		t.Errorf("sensor %+v", s)
	}

	c, err := ParseCAP(read(t, cancelCAP), "flood_sensor")
	if err != nil {
		t.Fatal(err)
	}
	if c.MsgType != "Cancel" || len(c.Supersedes) != 1 || c.Supersedes[0] != s.ID {
		t.Errorf("cancel %+v supersedes %v, want %s", c.MsgType, c.Supersedes, s.ID)
	}

	if _, err := ParseCAP([]byte(`<alert><identifier>x</identifier></alert>`), "flood"); err == nil {
		t.Error("an alert without info must fail")
	}
}

func TestClosureLevel(t *testing.T) {
	cases := map[string]int{
		"今天停止上班、停止上課。":           2,
		"今天照常上班、照常上課。":           0,
		"今天停止上課，照常上班。":           1,
		"今天下午停止上班、停止上課。":         1,
		"今天烏來區停止上班、停止上課，其他地區照常。": 1,
		"明天停止上班、停止上課。":           2,
		"明天已達停止上班及上課標準。":         2,
		"明天未達停止上班及上課標準。":         0,
	}
	for status, want := range cases {
		if got := ClosureLevel(status); got != want {
			t.Errorf("ClosureLevel(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestParseClosures(t *testing.T) {
	names := map[string]bool{"臺北市": true, "金門縣": true, "連江縣": true, "宜蘭縣": true}
	updated, list, err := ParseClosures(read(t, closureDoc), func(s string) bool { return names[s] })
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Equal(time.Date(2026, 10, 23, 16, 44, 4, 0, pipeline.Taipei)) {
		t.Errorf("updated %v", updated)
	}
	if len(list) != 4 {
		t.Fatalf("closures %+v", list)
	}
	for _, c := range list {
		want := 2
		if c.County == "金門縣" {
			want = 0
		}
		if c.Level != want {
			t.Errorf("%s: %q level %d", c.County, c.Status, c.Level)
		}
	}
	if _, list, err := ParseClosures([]byte("<html><body>無停班停課訊息</body></html>"), func(string) bool { return true }); err != nil || len(list) != 0 {
		t.Errorf("empty page: %v %v", list, err)
	}
}

func TestClosureRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/nds.html": closureDoc})
	p := &ClosurePipeline{URL: srv.URL + "/nds.html"}
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei)
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsInserted != 22 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.work_closure WHERE level = 2`); n != 21 {
		t.Errorf("closed counties = %d", n)
	}

	srv.Set("/nds.html", "testdata/closure_tomorrow.html")
	evening := time.Date(2026, 10, 23, 22, 50, 0, 0, pipeline.Taipei)
	if res, err := p.Run(ctx, srv.Env(db, evening)); err != nil || res.RowsFetched != 44 {
		t.Fatalf("evening run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.work_closure WHERE day = '2026-10-24'`); n != 22 {
		t.Errorf("tomorrow rows = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.work_closure WHERE day = '2026-10-24' AND level = 2`); n != 13 {
		t.Errorf("counties closed tomorrow = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.work_closure WHERE day = '2026-10-24' AND level = 1`); n != 0 {
		t.Errorf("「明天未達標準」 must not count as partly closed: %d", n)
	}
}

func TestDayLines(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, pipeline.Taipei)
	got := dayLines(`<FONT>今天停止上班、停止上課。</FONT><br><FONT>明天停止上課。</FONT><br><FONT>烏來區照常上班。</FONT>`, today)
	if len(got) != 2 || !got[0].Day.Equal(today) || got[0].Level != 2 ||
		!got[1].Day.Equal(today.AddDate(0, 0, 1)) || got[1].Status != "明天停止上課。 烏來區照常上班。" || got[1].Level != 1 {
		t.Errorf("lines %+v", got)
	}
}

func TestNCDRRun(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	srv := pipelinetest.NewServer(t, map[string]string{
		"/Capstorage/ARDSWC/2026/debrisFlow_cap/ardswc.gov.tw_debrisFlowLandSlide_202610231530.cap": debrisCAP,
		"/Capstorage/WRA/2026/FloodSensor/WRA_floodSensor_20261023162700_0000.cap":                  sensorCAP,
		"/Capstorage/WRA/2026/FloodSensor/WRA_floodSensor_20261023165700_0000.cap":                  cancelCAP,
		"/Capstorage/THB/2026/roadClose/THB-Bobe_633_23792_42700_20261023150127.cap":                roadCAP,
	})
	feed := strings.ReplaceAll(string(read(t, "testdata/feed.xml")), "{{HOST}}", srv.URL)
	feedPath := filepath.Join(dir, "feed.xml")
	if err := os.WriteFile(feedPath, []byte(feed), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.Set("/feed", feedPath)
	p := &NCDRPipeline{URL: srv.URL + "/feed", CAPHost: srv.URL + "/"}
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei)
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsFetched != 4 || res.RowsInserted != 4 || len(res.Warnings) != 0 {
		t.Fatalf("run %+v, %v", res, err)
	}
	if _, err := db.Exec(`UPDATE data.disaster_alert SET expires_at = now() + interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.disaster_alert_active`); n != 2 {
		t.Errorf("active = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.disaster_alert WHERE geom IS NOT NULL`); n != 3 {
		t.Errorf("with points = %d", n)
	}
	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}

	p.CAPHost = "https://alerts.ncdr.nat.gov.tw/"
	if _, err := db.Exec(`DELETE FROM data.disaster_alert`); err != nil {
		t.Fatal(err)
	}
	if res, _ := p.Run(ctx, srv.Env(db, now)); res.RowsInserted != 0 || len(res.Warnings) != 4 {
		t.Errorf("foreign links: %+v", res)
	}
}
