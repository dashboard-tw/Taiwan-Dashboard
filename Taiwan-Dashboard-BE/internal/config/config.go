package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvProduction  Env = "production"
)

type Config struct {
	Env                                Env
	HTTPAddr                           string
	DatabaseURL                        string
	AuthDevLogin                       bool
	MoenvAPIKey                        string
	CwaAPIKey                          string
	TdxClientID                        string
	TdxClientSecret                    string
	TdxClientID2                       string
	TdxClientSecret2                   string
	TdxClientID3                       string
	TdxClientSecret3                   string
	GitHubClientID, GitHubClientSecret string
	GoogleClientID, GoogleClientSecret string
	EmailKey                           []byte
	ReaderDatabaseURL                  string
	ClientIPHeader                     string
	Origins                            []string
	SharedDatabase                     bool
	DownloadDir                        string
	ImageDir                           string
	ImageProxyURL                      string
	WebDir                             string
	SiteIndexable                      bool
	Mail                               Mail
}

type Mail struct {
	Mode         string // smtp 或 brevo
	From         string
	FromName     string
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	BrevoAPIKey  string
	DailyLimit   int
	Sandbox      bool
	WebhookToken string
}

var ErrDevLoginInProduction = errors.New("AUTH_DEV_LOGIN must not be enabled when APP_ENV=production")

func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Env:         Env(withDefault(getenv("APP_ENV"), string(EnvDevelopment))),
		HTTPAddr:    withDefault(getenv("HTTP_ADDR"), "127.0.0.1:8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		MoenvAPIKey: getenv("MOENV_API_KEY"),
		CwaAPIKey:   getenv("CWA_API_KEY"),

		TdxClientID:     getenv("TDX_CLIENT_ID"),
		TdxClientSecret: getenv("TDX_CLIENT_SECRET"),

		TdxClientID2:     getenv("TDX_CLIENT_ID_2"),
		TdxClientSecret2: getenv("TDX_CLIENT_SECRET_2"),
		TdxClientID3:     getenv("TDX_CLIENT_ID_3"),
		TdxClientSecret3: getenv("TDX_CLIENT_SECRET_3"),

		GitHubClientID:     getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET"),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET"),

		ReaderDatabaseURL: getenv("READER_DATABASE_URL"),
		DownloadDir:       getenv("DOWNLOAD_DIR"),
		ImageDir:          getenv("IMAGE_DIR"),
		ImageProxyURL:     getenv("IMAGE_PROXY_URL"),
		ClientIPHeader:    getenv("CLIENT_IP_HEADER"),
		WebDir:            getenv("WEB_DIR"),
	}
	for _, o := range strings.Split(getenv("PUBLIC_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.Origins = append(cfg.Origins, strings.TrimSuffix(o, "/"))
		}
	}

	switch cfg.Env {
	case EnvDevelopment, EnvProduction:
	default:
		return Config{}, fmt.Errorf("APP_ENV must be %q or %q, got %q", EnvDevelopment, EnvProduction, cfg.Env)
	}

	indexable, err := parseBool(getenv("SITE_INDEXABLE"))
	if err != nil {
		return Config{}, fmt.Errorf("SITE_INDEXABLE: %w", err)
	}
	cfg.SiteIndexable = indexable
	devLogin, err := parseBool(getenv("AUTH_DEV_LOGIN"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_DEV_LOGIN: %w", err)
	}
	cfg.AuthDevLogin = devLogin

	shared, err := parseBool(getenv("DATABASE_SHARED"))
	if err != nil {
		return Config{}, fmt.Errorf("DATABASE_SHARED: %w", err)
	}
	cfg.SharedDatabase = shared

	if k := strings.TrimSpace(getenv("EMAIL_KEY")); k != "" {
		key, err := decodeKey(k)
		if err != nil {
			return Config{}, fmt.Errorf("EMAIL_KEY: %w", err)
		}
		cfg.EmailKey = key
	}

	if cfg.Mail, err = loadMail(getenv); err != nil {
		return Config{}, err
	}

	if cfg.Env == EnvProduction && cfg.AuthDevLogin {
		return Config{}, ErrDevLoginInProduction
	}
	if cfg.Env == EnvDevelopment && len(cfg.Origins) == 0 {
		cfg.Origins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}

	return cfg, nil
}

