package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"dashboard.tw/backend/internal/api"
	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/mail"
	"dashboard.tw/backend/internal/notify"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/screen"
)

func newServeCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			readerURL, fallback, err := cfg.ReaderURL()
			if err != nil {
				return err
			}
			gdb, err := db.Connect(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer db.Close(gdb)
			appDB, err := gdb.DB()
			if err != nil {
				return err
			}

			readerDB, err := sql.Open("pgx", readerURL)
			if err != nil {
				return fmt.Errorf("open reader database: %w", err)
			}
			defer readerDB.Close()
			readerDB.SetMaxOpenConns(10)
			readerDB.SetMaxIdleConns(5)
			readerDB.SetConnMaxIdleTime(5 * time.Minute)
			if fallback {
				slog.Warn("READER_DATABASE_URL not set: component queries switch the DATABASE_URL session to dashboard_reader (development only)")
			}
			checkCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			err = component.VerifyReader(checkCtx, readerDB)
			cancel()
			if errors.Is(err, component.ErrUnsafeReader) {
				return err
			}
			if err != nil {
				slog.Warn("cannot check the reader role yet", "error", err)
			}

			reader := &component.Verified{Exec: &component.Executor{DB: readerDB}}
			svc := screen.New(appDB, reader)

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			listener := &db.Listener{
				DatabaseURL: cfg.DatabaseURL,
				Channels:    []string{pipeline.DatasetChannel, component.ConfigChannel},
				OnNotify: func(channel, payload string) {
					if channel == pipeline.DatasetChannel {
						svc.DatasetUpdated(payload)
					} else {
						svc.ConfigUpdated()
					}
				},
				OnConnect: svc.Reset,
				Logger:    slog.Default(),
			}
			go listener.Run(ctx)

			pipelines := map[string]pipeline.Pipeline{}
			var runOrder []string
			for _, s := range registry(cfg) {
				id := s.pipeline.Dataset().ID
				if _, dup := pipelines[id]; !dup {
					runOrder = append(runOrder, id)
				}
				pipelines[id] = s.pipeline
			}
			runner := &pipeline.Runner{
				DB:      appDB,
				Fetch:   pipeline.NewFetcher(version),
				Now:     time.Now,
				Logger:  slog.Default(),
				Timeout: 5 * time.Minute,
			}
			runner.OnFinish = pipelineNotifier(appDB)
			runDataset := func(ctx context.Context, id string) (pipeline.Result, error) {
				p, ok := pipelines[id]
				if !ok {
					return pipeline.Result{}, api.ErrUnknownDataset
				}
				return runner.Run(context.WithoutCancel(ctx), p)
			}
			if cfg.SharedDatabase {
				runDataset = nil
				slog.Warn("DATABASE_SHARED is on: connected to a database another machine owns; manual pipeline runs are disabled")
			}

			if cfg.Env == config.EnvProduction {
				gin.SetMode(gin.ReleaseMode)
			}
			oauth := auth.ConfiguredOAuth(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GoogleClientID, cfg.GoogleClientSecret)
			for _, p := range oauth {
				slog.Info("sign-in provider enabled", "provider", p.ID)
			}
			if err := cfg.RequireEmailKey(); err != nil {
				return err
			}
			emails, err := emailVault(cfg)
			if err != nil {
				return err
			}
			if emails == nil && len(oauth) > 0 {
				slog.Warn("EMAIL_KEY is not set: accounts will keep no email")
			}
			var imageProxy *url.URL
			if cfg.ImageDir == "" && cfg.ImageProxyURL != "" {
				u, err := url.Parse(cfg.ImageProxyURL)
				if err != nil || u.Scheme == "" || u.Host == "" {
					return fmt.Errorf("IMAGE_PROXY_URL is not a URL: %q", cfg.ImageProxyURL)
				}
				imageProxy = u
				slog.Info("serving weather images from another server", "url", u.Redacted())
			}
			store := &auth.Store{DB: appDB, Emails: emails}
			notifier, err := newNotifier(cfg, store)
			if err != nil {
				return err
			}
			var sealer hooks.Sealer
			if emails != nil {
				sealer = emails
			}
			if v := os.Getenv("DISCORD_LINK_EMOJIS"); v != "" {
				emojis, err := hooks.ParseEmojis(v)
				if err != nil {
					return err
				}
				hooks.DiscordEmojis = emojis
			}
			dispatcher := hooks.NewDispatcher(appDB, sealer)
			if notifier != nil {
				notifier.Hooks = dispatcher
			}
			defer func() {
				stop()
				notifier.Wait()
				dispatcher.Wait()
			}()
			notifier.Start(ctx)
			dispatcher.Start(ctx)
			router := api.NewRouter(api.Deps{
				DB:             gormPinger{gdb},
				Version:        version,
				Screen:         svc,
				ClientIPHeader: cfg.ClientIPHeader,
				Auth: api.AuthDeps{
					Store:            store,
					Notify:           notifier,
					Hooks:            dispatcher,
					MailWebhookToken: cfg.Mail.WebhookToken,
					Providers:        auth.Providers(cfg.AuthDevLogin, cfg.Env == config.EnvProduction, oauth...),
					OAuth:            oauth,
					Origins:          cfg.Origins,
					Secure:           cfg.Env == config.EnvProduction,
				},
				Admin:       api.AdminDeps{RunDataset: runDataset, RunOrder: runOrder, Reader: reader.Query},
				OpenData:    &opendata.Service{DB: appDB},
				DownloadDir: cfg.DownloadDir,
				ImageDir:    cfg.ImageDir,
				ImageProxy:  imageProxy,
				WebDir:      cfg.WebDir,
				Indexable:   cfg.SiteIndexable,
			})
			if cfg.WebDir != "" {
				slog.Info("serving the frontend", "dir", cfg.WebDir, "indexable", cfg.SiteIndexable)
			}
			if cfg.AuthDevLogin {
				slog.Warn("AUTH_DEV_LOGIN is on: anyone who can reach this server can sign in as the administrator (development only)")
			}
			return serveHTTP(ctx, cfg.HTTPAddr, router)
		},
	}
}

