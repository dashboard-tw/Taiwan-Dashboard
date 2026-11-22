package cli

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/opendata"
)

func newExportCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Write the monthly bulk download files now",
		Long: "Write a .csv.gz for every finished month that is missing or out of date, and each dataset's index.json,\n" +
			"under DOWNLOAD_DIR. The worker does this daily; run it by hand to fill in past months at once.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.DownloadDir == "" {
				return fmt.Errorf("DOWNLOAD_DIR is not set")
			}
			runner, closeDB, err := newRunner(cmd.Context(), version)
			if err != nil {
				return err
			}
			defer closeDB()
			e := &opendata.Exporter{Service: &opendata.Service{DB: runner.DB}, Dir: cfg.DownloadDir, Now: time.Now, Logger: slog.Default()}
			res, err := e.Run(cmd.Context())
			fmt.Fprintf(cmd.OutOrStdout(), "written %d, already current %d\n", res.Written, res.Current)
			return err
		},
	}
}
