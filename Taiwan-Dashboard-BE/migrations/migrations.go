package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func newProvider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, files)
}

func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	p, err := newProvider(db)
	if err != nil {
		return nil, err
	}
	return p.Up(ctx)
}

func Down(ctx context.Context, db *sql.DB) (*goose.MigrationResult, error) {
	p, err := newProvider(db)
	if err != nil {
		return nil, err
	}
	return p.Down(ctx)
}

func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := newProvider(db)
	if err != nil {
		return nil, err
	}
	return p.Status(ctx)
}

func Describe(s *goose.MigrationStatus) string {
	state := "pending"
	if s.State == goose.StateApplied {
		state = "applied " + s.AppliedAt.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("%05d  %-40s %s", s.Source.Version, s.Source.Path, state)
}
