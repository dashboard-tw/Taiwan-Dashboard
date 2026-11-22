package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/migrations"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply or inspect database migrations",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "up",
			Short: "Apply every pending migration",
			RunE: func(cmd *cobra.Command, _ []string) error {
				if err := refuseShared("migrate up"); err != nil {
					return err
				}
				sqlDB, gdb, err := openDatabase(cmd.Context())
				if err != nil {
					return err
				}
				defer db.Close(gdb)
				results, err := migrations.Up(cmd.Context(), sqlDB)
				for _, r := range results {
					fmt.Fprintf(cmd.OutOrStdout(), "applied %s (%s)\n", r.Source.Path, r.Duration.Round(time.Millisecond))
				}
				if err != nil {
					return err
				}
				if len(results) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "database is up to date")
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "down",
			Short: "Roll back the most recent migration",
			RunE: func(cmd *cobra.Command, _ []string) error {
				if err := refuseShared("migrate down"); err != nil {
					return err
				}
				sqlDB, gdb, err := openDatabase(cmd.Context())
				if err != nil {
					return err
				}
				defer db.Close(gdb)
				r, err := migrations.Down(cmd.Context(), sqlDB)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "rolled back %s\n", r.Source.Path)
				return nil
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "List migrations and whether they are applied",
			RunE: func(cmd *cobra.Command, _ []string) error {
				sqlDB, gdb, err := openDatabase(cmd.Context())
				if err != nil {
					return err
				}
				defer db.Close(gdb)
				statuses, err := migrations.Status(cmd.Context(), sqlDB)
				if err != nil {
					return err
				}
				for _, s := range statuses {
					fmt.Fprintln(cmd.OutOrStdout(), migrations.Describe(s))
				}
				return nil
			},
		},
	)
	return cmd
}
