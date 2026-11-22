package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestLoadEmailKey(t *testing.T) {
	raw := bytes.Repeat([]byte{0xfb}, 32)
	for _, s := range []string{
		base64.StdEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
		" " + base64.StdEncoding.EncodeToString(raw) + "\n",
	} {
		cfg, err := load(envFrom(map[string]string{"EMAIL_KEY": s}))
		if err != nil || !bytes.Equal(cfg.EmailKey, raw) {
			t.Errorf("EMAIL_KEY=%q: key %x, err %v", s, cfg.EmailKey, err)
		}
	}
	for _, bad := range []string{base64.StdEncoding.EncodeToString(raw[:16]), "not base64!!"} {
		if _, err := load(envFrom(map[string]string{"EMAIL_KEY": bad})); err == nil {
			t.Errorf("EMAIL_KEY=%q accepted", bad)
		}
	}
	if cfg, _ := load(envFrom(nil)); cfg.EmailKey != nil {
		t.Error("unset EMAIL_KEY gave a key")
	}
}

func TestRequireEmailKey(t *testing.T) {
	prodGitHub := map[string]string{"APP_ENV": "production", "GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "s"}
	cfg, _ := load(envFrom(prodGitHub))
	if !errors.Is(cfg.RequireEmailKey(), ErrNoEmailKey) {
		t.Error("production GitHub sign-in without EMAIL_KEY allowed")
	}
	prodGitHub["EMAIL_KEY"] = base64.StdEncoding.EncodeToString(make([]byte, 32))
	cfg, _ = load(envFrom(prodGitHub))
	if err := cfg.RequireEmailKey(); err != nil {
		t.Errorf("with key: %v", err)
	}
	for _, env := range []map[string]string{
		{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "s"}, // development
		{"APP_ENV": "production"},                               // no OAuth
	} {
		cfg, _ := load(envFrom(env))
		if err := cfg.RequireEmailKey(); err != nil {
			t.Errorf("%v: %v", env, err)
		}
	}
}

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(envFrom(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.AuthDevLogin {
		t.Error("AuthDevLogin should default to false")
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	if _, err := load(envFrom(map[string]string{"APP_ENV": "staging"})); err == nil {
		t.Fatal("expected an error for an unknown APP_ENV")
	}
}

func TestLoadRejectsDevLoginInProduction(t *testing.T) {
	_, err := load(envFrom(map[string]string{"APP_ENV": "production", "AUTH_DEV_LOGIN": "true"}))
	if !errors.Is(err, ErrDevLoginInProduction) {
		t.Fatalf("err = %v, want ErrDevLoginInProduction", err)
	}
}

func TestLoadAllowsDevLoginInDevelopment(t *testing.T) {
	cfg, err := load(envFrom(map[string]string{"AUTH_DEV_LOGIN": "true"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.AuthDevLogin {
		t.Error("AuthDevLogin should be true")
	}
}

func TestLoadRejectsInvalidBool(t *testing.T) {
	if _, err := load(envFrom(map[string]string{"AUTH_DEV_LOGIN": "maybe"})); err == nil {
		t.Fatal("expected an error for an invalid AUTH_DEV_LOGIN")
	}
}

func TestRequireDatabase(t *testing.T) {
	if err := (Config{}).RequireDatabase(); err == nil {
		t.Error("expected an error when DATABASE_URL is empty")
	}
	if err := (Config{DatabaseURL: "postgres://x"}).RequireDatabase(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReaderURL(t *testing.T) {
	dev, _ := load(envFrom(map[string]string{"DATABASE_URL": "postgres://u:p@127.0.0.1:5432/dashboardtw?sslmode=disable"}))
	url, fallback, err := dev.ReaderURL()
	if err != nil || !fallback || url != "postgres://u:p@127.0.0.1:5432/dashboardtw?role=dashboard_reader&sslmode=disable" {
		t.Errorf("development fallback = %q, %v, %v", url, fallback, err)
	}

	prod, _ := load(envFrom(map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres:///dashboardtw"}))
	if _, _, err := prod.ReaderURL(); !errors.Is(err, ErrNoReaderURL) {
		t.Errorf("production without READER_DATABASE_URL: err = %v", err)
	}

	prod.ReaderDatabaseURL = "postgres:///dashboardtw?user=dashboard_reader"
	if url, fallback, err := prod.ReaderURL(); err != nil || fallback || url != prod.ReaderDatabaseURL {
		t.Errorf("explicit reader URL = %q, %v, %v", url, fallback, err)
	}
}

func TestOrigins(t *testing.T) {
	dev, _ := load(envFrom(nil))
	if len(dev.Origins) != 2 || dev.Origins[0] != "http://localhost:5173" {
		t.Errorf("development default origins = %v", dev.Origins)
	}
	prod, _ := load(envFrom(map[string]string{"APP_ENV": "production"}))
	if len(prod.Origins) != 0 {
		t.Errorf("production must not allow any origin by default: %v", prod.Origins)
	}
	set, _ := load(envFrom(map[string]string{"APP_ENV": "production", "PUBLIC_ORIGINS": "https://dashboard.tw/, https://www.dashboard.tw"}))
	if len(set.Origins) != 2 || set.Origins[0] != "https://dashboard.tw" || set.Origins[1] != "https://www.dashboard.tw" {
		t.Errorf("PUBLIC_ORIGINS = %v", set.Origins)
	}
}

func TestRefuseShared(t *testing.T) {
	env := map[string]string{"DATABASE_SHARED": "true"}
	cfg, err := load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RefuseShared("migrate up"); !errors.Is(err, ErrSharedDatabase) {
		t.Errorf("shared database: err = %v, want ErrSharedDatabase", err)
	}
	cfg, err = load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RefuseShared("migrate up"); err != nil {
		t.Errorf("own database: err = %v", err)
	}
	env["DATABASE_SHARED"] = "maybe"
	if _, err := load(func(k string) string { return env[k] }); err == nil {
		t.Error("an invalid DATABASE_SHARED should be an error")
	}
}
