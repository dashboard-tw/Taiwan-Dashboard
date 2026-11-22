package component_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/seed"
)

func mustCompile(t *testing.T, sql string) component.Compiled {
	t.Helper()
	q, err := component.Compile(sql)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestExecutorBindsParameters(t *testing.T) {
	db := testdb.New(t)
	exec := &component.Executor{DB: testdb.Reader(t, db)}
	q := mustCompile(t, `SELECT code AS x_axis, sort_order AS data FROM ref.county
		WHERE (:county::char(5) IS NULL OR code = :county) ORDER BY sort_order`)

	all, err := exec.Query(context.Background(), q, component.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Values) != 22 {
		t.Errorf("national: %d rows, want 22", len(all.Values))
	}
	county := "63000"
	one, err := exec.Query(context.Background(), q, component.Params{County: &county})
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Values) != 1 || one.Values[0][0] != "63000" {
		t.Errorf("county: %v", one.Values)
	}

	evil := "63000' OR '1'='1"
	none, err := exec.Query(context.Background(), q, component.Params{County: &evil})
	if err == nil && len(none.Values) != 0 {
		t.Errorf("injection returned %d rows", len(none.Values))
	}
}

func TestExecutorCannotReachAppSchema(t *testing.T) {
	db := testdb.New(t)
	exec := &component.Executor{DB: testdb.Reader(t, db)}
	ctx := context.Background()

	for _, sql := range []string{
		`SELECT id AS x_axis, 1 AS data FROM app.dataset`,
		`SELECT slug AS x_axis, 1 AS data FROM app.component`,
	} {
		if _, err := exec.Query(ctx, mustCompile(t, sql), component.Params{}); err == nil ||
			!strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s: err = %v, want permission denied", sql, err)
		}
	}
}

func TestExecutorIsReadOnly(t *testing.T) {
	db := testdb.New(t)
	exec := &component.Executor{DB: testdb.Reader(t, db)}
	_, err := exec.Query(context.Background(),
		mustCompile(t, `WITH d AS (DELETE FROM ref.county RETURNING code) SELECT code AS x_axis, 1 AS data FROM d`),
		component.Params{})
	if err == nil {
		t.Fatal("a data-modifying query must fail")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ref.county`).Scan(&n); err != nil || n != 22 {
		t.Errorf("counties after attempted delete = %d, %v", n, err)
	}
}

func TestExecutorTimeout(t *testing.T) {
	db := testdb.New(t)
	exec := &component.Executor{DB: testdb.Reader(t, db), Timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := exec.Query(context.Background(), mustCompile(t, `SELECT pg_sleep(5)::text AS x_axis, 1 AS data`), component.Params{})
	if !errors.Is(err, component.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("query ran %v, should stop near the 200ms timeout", elapsed)
	}
}

func TestVerifyReader(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := component.VerifyReader(ctx, testdb.Reader(t, db)); err != nil {
		t.Errorf("reader pool: %v", err)
	}
	if err := component.VerifyReader(ctx, db); !errors.Is(err, component.ErrUnsafeReader) {
		t.Errorf("admin pool: err = %v, want ErrUnsafeReader", err)
	}
}

func TestImportSeedAndLoadCatalog(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	testdb.RegisterDatasets(t, db)

	bundle, err := component.ParseBundle(seed.MVP)
	if err != nil {
		t.Fatal(err)
	}
	res, err := component.Import(ctx, db, bundle, "test", false)
	if err != nil {
		t.Fatalf("import seed: %v", err)
	}
	if _, err := component.Import(ctx, db, bundle, "again", false); err != nil {
		t.Fatalf("re-import seed: %v", err)
	}

	cat, err := component.LoadCatalog(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Components) != res.Components || len(cat.Dashboards) != res.Dashboards {
		t.Errorf("catalog has %d components and %d dashboards, imported %d and %d",
			len(cat.Components), len(cat.Dashboards), res.Components, res.Dashboards)
	}
	if cat.Dashboards[0].Slug != "home" || len(cat.Counties) != 22 || !cat.IsCounty("63000") {
		t.Errorf("unexpected catalog: first dashboard %q, %d counties", cat.Dashboards[0].Slug, len(cat.Counties))
	}
	aqi := cat.Components["aqi-by-county"]
	if aqi == nil || aqi.Queries["national"].History == nil || aqi.Queries["county"] == nil {
		t.Fatalf("aqi-by-county not loaded with both scopes: %+v", aqi)
	}

	var versions int
	if err := db.QueryRow(`SELECT count(*) FROM app.component_version`).Scan(&versions); err != nil || versions != 2*res.Components {
		t.Errorf("versions = %d, %v; want one per import per component", versions, err)
	}
}

func TestImportRejectsBadComponents(t *testing.T) {
	db := testdb.New(t)
	testdb.RegisterDatasets(t, db)
	base := func() component.ComponentSpec {
		return component.ComponentSpec{
			Slug: "test-comp", Name: "測試", Topic: "aqi", DatasetIDs: []string{"aqi_obs"},
			TimeRange: "current", ChartTypes: []string{"BarChart"}, Status: "published",
			Queries: []component.QuerySpec{{Scope: "national", DataShape: "two_d",
				ChartSQL: "SELECT name AS x_axis, sort_order AS data FROM ref.county"}},
		}
	}
	cases := map[string]func(*component.ComponentSpec){
		"reads app schema": func(c *component.ComponentSpec) {
			c.Queries[0].ChartSQL = "SELECT id AS x_axis, 1 AS data FROM app.dataset"
		},
		"wrong columns": func(c *component.ComponentSpec) {
			c.Queries[0].ChartSQL = "SELECT name, sort_order FROM ref.county"
		},
		"county query ignores :county": func(c *component.ComponentSpec) {
			c.Queries = append(c.Queries, component.QuerySpec{Scope: "county", DataShape: "two_d",
				ChartSQL: "SELECT name AS x_axis, 1 AS data FROM ref.county"})
		},
		"unknown dataset": func(c *component.ComponentSpec) { c.DatasetIDs = []string{"nope"} },
		"bad slug":        func(c *component.ComponentSpec) { c.Slug = "Bad Slug" },
		"no national":     func(c *component.ComponentSpec) { c.Queries[0].Scope = "county" },
	}
	for name, mutate := range cases {
		c := base()
		mutate(&c)
		if _, err := component.Import(context.Background(), db, component.Bundle{Components: []component.ComponentSpec{c}}, "", false); err == nil {
			t.Errorf("%s: import should fail", name)
		}
	}
	if _, err := component.Import(context.Background(), db, component.Bundle{Components: []component.ComponentSpec{base()}}, "", false); err != nil {
		t.Errorf("valid component: %v", err)
	}
}

func TestParseBundleRejectsUnknownFields(t *testing.T) {
	if _, err := component.ParseBundle([]byte(`{"components":[{"slug":"a","chart_type":["x"]}]}`)); err == nil {
		t.Error("a misspelt field must be rejected")
	}
}