func serveHTTP(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type gormPinger struct{ gdb *gorm.DB }

func (p gormPinger) Ping(ctx context.Context) error {
	return db.Ping(ctx, p.gdb, 2*time.Second)
}

func newNotifier(cfg config.Config, store *auth.Store) (*notify.Notifier, error) {
	site := "https://dashboard.tw"
	if len(cfg.Origins) > 0 {
		site = cfg.Origins[0]
	}
	m := cfg.Mail
	var sender mail.Sender
	switch m.Mode {
	case "smtp":
		s, err := mail.NewSMTP(mail.Config{Host: m.SMTPHost, Port: m.SMTPPort, User: m.SMTPUser, Password: m.SMTPPassword, From: m.From, FromName: m.FromName})
		if err != nil {
			return nil, err
		}
		sender = s
	case "brevo":
		b, err := mail.NewBrevo(m.BrevoAPIKey, m.From, m.FromName)
		if err != nil {
			return nil, err
		}
		b.Sandbox = m.Sandbox
		sender = b
	default:
		slog.Info("MAIL_MODE is not set: notification emails are off")
		return nil, nil
	}
	if store.Emails == nil {
		slog.Warn("MAIL_MODE is set but EMAIL_KEY is not: there are no addresses to send to")
	}
	n := notify.New(sender, store, site)
	n.DailyLimit = m.DailyLimit
	n.Sandbox = m.Sandbox
	store.OnEvent = n.OnEvent
	slog.Info("notification emails are on", "mode", m.Mode, "from", m.From, "daily_limit", m.DailyLimit, "sandbox", m.Sandbox)
	if m.Sandbox {
		slog.Warn("MAIL_SANDBOX is on: Brevo checks each request but sends nothing")
	}
	return n, nil
}
