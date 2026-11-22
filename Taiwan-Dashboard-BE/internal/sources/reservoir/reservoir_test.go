package reservoir

import (
	"context"
	"math"
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

func TestParseRealtime(t *testing.T) {
	obs, warnings, err := ParseRealtime(read(t, "testdata/realtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(obs) != 9 {
		t.Fatalf("got %d observations, want 9", len(obs))
	}
	var shimen *Observation
	for i := range obs {
		if obs[i].ReservoirID == "10201" {
			shimen = &obs[i]
			break
		}
	}
	if shimen == nil || shimen.Storage == nil || *shimen.Storage != 18948.56 || *shimen.WaterLevel != 243.14 {
		t.Fatalf("石門 = %+v", shimen)
	}
	if want := time.Date(2026, 10, 20, 15, 0, 0, 0, pipeline.Taipei); !shimen.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %v, want %v", shimen.ObservedAt, want)
	}
}

func TestParseTimeFormats(t *testing.T) {
	for _, s := range []string{"2023-05-03t13:20:10", "20230503t132010", "2023-05-03T13:20:10"} {
		got, err := parseTime(s)
		if err != nil {
			t.Errorf("%s: %v", s, err)
			continue
		}
		if want := time.Date(2023, 5, 3, 13, 20, 10, 0, pipeline.Taipei); !got.Equal(want) {
			t.Errorf("%s = %v", s, got)
		}
	}
}

func TestParseDailyAndBasic(t *testing.T) {
	daily, _, err := ParseDaily(read(t, "testdata/daily.json"))
	if err != nil || len(daily) != 4 {
		t.Fatalf("daily = %d, err %v", len(daily), err)
	}
	basic, _, err := ParseBasic(read(t, "testdata/basic.json"))
	if err != nil || len(basic) != 3 {
		t.Fatalf("basic = %d, err %v", len(basic), err)
	}
	for _, b := range basic {
		if b.ReservoirID == "10205" && (b.Capacity == nil || *b.Capacity != 33347.3) {
			t.Errorf("翡翠 basic capacity = %v (thousands separator not handled?)", b.Capacity)
		}
	}
}

func TestCountyFor(t *testing.T) {
	counties := testCounties(t)
	cases := []struct{ id, township, want string }{
		{"10201", "桃園市龍潭區、\n大溪區、復興區", "68000"},
		{"10204", "基隆巿安樂區", "10017"}, // variant 巿
		{"10802", "", "10002"},       // hand-checked override
		{"50106", "", "10016"},       // 501xx: Penghu
		{"50201", "", "09020"},       // 502xx: Kinmen
		{"50309", "", "09007"},       // 503xx: Lienchiang
		{"30306", "", ""},            // unknown
	}
	for _, c := range cases {
		if got := CountyFor(c.id, c.township, counties); got != c.want {
			t.Errorf("CountyFor(%s, %q) = %q, want %q", c.id, c.township, got, c.want)
		}
	}
}

func TestFixStorageUnit(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		storage, capacity float64
		want              float64
		converted         bool
	}{
		{1073977, 106.1, 107.3977, true},  // 虎頭埤：立方公尺
		{193343, 47.1, 19.3343, true},     // 鹽水埤：立方公尺
		{7554.84, 8180.9, 7554.84, false}, // 南化：已經是萬立方公尺
		{120, 100, 120, false},            // 滿水位以上一點，不換算
		{5000000, 100, 5000000, false},    // 換算後仍不合理，保留原值讓警告提醒
	}
	for _, c := range cases {
		got, ok := FixStorageUnit(f(c.storage), f(c.capacity))
		if ok != c.converted || *got != c.want {
			t.Errorf("%v / %v: got %v %v, want %v %v", c.storage, c.capacity, *got, ok, c.want, c.converted)
		}
	}
	if got, ok := FixStorageUnit(nil, f(100)); got != nil || ok {
		t.Error("unknown storage stays unknown")
	}
	if got, ok := FixStorageUnit(f(500000), nil); *got != 500000 || ok {
		t.Error("without a capacity there is nothing to compare with")
	}
}

