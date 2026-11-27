package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/seed"
)

func newAdminCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative tasks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAdminImportCmd(version), newAdminCreateCmd(), newAdminCheckEmailCmd(), newAdminDevCmd())
	return cmd
}

func emailVault(cfg config.Config) (*auth.EmailVault, error) {
	if cfg.EmailKey == nil {
		return nil, nil
	}
	return auth.NewEmailVault(cfg.EmailKey)
}

func newAdminCheckEmailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-email <user-id> <email>",
		Short: "Check whether an email belongs to a user (fingerprint only, nothing decrypted)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("user id: %w", err)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			vault, err := emailVault(cfg)
			if err != nil {
				return err
			}
			if vault == nil {
				return fmt.Errorf("EMAIL_KEY is not set")
			}
			sqlDB, gdb, err := openDatabase(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close(gdb)
			ok, err := (&auth.Store{DB: sqlDB, Emails: vault}).EmailBelongsTo(cmd.Context(), id, args[1])
			if err != nil {
				return err
			}
			if ok {
				fmt.Printf("yes: this is the email stored for user %d\n", id)
			} else {
				fmt.Printf("no: user %d has no such email stored\n", id)
			}
			return nil
		},
	}
}

func newAdminDevCmd() *cobra.Command {
	var off bool
	cmd := &cobra.Command{
		Use:   "dev-account <email>",
		Short: "Mark an account as a dev account: no survey, no credential or quota limits, left out of admin statistics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			vault, err := emailVault(cfg)
			if err != nil {
				return err
			}
			sqlDB, gdb, err := openDatabase(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close(gdb)
			id, err := (&auth.Store{DB: sqlDB, Emails: vault}).SetDev(cmd.Context(), args[0], !off)
			if err != nil {
				return err
			}
			if off {
				fmt.Printf("user %d is no longer a dev account\n", id)
			} else {
				fmt.Printf("user %d is now a dev account\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&off, "off", false, "turn the dev flag off")
	return cmd
}

func newAdminImportCmd(version string) *cobra.Command {
	var dryRun bool
	var note string
	cmd := &cobra.Command{
		Use:   "import [bundle.json]",
		Short: "Import components and dashboards from a JSON bundle",
		Long: "Import components and dashboards from a JSON bundle, test-running every query first.\n" +
			"Without a file, imports the built-in MVP bundle (backend/seed/mvp.json).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := seed.MVP
			source := "built-in MVP bundle"
			if len(args) == 1 {
				b, err := os.ReadFile(args[0])
				if err != nil {
					return err
				}
				raw, source = b, args[0]
			}
			bundle, err := component.ParseBundle(raw)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RefuseShared("admin import"); err != nil {
				return err
			}
			runner, closeDB, err := newRunner(cmd.Context(), version)
			if err != nil {
				return err
			}
			defer closeDB()
			if err := runner.RegisterDatasets(cmd.Context(), pipelinesOf(registry(cfg))); err != nil {
				return err
			}

			if note == "" {
				note = "import " + source
			}
			res, err := component.Import(cmd.Context(), runner.DB, bundle, note, dryRun)
			if err != nil {
				return err
			}
			verb := "imported"
			if dryRun {
				verb = "checked (dry run, nothing saved)"
			}
			fmt.Printf("%s %d components and %d dashboards from %s\n", verb, res.Components, res.Dashboards, source)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "check the bundle and test-run its queries without saving")
	cmd.Flags().StringVar(&note, "note", "", "note stored with each component version")
	return cmd
}

func newAdminCreateCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create the administrator account (development sign-in)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			sqlDB, gdb, err := openDatabase(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close(gdb)
			u, err := (&auth.Store{DB: sqlDB}).CreateAdmin(cmd.Context(), name)
			if err != nil {
				return err
			}
			fmt.Printf("created administrator %q (id %d); sign in with the development provider\n", u.DisplayName, u.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name of the administrator")
	return cmd
}
