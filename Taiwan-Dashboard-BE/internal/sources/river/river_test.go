package river

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

func TestParseStations(t *testing.T) {
	stations, warnings, err := ParseStations(read(t, "testdata/stations.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 8 || len(warnings) != 0 {
		t.Fatalf("stations=%d warnings=%v", len(stations), warnings)
	}
	by := map[string]Station{}
	for _, s := range stations {
		by[s.ID] = s
	}
	s := by["1010H006"]
	if s.Name != "新磺溪橋" || s.River != "磺溪" || !s.Active || *s.Alerts[0] != 5.8 || *s.Alerts[1] != 4.6 || s.Alerts[2] != nil {
		t.Errorf("1010H006 = %+v", s)
	}
	if s := by["1140H117"]; s.X != nil {
		t.Errorf("1140H117 has no coordinates in the source: %+v", s)
	}
	if by["1010H001"].Active {
		t.Error("1010H001 is 已廢")
	}
}

func TestParseStationsRejectsBadAlertOrder(t *testing.T) {
	body := `[{"basinidentifier":"1140H001","observatoryname":"測試","alertlevel1":"3","alertlevel2":"5","alertlevel3":"",
	           "locationbytwd97_xy":"1 2","observationstatus":"現存"}]`
	stations, warnings, err := ParseStations([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	s := stations[0]
	if s.Alerts[0] != nil || s.Alerts[1] != nil || s.X != nil || len(warnings) != 2 {
		t.Errorf("station = %+v, warnings = %v", s, warnings)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{"新磺溪橋(即時)": "新磺溪橋", "台北橋": "台北橋", " 碧潭橋（即時） ": "碧潭橋"} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q", in, got)
		}
	}
}

func TestParseLevels(t *testing.T) {
	readings, warnings, err := ParseLevels(read(t, "testdata/levels.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 10 || len(warnings) != 0 {
		t.Fatalf("readings=%d warnings=%v", len(readings), warnings)
	}
	r := readings[0]
	if r.StationID != "1010H006" || *r.LevelM != 1.83 ||
		!r.ObservedAt.Equal(time.Date(2026, 10, 22, 10, 20, 0, 0, pipeline.Taipei)) {
		t.Errorf("first reading = %+v", r)
	}
}

func TestStationAndLevelRun(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/stations": "testdata/stations.json",
		"/levels":   "testdata/levels.json",
	})
	env := srv.Env(db, time.Date(2026, 10, 22, 10, 30, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	stations := &StationPipeline{URL: srv.URL + "/stations"}
	levels := &LevelPipeline{URL: srv.URL + "/levels"}

	res, err := stations.Run(ctx, env)
	if err != nil || res.RowsInserted != 8 {
		t.Fatalf("stations: %+v, %v", res, err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "1140H052") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	counties := map[string]string{
		"1010H006": "65000", // 行政區代碼
		"1140H052": "63000", // 站址「台北市…」
		"1140H117": "63000", // 行政區代碼，沒有座標
		"1010H007": "65000", // 站址
	}
	for id, want := range counties {
		var got string
		if err := db.QueryRow(`SELECT coalesce(county_code, '') FROM ref.river_station WHERE station_id = $1`, id).Scan(&got); err != nil || got != want {
			t.Errorf("%s county = %q, want %q (%v)", id, got, want, err)
		}
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.river_station WHERE station_id = '1140H173' AND county_code IS NOT NULL`); n != 1 {
		t.Error("1140H173 was not located by its coordinates")
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.river_station WHERE geom IS NOT NULL`); n != 6 {
		t.Errorf("stations with geometry = %d, want 6 (1140H117 has none, 1140H052's is dropped)", n)
	}
	if res, err := stations.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("unchanged stations written again: %+v, %v", res, err)
	}

	res, err = levels.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 9 || len(res.Warnings) != 2 {
		t.Errorf("levels: %+v", res)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.river_station WHERE station_id = '9999H999'`); n != 1 {
		t.Error("no placeholder for the unknown station")
	}
	var level float64
	if err := db.QueryRow(`SELECT water_level_m FROM data.river_level_current WHERE station_id = '1010H007'`).Scan(&level); err != nil || level != 0.38 {
		t.Errorf("current 1010H007 = %v, %v", level, err)
	}
	if res, err := levels.Run(ctx, env); err != nil || res.RowsInserted != 0 || len(res.Warnings) != 1 {
		t.Errorf("second run: %+v, %v", res, err)
	}

	if res, err := stations.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("stations after levels: %+v, %v", res, err)
	}
}
