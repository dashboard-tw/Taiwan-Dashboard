package cli

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/pipeline"
)

func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "dashboardtw",
		Short:         "dashboard.tw backend",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	root.AddCommand(
		newServeCmd(version),
		newWorkerCmd(version),
		newMigrateCmd(),
		newAdminCmd(version),
		newExportCmd(version),
	)
	return root
}

func refuseShared(action string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return cfg.RefuseShared(action)
}

func openDatabase(ctx context.Context) (*sql.DB, *gorm.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.RequireDatabase(); err != nil {
		return nil, nil, err
	}
	gdb, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Ping(ctx, gdb, 5*time.Second); err != nil {
		_ = db.Close(gdb)
		return nil, nil, fmt.Errorf("cannot reach the database: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		_ = db.Close(gdb)
		return nil, nil, err
	}
	return sqlDB, gdb, nil
}

func pipelineNotifier(sqlDB *sql.DB) func(context.Context, pipeline.Dataset, error) {
	site := "https://dashboard.tw"
	if cfg, err := config.Load(); err == nil && len(cfg.Origins) > 0 {
		site = cfg.Origins[0]
	}
	return func(ctx context.Context, d pipeline.Dataset, runErr error) {
		hooks.PipelineFinished(ctx, sqlDB, site, d.ID, d.Name, runErr)
	}
}
