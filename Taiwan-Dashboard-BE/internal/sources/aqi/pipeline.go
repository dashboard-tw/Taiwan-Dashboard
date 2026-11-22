package aqi

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://data.moenv.gov.tw/api/v2/aqx_p_432"

type Pipeline struct {
	URL    string
	APIKey string // MOENV_API_KEY; the API rejects requests without one
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "aqi_obs",
		Name:         "空氣品質指標（每小時）",
		SourceAgency: "環境部",
		SourceURL:    "https://data.gov.tw/dataset/40448",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.APIKey == "" {
		return res, fmt.Errorf("%w：未設定 MOENV_API_KEY", pipeline.ErrNotConfigured)
	}

	q := url.Values{"format": {"JSON"}, "limit": {"1000"}, "api_key": {p.APIKey}}
	redacted := p.URL + "?format=JSON&limit=1000&api_key=***"
	body, err := env.Fetch.Get(ctx, p.URL+"?"+q.Encode(), redacted)
	if err != nil {
		return res, err
	}

	obs, warnings, err := Parse(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(obs)
	if len(obs) == 0 {
		return res, fmt.Errorf("AQI 回應沒有任何測站資料")
	}

	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, o := range obs {
		var county *string
		if code, ok := counties.Code(o.County); ok {
			county = &code
		} else {
			res.Warnf("測站 %s（%d）的縣市「%s」對不到縣市代碼", o.SiteName, o.SiteID, o.County)
		}
		if err := upsertSite(ctx, tx, o, county); err != nil {
			return res, err
		}
		n, err := insertObservation(ctx, tx, o)
		if err != nil {
			return res, err
		}
		res.RowsInserted += n
		res.ObserveTime(o.ObservedAt)
	}
	return res, tx.Commit()
}

func upsertSite(ctx context.Context, tx *sql.Tx, o Observation, county *string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ref.aqi_site (site_id, name, county_code, geom, updated_at)
		VALUES ($1, $2, $3,
		        CASE WHEN $4::float8 IS NULL OR $5::float8 IS NULL THEN NULL
		             ELSE ST_SetSRID(ST_MakePoint($4::float8, $5::float8), 4326) END,
		        now())
		ON CONFLICT (site_id) DO UPDATE SET
			name = EXCLUDED.name,
			county_code = COALESCE(EXCLUDED.county_code, ref.aqi_site.county_code),
			geom = COALESCE(EXCLUDED.geom, ref.aqi_site.geom),
			updated_at = now()`,
		o.SiteID, o.SiteName, county, o.Lng, o.Lat)
	if err != nil {
		return fmt.Errorf("upsert site %d: %w", o.SiteID, err)
	}
	return nil
}

var obsColumns = func() []string {
	cols := []string{"site_id", "observed_at", "aqi", "status", "pollutant"}
	for _, f := range numericFields {
		cols = append(cols, f.column)
	}
	return cols
}()

func obsArgs(o Observation) []any {
	args := []any{o.SiteID, o.ObservedAt, o.AQI, pipeline.NullIfEmpty(o.Status), pipeline.NullIfEmpty(o.Pollutant)}
	for _, f := range numericFields {
		args = append(args, o.Values[f.column])
	}
	return args
}

var (
	insertHistorySQL = fmt.Sprintf(
		`INSERT INTO data.aqi_obs_history (%s) VALUES (%s) ON CONFLICT (site_id, observed_at) DO NOTHING`,
		strings.Join(obsColumns, ", "), pipeline.Placeholders(len(obsColumns)))

	upsertCurrentSQL = fmt.Sprintf(
		`INSERT INTO data.aqi_obs_current (%s) VALUES (%s)
		 ON CONFLICT (site_id) DO UPDATE SET %s, ingested_at = now()
		 WHERE data.aqi_obs_current.observed_at <= EXCLUDED.observed_at`,
		strings.Join(obsColumns, ", "), pipeline.Placeholders(len(obsColumns)), pipeline.SetExcluded(obsColumns[1:]))
)

func insertObservation(ctx context.Context, tx *sql.Tx, o Observation) (int, error) {
	args := obsArgs(o)
	r, err := tx.ExecContext(ctx, insertHistorySQL, args...)
	if err != nil {
		return 0, fmt.Errorf("insert AQI history for site %d: %w", o.SiteID, err)
	}
	if _, err := tx.ExecContext(ctx, upsertCurrentSQL, args...); err != nil {
		return 0, fmt.Errorf("upsert AQI current for site %d: %w", o.SiteID, err)
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}
