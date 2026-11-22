package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"dashboard.tw/backend/migrations"
)

func NewEmpty(t *testing.T) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })

	name := "dtw_test_" + randomSuffix(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ensureReaderRole(t, ctx, admin)
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	db, err := sql.Open("pgx", withDatabase(t, base, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})
	return db
}

func New(t *testing.T) *sql.DB {
	t.Helper()
	db := NewEmpty(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

const ReaderRole = "dashboard_reader"

func ensureReaderRole(t *testing.T, ctx context.Context, admin *sql.DB) {
	t.Helper()
	_, err := admin.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '`+ReaderRole+`') THEN
				CREATE ROLE `+ReaderRole+` NOLOGIN;
			END IF;
		EXCEPTION WHEN duplicate_object OR unique_violation THEN
			NULL;
		END
		$$`)
	if err != nil {
		t.Fatalf("create %s role: %v", ReaderRole, err)
	}
}

func Reader(t *testing.T, db *sql.DB) *sql.DB {
	t.Helper()
	u, err := url.Parse(URL(t, db))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("role", ReaderRole)
	u.RawQuery = q.Encode()
	r, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

const AppRole = "dashboard_test_app"

func App(t *testing.T, db *sql.DB) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '`+AppRole+`') THEN
				CREATE ROLE `+AppRole+` NOLOGIN NOSUPERUSER NOBYPASSRLS;
			END IF;
		EXCEPTION WHEN duplicate_object OR unique_violation THEN
			NULL;
		END
		$$`); err != nil {
		t.Fatalf("create %s role: %v", AppRole, err)
	}
	if _, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			GRANT `+ReaderRole+` TO `+AppRole+`;
		EXCEPTION WHEN OTHERS THEN
			IF NOT pg_has_role('`+AppRole+`', '`+ReaderRole+`', 'MEMBER') THEN RAISE; END IF;
		END
		$$`); err != nil {
		t.Fatalf("grant %s to %s: %v", ReaderRole, AppRole, err)
	}
	if _, err := db.ExecContext(ctx, `
		GRANT USAGE ON SCHEMA app, ref, data TO `+AppRole+`;
		GRANT ALL ON ALL TABLES IN SCHEMA app, ref, data TO `+AppRole+`;
		GRANT ALL ON ALL SEQUENCES IN SCHEMA app, ref, data TO `+AppRole); err != nil {
		t.Fatalf("grant privileges to %s: %v", AppRole, err)
	}
	u, err := url.Parse(URL(t, db))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("role", AppRole)
	u.RawQuery = q.Encode()
	a, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

var Datasets = []string{"aqi_obs", "reservoir_obs", "reservoir_ref", "river_station", "river_level", "water_outage", "water_outage_taipei", "power_outage", "earthquake", "earthquake_local", "typhoon_track", "typhoon_warning", "weather_warning", "rain_obs", "radar_image", "satellite_image", "radar_layer", "cloud_layer", "power_gen", "power_supply_daily", "power_supply_forecast",
	"uv_obs", "aqi_forecast", "power_region", "power_load", "power_peak_daily", "weather_obs", "county_forecast", "wind_layer",
	"disaster_alert", "work_closure", "freeway_section", "freeway_traffic", "fuel_price",
	"medical_facility", "ltc_facility", "shelter", "shelter_report",
	"population_year", "traffic_accident", "tourism_attraction", "tourism_hotel",
	"river_quality", "renewable_month", "school",
	"rail_alert", "tra_station", "tra_live", "bike_station", "bike_availability",
	"vital_month", "childcare_center", "power_sales", "waste",
	"farm_price", "food_rejection", "fire_incident",
	"real_estate", "fuel_station",
	"dengue_daily", "nhi_visit", "econ_indicator", "income_town", "shelter_animal", "shelter_stats", "airport", "flight",
	"debris_alert", "flood_sensor", "ev_station", "ev_live", "poultry_price", "hog_trade"}

func RegisterDatasets(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, id := range Datasets {
		if _, err := db.Exec(`
			INSERT INTO app.dataset (id, name, source_agency, source_url, license, update_freq, stale_after)
			VALUES ($1, $1, '測試', 'https://example.test', '測試授權', '1 hour', '3 hours')`, id); err != nil {
			t.Fatal(err)
		}
	}
}

func URL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var name string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return withDatabase(t, os.Getenv("TEST_DATABASE_URL"), name)
}

func withDatabase(t *testing.T, base, name string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