func TestStoragePct(t *testing.T) {
	v := func(f float64) *float64 { return &f }
	if got := StoragePct(v(18948.56), v(20526)); got == nil || math.Abs(*got-92.31) > 0.001 {
		t.Errorf("StoragePct = %v, want 92.31", deref(got))
	}
	if StoragePct(nil, v(1)) != nil || StoragePct(v(1), nil) != nil || StoragePct(v(1), v(0)) != nil {
		t.Error("missing storage or capacity must give nil")
	}
}

func TestRefThenObs(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/daily":    "testdata/daily.json",
		"/basic":    "testdata/basic.json",
		"/realtime": "testdata/realtime.json",
	})
	env := srv.Env(db, time.Date(2026, 10, 20, 17, 0, 0, 0, pipeline.Taipei))
	ctx := context.Background()
	ref := &RefPipeline{DailyURL: srv.URL + "/daily", BasicURL: srv.URL + "/basic"}
	obs := &ObsPipeline{URL: srv.URL + "/realtime"}

	if _, err := ref.Run(ctx, env); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir`); n != 5 {
		t.Errorf("reservoirs = %d, want 5", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '10201' AND effective_capacity = 20526 AND county_code = '68000' AND is_major`); n != 1 {
		t.Error("石門 should have capacity 20526, county 68000 and be major")
	}

	res, err := obs.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 8 {
		t.Errorf("inserted %d observations, want 8", res.RowsInserted)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '30901' AND name = '水庫 30901'`); n != 1 {
		t.Error("expected a placeholder for reservoir 30901")
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning about the unknown reservoir")
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.reservoir_obs_current WHERE reservoir_id = '10201' AND storage_pct = 92.31 AND observed_at = '2026-10-20 16:00+08'`); n != 1 {
		t.Error("石門 current row should be the 16:00 reading at 92.31%")
	}

	if res, err = obs.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: inserted %d, err %v", res.RowsInserted, err)
	}

	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.reservoir_obs_history WHERE observed_at > '2026-10-20 17:00+08'`); n != 0 {
		t.Errorf("%d readings from the future were stored", n)
	}
	if !res.LastObservedAt.Equal(time.Date(2026, 10, 20, 16, 0, 0, 0, pipeline.Taipei)) {
		t.Errorf("last observed = %v, want 16:00", res.LastObservedAt)
	}
	morning := srv.Env(db, time.Date(2026, 10, 21, 7, 30, 0, 0, pipeline.Taipei))
	if res, err = obs.Run(ctx, morning); err != nil || res.RowsInserted != 1 {
		t.Errorf("after 07:00: inserted %d, err %v, want the Kinmen reading", res.RowsInserted, err)
	}

	if res, err = ref.Run(ctx, env); err != nil || res.RowsInserted != 0 {
		t.Errorf("unchanged ref refresh: changed %d, err %v", res.RowsInserted, err)
	}

	if _, err := db.Exec(`UPDATE ref.reservoir SET county_code = '63000' WHERE reservoir_id = '10201'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ref.Run(ctx, env); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '10201' AND county_code = '63000'`); n != 1 {
		t.Error("the reference refresh overwrote a county set by hand")
	}

	if _, err := db.Exec(`UPDATE ref.reservoir SET effective_capacity = 12345, capacity_locked = true WHERE reservoir_id = '10201';
		UPDATE ref.reservoir SET effective_capacity = 1 WHERE reservoir_id = '10205'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ref.Run(ctx, env); err != nil {
		t.Fatal(err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '10201' AND effective_capacity = 12345`); n != 1 {
		t.Error("the reference refresh overwrote a locked capacity")
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '10205' AND effective_capacity <> 1`); n != 1 {
		t.Error("an unlocked capacity should follow the source")
	}

	if _, err := db.Exec(`UPDATE ref.reservoir SET county_code = '63000' WHERE county_code IS NULL`); err != nil {
		t.Fatal(err)
	}
	res, err = ref.Run(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "沒有縣市") {
			t.Errorf("warning after every county was set: %s", w)
		}
	}
}

func testCounties(t *testing.T) *pipeline.Counties {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") != "" {
		db := testdb.New(t)
		c, err := pipeline.LoadCounties(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	return pipeline.NewCounties(map[string][]string{
		"68000": {"桃園市"},
		"10017": {"基隆市"},
	})
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