func loadMail(getenv func(string) string) (Mail, error) {
	m := Mail{
		Mode:         strings.ToLower(strings.TrimSpace(getenv("MAIL_MODE"))),
		From:         withDefault(getenv("MAIL_FROM"), "noreply@dashboard.tw"),
		FromName:     withDefault(getenv("MAIL_FROM_NAME"), "臺灣儀表板"),
		SMTPHost:     getenv("SMTP_HOST"),
		SMTPUser:     getenv("SMTP_USER"),
		SMTPPassword: getenv("SMTP_PASSWORD"),
		BrevoAPIKey:  getenv("BREVO_API_KEY"),
		WebhookToken: strings.TrimSpace(getenv("BREVO_WEBHOOK_TOKEN")),
	}
	sandbox, err := parseBool(getenv("MAIL_SANDBOX"))
	if err != nil {
		return Mail{}, fmt.Errorf("MAIL_SANDBOX: %w", err)
	}
	m.Sandbox = sandbox
	if m.WebhookToken != "" && len(m.WebhookToken) < 32 {
		return Mail{}, errors.New("BREVO_WEBHOOK_TOKEN must be at least 32 characters")
	}
	m.DailyLimit = 295
	if v := getenv("MAIL_DAILY_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Mail{}, fmt.Errorf("MAIL_DAILY_LIMIT must be 0 or more, got %q", v)
		}
		m.DailyLimit = n
	}
	if p := getenv("SMTP_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return Mail{}, fmt.Errorf("SMTP_PORT must be a port number, got %q", p)
		}
		m.SMTPPort = n
	}
	switch m.Mode {
	case "":
	case "smtp":
		if m.SMTPHost == "" {
			return Mail{}, errors.New("MAIL_MODE=smtp needs SMTP_HOST")
		}
		if m.Sandbox {
			return Mail{}, errors.New("MAIL_SANDBOX only works with MAIL_MODE=brevo")
		}
	case "brevo":
		if m.BrevoAPIKey == "" {
			return Mail{}, errors.New("MAIL_MODE=brevo needs BREVO_API_KEY")
		}
	default:
		return Mail{}, fmt.Errorf("MAIL_MODE must be smtp or brevo, got %q", m.Mode)
	}
	return m, nil
}

func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	return nil
}

var ErrSharedDatabase = errors.New("DATABASE_SHARED is set")

func (c Config) RefuseShared(action string) error {
	if !c.SharedDatabase {
		return nil
	}
	return fmt.Errorf("%w: %s must run on the machine that owns this database, not over the tunnel", ErrSharedDatabase, action)
}

var ErrNoReaderURL = errors.New("READER_DATABASE_URL must be set when APP_ENV=production")

func (c Config) ReaderURL() (url string, fallback bool, err error) {
	if c.ReaderDatabaseURL != "" {
		return c.ReaderDatabaseURL, false, nil
	}
	if c.Env == EnvProduction {
		return "", false, ErrNoReaderURL
	}
	u, err := neturl.Parse(c.DatabaseURL)
	if err != nil {
		return "", false, fmt.Errorf("DATABASE_URL: %w", err)
	}
	q := u.Query()
	q.Set("role", "dashboard_reader")
	u.RawQuery = q.Encode()
	return u.String(), true, nil
}

var ErrNoEmailKey = errors.New("EMAIL_KEY must be set when APP_ENV=production and GitHub or Google sign-in is configured")

func (c Config) RequireEmailKey() error {
	oauth := (c.GitHubClientID != "" && c.GitHubClientSecret != "") || (c.GoogleClientID != "" && c.GoogleClientSecret != "")
	if c.Env == EnvProduction && oauth && c.EmailKey == nil {
		return ErrNoEmailKey
	}
	return nil
}

func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			if len(b) != 32 {
				return nil, fmt.Errorf("must be 32 bytes, got %d", len(b))
			}
			return b, nil
		}
	}
	return nil, errors.New("not base64")
}

func withDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseBool(v string) (bool, error) {
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}
