package opendata

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"dashboard.tw/backend/internal/csvout"
)

const IndexFile = "index.json"

const exportTimeout = 15 * time.Minute

var MonthFile = regexp.MustCompile(`^(\d{4}-(0[1-9]|1[0-2]))\.csv\.gz$`)

type FileInfo struct {
	Month       string    `json:"month"`
	File        string    `json:"file"`
	Rows        int64     `json:"rows"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
	GeneratedAt time.Time `json:"generated_at"`
}

type Index struct {
	Dataset     string     `json:"dataset"`
	Description string     `json:"description"`
	License     string     `json:"license"`
	Source      string     `json:"source"`
	Fields      []Field    `json:"fields"`
	Files       []FileInfo `json:"files"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Exporter struct {
	Service *Service
	Dir     string
	Now     func() time.Time
	Logger  *slog.Logger
}

type ExportResult struct {
	Written int
	Current int
}

func (e *Exporter) Run(ctx context.Context) (ExportResult, error) {
	var res ExportResult
	var errs []error
	thisMonth := e.Now().In(taipei).Format("2006-01")
	for i := range Datasets {
		d := &Datasets[i]
		if d.Months == "" {
			continue
		}
		w, c, err := e.dataset(ctx, d, thisMonth)
		res.Written += w
		res.Current += c
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.ID, err))
		}
	}
	return res, errors.Join(errs...)
}

func (e *Exporter) dataset(ctx context.Context, d *Dataset, thisMonth string) (written, current int, err error) {
	months, err := e.monthCounts(ctx, d)
	if err != nil {
		return 0, 0, err
	}
	dir := filepath.Join(e.Dir, d.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, 0, err
	}
	idx, err := readIndex(dir)
	if err != nil {
		return 0, 0, err
	}
	known := map[string]FileInfo{}
	for _, f := range idx.Files {
		known[f.Month] = f
	}
	changed := idx.Dataset == ""
	for _, m := range months {
		if m.month >= thisMonth {
			continue // 這個月還沒結束
		}
		if f, ok := known[m.month]; ok && f.Rows == m.rows {
			if _, err := os.Stat(filepath.Join(dir, f.File)); err == nil {
				current++
				continue
			}
		}
		info, err := e.month(ctx, d, dir, m.month)
		if err != nil {
			return written, current, fmt.Errorf("%s: %w", m.month, err)
		}
		known[m.month] = info
		written++
		changed = true
		e.Logger.Info("bulk file written", "dataset", d.ID, "month", m.month, "rows", info.Rows, "bytes", info.Bytes)
	}
	if !changed {
		return written, current, nil
	}
	sum, err := e.Service.Get(ctx, d.ID)
	if err != nil {
		return written, current, err
	}
	idx = Index{Dataset: d.ID, Description: d.Description, Fields: d.Fields, UpdatedAt: e.Now().In(taipei)}
	if sum != nil {
		idx.License, idx.Source = sum.License, sum.Agency
	}
	for _, m := range months {
		if f, ok := known[m.month]; ok && m.month < thisMonth {
			idx.Files = append(idx.Files, f)
		}
	}
	if idx.Files == nil {
		idx.Files = []FileInfo{}
	}
	return written, current, writeJSON(filepath.Join(dir, IndexFile), idx)
}

type monthCount struct {
	month string
	rows  int64
}

func (e *Exporter) monthCounts(ctx context.Context, d *Dataset) ([]monthCount, error) {
	rows, err := e.Service.DB.QueryContext(ctx, d.Months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []monthCount
	for rows.Next() {
		var m monthCount
		if err := rows.Scan(&m.month, &m.rows); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (e *Exporter) month(ctx context.Context, d *Dataset, dir, month string) (FileInfo, error) {
	start, err := time.ParseInLocation("2006-01", month, taipei)
	if err != nil {
		return FileInfo{}, err
	}
	name := month + ".csv.gz"
	tmp, err := os.CreateTemp(dir, "."+name+".*")
	if err != nil {
		return FileInfo{}, err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename
	hash := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(tmp, hash))
	gz.Name = d.ID + "-" + month + ".csv"
	cols := make([]string, len(d.Fields))
	for i, f := range d.Fields {
		cols[i] = f.Name
	}
	w, err := csvout.New(gz, cols)
	if err != nil {
		tmp.Close()
		return FileInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, exportTimeout)
	defer cancel()
	var n int64
	err = e.Service.stream(ctx, d.Fields, 2, d.HistorySQL,
		[]any{nil, nil, start, start.AddDate(0, 1, 0), nil, "", int64(1) << 62},
		func(row []any) error {
			n++
			return w.Row(row[:len(d.Fields)])
		})
	if err == nil {
		err = w.Flush()
	}
	if err == nil {
		err = gz.Close()
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return FileInfo{}, err
	}
	st, err := os.Stat(tmp.Name())
	if err != nil {
		return FileInfo{}, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return FileInfo{}, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Month: month, File: name, Rows: n, Bytes: st.Size(),
		SHA256: hex.EncodeToString(hash.Sum(nil)), GeneratedAt: e.Now().In(taipei),
	}, nil
}

func readIndex(dir string) (Index, error) {
	var idx Index
	b, err := os.ReadFile(filepath.Join(dir, IndexFile))
	if errors.Is(err, fs.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return Index{}, nil
	}
	return idx, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".index.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
