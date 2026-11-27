package earthquake

import (
	"context"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseStations(t *testing.T) {
	reports, warnings, err := Parse(read(t, "testdata/stations.json"))
	if err != nil || len(warnings) != 0 || len(reports) != 1 {
		t.Fatalf("reports=%d warnings=%v err=%v", len(reports), warnings, err)
	}
	r := reports[0]
	if r.ShakemapURL == "" || r.ReportType != "地震報告" || r.Remark == "" || len(r.Stations) != 51 {
		t.Fatalf("report shakemap=%q type=%q remark=%q stations=%d", r.ShakemapURL, r.ReportType, r.Remark, len(r.Stations))
	}
	s := r.Stations[0]
	if s.ID != "LAY" || s.Name != "蘭嶼" || s.County != "臺東縣" || s.Intensity != "4級" || *s.Rank != 4 ||
		*s.PGA != 44.69 || *s.PGAEW != 51.81 || *s.PGV != 1.5 || *s.DistanceKm != 20.36 || *s.Lat != 22.037 ||
		s.WaveURL == "" || s.Status != "observe" {
		t.Errorf("first station = %+v", s)
	}
}

func TestRunSavesStations(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{"/eq": "testdata/stations.json"})
	p := &Pipeline{Kind: "significant", URL: srv.URL + "/eq", APIKey: "k"}
	env := srv.Env(db, time.Date(2026, 11, 27, 9, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 1 {
		t.Fatalf("run: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `
		SELECT count(*) FROM data.earthquake_station
		 WHERE report_id = '2026081' AND geom IS NOT NULL AND county_code IS NOT NULL`); n != 51 {
		t.Errorf("stations = %d", n)
	}
	// 有 4 站沒有加速度資料
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.earthquake_station WHERE pga_gal > 0`); n != 47 {
		t.Errorf("stations with PGA = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.earthquake WHERE shakemap_url LIKE 'https://%'`); n != 1 {
		t.Errorf("shakemap not stored")
	}
	// 舊的報告沒有測站時，下次執行要補上
	if _, err := db.Exec(`DELETE FROM data.earthquake_station`); err != nil {
		t.Fatal(err)
	}
	if res, err := p.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Fatalf("second run: %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.earthquake_station`); n != 51 {
		t.Errorf("backfilled stations = %d", n)
	}
}
