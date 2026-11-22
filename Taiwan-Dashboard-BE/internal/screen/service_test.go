package screen_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/screen"
	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/seed"
)

type countingQuerier struct {
	inner screen.Querier
	calls atomic.Int32
	fail  atomic.Bool
	delay time.Duration
}

func (q *countingQuerier) Query(ctx context.Context, c component.Compiled, p component.Params) (component.Rows, error) {
	q.calls.Add(1)
	time.Sleep(q.delay)
	if q.fail.Load() {
		return component.Rows{}, errors.New("database down")
	}
	return q.inner.Query(ctx, c, p)
}

func setup(t *testing.T) (*sql.DB, *screen.Service, *countingQuerier) {
	t.Helper()
	database := testdb.New(t)
	testdb.RegisterDatasets(t, database)
	bundle, err := component.ParseBundle(seed.MVP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := component.Import(context.Background(), database, bundle, "test", false); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO ref.aqi_site (site_id, name, county_code) VALUES (1, '中山', '63000'), (2, '大同', '63000');
		INSERT INTO data.aqi_obs_current (site_id, observed_at, aqi, status)
		VALUES (1, now(), 40, '良好'), (2, now(), 60, '普通')`); err != nil {
		t.Fatal(err)
	}
	q := &countingQuerier{inner: &component.Executor{DB: testdb.Reader(t, database)}}
	return database, screen.New(database, q), q
}

func taipeiAQI(t *testing.T, svc *screen.Service) float64 {
	t.Helper()
	res, err := svc.Data(context.Background(), screen.Request{Slug: "aqi-by-county"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(res.Chart.Data)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		County string   `json:"county_code"`
		Data   *float64 `json:"data"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.County == "63000" && r.Data != nil {
			return *r.Data
		}
	}
	t.Fatalf("no 63000 value in %s", b)
	return 0
}

func TestThousandConcurrentRequestsQueryOnce(t *testing.T) {
	_, svc, q := setup(t)
	q.delay = 100 * time.Millisecond // keep the first load in flight while the rest arrive

	var wg sync.WaitGroup
	var failures atomic.Int32
	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Data(context.Background(), screen.Request{Slug: "aqi-by-county"}); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d requests failed", failures.Load())
	}
	if n := q.calls.Load(); n != 1 {
		t.Errorf("1000 concurrent requests made %d database queries, want 1", n)
	}
}

func TestWorkerNotificationRefreshesCache(t *testing.T) {
	database, svc, q := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connected := make(chan struct{}, 1)
	l := &db.Listener{
		DatabaseURL: testdb.URL(t, database),
		Channels:    []string{pipeline.DatasetChannel, component.ConfigChannel},
		OnNotify: func(channel, payload string) {
			if channel == pipeline.DatasetChannel {
				svc.DatasetUpdated(payload)
			} else {
				svc.ConfigUpdated()
			}
		},
		OnConnect: func() { svc.Reset(); connected <- struct{}{} },
		Logger:    slog.New(slog.DiscardHandler),
	}
	go l.Run(ctx)
	<-connected

	if got := taipeiAQI(t, svc); got != 50 {
		t.Fatalf("Taipei AQI = %v, want 50", got)
	}
	if _, err := database.Exec(`UPDATE data.aqi_obs_current SET aqi = 80 WHERE site_id = 2`); err != nil {
		t.Fatal(err)
	}
	if got := taipeiAQI(t, svc); got != 50 {
		t.Fatalf("without a notification the cached 50 should be served, got %v", got)
	}

	if _, err := database.Exec(`SELECT pg_notify($1, 'aqi_obs')`, pipeline.DatasetChannel); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for taipeiAQI(t, svc) != 60 {
		if time.Now().After(deadline) {
			t.Fatal("cache was not refreshed after NOTIFY dataset_updated")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if q.calls.Load() != 2 {
		t.Errorf("queries = %d, want 2 (initial and after the notification)", q.calls.Load())
	}
}

func TestDatabaseDownServesLastResult(t *testing.T) {
	database, svc, q := setup(t)
	first, err := svc.Data(context.Background(), screen.Request{Slug: "aqi-by-county"})
	if err != nil {
		t.Fatal(err)
	}

	q.fail.Store(true)
	database.Close()
	svc.DatasetUpdated("aqi_obs")

	res, err := svc.Data(context.Background(), screen.Request{Slug: "aqi-by-county"})
	if err != nil {
		t.Fatalf("with the database down: %v, want the previous result", err)
	}
	if !res.Meta.Stale {
		t.Error("meta.stale should be true")
	}
	if len(res.Rows.Values) != len(first.Rows.Values) {
		t.Error("the previous data should be served")
	}
	if first.Meta.Stale {
		t.Error("the cached original must not be modified")
	}
}

func TestDataValidation(t *testing.T) {
	_, svc, _ := setup(t)
	ctx := context.Background()
	from := time.Date(2026, 10, 2, 0, 0, 0, 0, pipeline.Taipei)
	to := from.Add(-time.Hour)
	long := from.Add(-6 * 365 * 24 * time.Hour)

	cases := map[string]struct {
		req  screen.Request
		code string
	}{
		"unknown component":    {screen.Request{Slug: "nope"}, "component_not_found"},
		"unknown county":       {screen.Request{Slug: "aqi-by-county", County: "99999"}, "invalid_county"},
		"national only":        {screen.Request{Slug: "power-gen-mix", County: "63000"}, "county_not_supported"},
		"no history":           {screen.Request{Slug: "aqi-status", History: true}, "history_not_found"},
		"from after to":        {screen.Request{Slug: "aqi-by-county", History: true, From: &from, To: &to}, "invalid_time_range"},
		"range over five year": {screen.Request{Slug: "aqi-by-county", History: true, From: &long, To: &from}, "invalid_time_range"},
	}
	for name, c := range cases {
		_, err := svc.Data(ctx, c.req)
		e, ok := screen.IsClientError(err)
		if !ok || e.Code != c.code {
			t.Errorf("%s: err = %v, want code %s", name, err, c.code)
		}
	}

	county, err := svc.Data(ctx, screen.Request{Slug: "aqi-by-county", County: "63000"})
	if err != nil || county.Meta.Scope != "county" || county.Meta.DataShape != "two_d" || len(county.Rows.Values) != 2 {
		t.Errorf("county data = %+v, %v", county, err)
	}
	hist, err := svc.Data(ctx, screen.Request{Slug: "aqi-by-county", County: "63000", History: true})
	if err != nil || hist.Meta.DataShape != "time" || hist.Meta.Step != "hour" || hist.Meta.From == "" {
		t.Errorf("county history (shared national query) = %+v, %v", hist, err)
	}
}
