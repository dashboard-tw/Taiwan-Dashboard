package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

var Taipei = time.FixedZone("Asia/Taipei", 8*60*60)

const License = "政府資料開放授權條款－第 1 版"

type Dataset struct {
	ID           string
	Name         string
	SourceAgency string
	SourceURL    string
	UpdateFreq   time.Duration // how often the source publishes
	StaleAfter   time.Duration // flag the dataset as stale when data is older than this
	License      string
}

type Result struct {
	RowsFetched    int
	RowsInserted   int
	LastObservedAt time.Time
	Warnings       []string
}

func (r *Result) Warnf(format string, args ...any) {
	r.Warnings = append(r.Warnings, sprintf(format, args...))
}

func (r *Result) ObserveTime(t time.Time) {
	if t.After(r.LastObservedAt) {
		r.LastObservedAt = t
	}
}

type Env struct {
	DB     *sql.DB
	Fetch  *Fetcher
	Now    func() time.Time
	Logger *slog.Logger
}

type Pipeline interface {
	Dataset() Dataset
	Run(ctx context.Context, env Env) (Result, error)
}

var ErrNotConfigured = errors.New("pipeline not configured")
