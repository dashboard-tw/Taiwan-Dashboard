package opendata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dashboard.tw/backend/internal/component"
)

const (
	PageSize     = 1000
	MaxRange     = 31 * 24 * time.Hour
	DefaultRange = 24 * time.Hour
	queryTimeout = 10 * time.Second
)

var taipei = time.FixedZone("Asia/Taipei", 8*60*60)

var ErrBadCursor = errors.New("invalid cursor")

type Service struct {
	DB *sql.DB
}

type Summary struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Agency            string     `json:"agency"`
	License           string     `json:"license"`
	SourceURL         string     `json:"source_url"`
	Status            string     `json:"status"`
	UpdateFreqSeconds int64      `json:"update_freq_seconds"`
	LastObservedAt    *time.Time `json:"last_observed_at"`
	HasSites          bool       `json:"has_sites"`
	HasCounty         bool       `json:"has_county"`
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	ids := make([]string, len(Datasets))
	for i, d := range Datasets {
		ids[i] = d.ID
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT d.id, d.name, d.source_agency, d.license, d.source_url, `+component.DatasetStatusSQL("d")+`,
		       extract(epoch FROM d.update_freq)::bigint, d.last_observed_at
		  FROM app.dataset d WHERE d.id = ANY($1) ORDER BY array_position($1, d.id)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var x Summary
		if err := rows.Scan(&x.ID, &x.Name, &x.Agency, &x.License, &x.SourceURL, &x.Status,
			&x.UpdateFreqSeconds, &x.LastObservedAt); err != nil {
			return nil, err
		}
		d := Find(x.ID)
		x.Description, x.HasSites, x.HasCounty = d.Description, d.HasSite, d.HasCounty
		if x.LastObservedAt != nil {
			t := x.LastObservedAt.In(taipei)
			x.LastObservedAt = &t
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

type CatalogEntry struct {
	Summary
	Topic        string  `json:"topic"`
	HasHistory   bool    `json:"has_history"`
	LatestPaged  bool    `json:"latest_paged"`
	HasDownloads bool    `json:"has_downloads"`
	Fields       []Field `json:"fields"`
	LatestFields []Field `json:"latest_fields"`
	SiteFields   []Field `json:"site_fields"`
}

type Catalog struct {
	Topics       []Topic        `json:"topics"`
	Datasets     []CatalogEntry `json:"datasets"`
	PageSize     int            `json:"page_size"`
	MaxRangeDays int            `json:"max_range_days"`
}

func (s *Service) Catalog(ctx context.Context) (Catalog, error) {
	list, err := s.List(ctx)
	if err != nil {
		return Catalog{}, err
	}
	out := Catalog{Topics: Topics, Datasets: make([]CatalogEntry, 0, len(list)),
		PageSize: PageSize, MaxRangeDays: int(MaxRange.Hours() / 24)}
	for _, x := range list {
		d := Find(x.ID)
		sites := d.SiteFields
		if sites == nil {
			sites = []Field{}
		}
		out.Datasets = append(out.Datasets, CatalogEntry{
			Summary: x, Topic: TopicOf(d.ID), HasHistory: d.HasHistory(), LatestPaged: d.LatestPaged,
			HasDownloads: d.Months != "", Fields: d.Fields, LatestFields: d.LatestRowFields(), SiteFields: sites,
		})
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Summary, error) {
	if Find(id) == nil {
		return nil, nil
	}
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, nil
}

type Table struct {
	Fields []Field
	Rows   [][]any
}

func (t Table) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, r := range t.Rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('{')
		for j, f := range t.Fields {
			if j > 0 {
				b.WriteByte(',')
			}
			k, _ := json.Marshal(f.Name)
			v, err := json.Marshal(r[j])
			if err != nil {
				return nil, err
			}
			b.Write(k)
			b.WriteByte(':')
			b.Write(v)
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}

func (t Table) CSVRows() component.Rows {
	cols := make([]string, len(t.Fields))
	for i, f := range t.Fields {
		cols[i] = f.Name
	}
	return component.Rows{Columns: cols, Values: t.Rows}
}

func (s *Service) Sites(ctx context.Context, d *Dataset, county *string) (Table, error) {
	rows, err := s.query(ctx, d.SiteFields, 0, d.SitesSQL, county)
	return Table{Fields: d.SiteFields, Rows: rows}, err
}

func (s *Service) Latest(ctx context.Context, d *Dataset, county, site *string, cursorText string) (Table, string, error) {
	fields := d.LatestRowFields()
	if !d.LatestPaged {
		rows, err := s.query(ctx, fields, 0, d.LatestSQL, county, site)
		return Table{Fields: fields, Rows: rows}, "", err
	}
	var key *string
	if cursorText != "" {
		c, err := decodeCursor(cursorText)
		if err != nil {
			return Table{}, "", err
		}
		key = &c.K
	}
	rows, err := s.query(ctx, fields, 1, d.LatestSQL, county, site, key, PageSize+1)
	if err != nil {
		return Table{}, "", err
	}
	next := ""
	if len(rows) > PageSize {
		rows = rows[:PageSize]
		next = encodeCursor(cursor{T: listCursorTime, K: fmt.Sprint(rows[len(rows)-1][len(fields)])})
	}
	for i, r := range rows {
		rows[i] = r[:len(fields)]
	}
	return Table{Fields: fields, Rows: rows}, next, nil
}

var listCursorTime = time.Unix(1, 0).UTC()

type HistoryQuery struct {
	County, Site *string
	From, To     time.Time
	Cursor       string
}

func (s *Service) History(ctx context.Context, d *Dataset, q HistoryQuery) (Table, string, error) {
	var ct *time.Time
	var ck string
	if q.Cursor != "" {
		c, err := decodeCursor(q.Cursor)
		if err != nil {
			return Table{}, "", err
		}
		ct, ck = &c.T, c.K
	}
	rows, err := s.query(ctx, d.Fields, 2, d.HistorySQL, q.County, q.Site, q.From, q.To, ct, ck, PageSize+1)
	if err != nil {
		return Table{}, "", err
	}
	next := ""
	if len(rows) > PageSize {
		rows = rows[:PageSize]
		last := rows[len(rows)-1]
		next = encodeCursor(cursor{T: last[len(d.Fields)].(time.Time), K: fmt.Sprint(last[len(d.Fields)+1])})
	}
	for i, r := range rows {
		rows[i] = r[:len(d.Fields)]
	}
	return Table{Fields: d.Fields, Rows: rows}, next, nil
}

func (s *Service) query(ctx context.Context, fields []Field, extra int, query string, args ...any) ([][]any, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	out := [][]any{}
	err := s.stream(ctx, fields, extra, query, args, func(row []any) error {
		out = append(out, row)
		return nil
	})
	return out, err
}

func (s *Service) stream(ctx context.Context, fields []Field, extra int, query string, args []any, fn func([]any) error) error {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := len(fields) + extra
	for rows.Next() {
		vals := make([]any, n)
		ptrs := make([]any, n)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for i, f := range fields {
			vals[i] = convert(vals[i], f.Type)
		}
		if err := fn(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

func convert(v any, typ string) any {
	switch x := v.(type) {
	case time.Time:
		if typ == "date" {
			return x.Format(time.DateOnly)
		}
		return x.In(taipei).Format(time.RFC3339)
	case []byte:
		return string(x)
	case string:
		if typ == "string" {
			return trimRight(x)
		}
	}
	return v
}

func trimRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

type cursor struct {
	T time.Time `json:"t"`
	K string    `json:"k"`
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) > 200 || json.Unmarshal(b, &c) != nil || c.T.IsZero() {
		return cursor{}, ErrBadCursor
	}
	return c, nil
}

func (s *Service) CountUse(ctx context.Context, dataset string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO app.api_usage_dataset_daily AS u (day, dataset, requests)
		VALUES ((now() AT TIME ZONE 'Asia/Taipei')::date, $1, 1)
		ON CONFLICT (day, dataset) DO UPDATE SET requests = u.requests + 1`, dataset)
	return err
}
