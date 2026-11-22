package aqi

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultUVURL = "https://data.moenv.gov.tw/api/v2/uv_s_01"

const maxUVI = 25

type UVObservation struct {
	SiteName   string
	County     string
	Agency     string
	Lng, Lat   *float64
	ObservedAt time.Time
	UVI        *float64
}

type UVPipeline struct {
	URL    string
	APIKey string
}

func (p *UVPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "uv_obs",
		Name:         "紫外線指數（每小時）",
		SourceAgency: "環境部、交通部中央氣象署",
		SourceURL:    "https://data.moenv.gov.tw/dataset/detail/UV_S_01",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

func ParseUV(body []byte) ([]UVObservation, []string, error) {
	records, err := decodeRecords(body)
	if err != nil {
		return nil, nil, err
	}
	var out []UVObservation
	var warnings []string
	for i, rec := range records {
		o, warn, err := parseUVRecord(rec)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（%s）：%v", i+1, rec["sitename"], err))
			continue
		}
		if warn != "" {
			warnings = append(warnings, fmt.Sprintf("%s %s：%s", o.SiteName, o.ObservedAt.Format("01/02 15:04"), warn))
		}
		out = append(out, o)
	}
	return out, warnings, nil
}

func parseUVRecord(rec map[string]string) (UVObservation, string, error) {
	o := UVObservation{
		SiteName: strings.TrimSpace(rec["sitename"]),
		County:   strings.TrimSpace(rec["county"]),
		Agency:   strings.TrimSpace(rec["unit"]),
	}
	if o.SiteName == "" {
		return o, "", fmt.Errorf("缺少測站名稱")
	}
	var err error
	if o.ObservedAt, err = pipeline.ParseTaipeiTime(rec["datacreationdate"], timeLayouts...); err != nil {
		return o, "", fmt.Errorf("觀測時間：%w", err)
	}
	if o.Lng, err = parseCoordinate(rec["wgs84_lon"], 114, 123); err != nil {
		return o, "", fmt.Errorf("經度：%w", err)
	}
	if o.Lat, err = parseCoordinate(rec["wgs84_lat"], 10, 27); err != nil {
		return o, "", fmt.Errorf("緯度：%w", err)
	}
	v, err := pipeline.ParseNumber(rec["uvi"])
	if err != nil {
		return o, "", fmt.Errorf("紫外線指數：%w", err)
	}
	warn := ""
	switch {
	case v == nil || *v < 0: // -999：儀器異常或無資料
	case *v > maxUVI:
		warn = fmt.Sprintf("紫外線指數 %v 不合理，視為無資料", *v)
	default:
		o.UVI = v
	}
	return o, warn, nil
}

func parseCoordinate(s string, lo, hi float64) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var v float64
	if parts := strings.Split(s, ","); len(parts) == 3 {
		var dms [3]float64
		for i, p := range parts {
			n, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%q 不是度分秒", s)
			}
			dms[i] = n
		}
		v = dms[0] + dms[1]/60 + dms[2]/3600
	} else {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("%q 不是座標", s)
		}
		v = n
	}
	if v < lo || v > hi {
		return nil, fmt.Errorf("%v 不在臺灣範圍內", v)
	}
	v = math.Round(v*1e6) / 1e6
	return &v, nil
}

func (p *UVPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.APIKey == "" {
		return res, fmt.Errorf("%w：未設定 MOENV_API_KEY", pipeline.ErrNotConfigured)
	}
	q := url.Values{"format": {"JSON"}, "limit": {"1000"}, "api_key": {p.APIKey}}
	body, err := env.Fetch.Get(ctx, p.URL+"?"+q.Encode(), p.URL+"?format=JSON&limit=1000&api_key=***")
	if err != nil {
		return res, err
	}
	obs, warnings, err := ParseUV(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(obs)
	if len(obs) == 0 {
		return res, fmt.Errorf("紫外線回應沒有任何測站資料")
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

	sites := map[string]int{}
	unknown := map[string]bool{}
	for _, o := range obs {
		id, ok := sites[o.SiteName]
		if !ok {
			var county *string
			if code, ok := counties.Code(o.County); ok {
				county = &code
			} else if !unknown[o.SiteName] {
				unknown[o.SiteName] = true
				res.Warnf("紫外線測站 %s 的縣市「%s」對不到縣市代碼", o.SiteName, o.County)
			}
			if id, err = upsertUVSite(ctx, tx, o, county); err != nil {
				return res, err
			}
			sites[o.SiteName] = id
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.uv_obs_history (site_id, observed_at, uvi) VALUES ($1, $2, $3)
			ON CONFLICT (site_id, observed_at) DO NOTHING`, id, o.ObservedAt, o.UVI)
		if err != nil {
			return res, fmt.Errorf("insert UV history for %s: %w", o.SiteName, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.uv_obs_current (site_id, observed_at, uvi) VALUES ($1, $2, $3)
			ON CONFLICT (site_id) DO UPDATE SET observed_at = EXCLUDED.observed_at, uvi = EXCLUDED.uvi, ingested_at = now()
			 WHERE data.uv_obs_current.observed_at <= EXCLUDED.observed_at`, id, o.ObservedAt, o.UVI); err != nil {
			return res, fmt.Errorf("upsert UV current for %s: %w", o.SiteName, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(o.ObservedAt)
	}
	return res, tx.Commit()
}

func upsertUVSite(ctx context.Context, tx *sql.Tx, o UVObservation, county *string) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO ref.uv_site (name, county_code, agency, geom, updated_at)
		VALUES ($1, $2, $3,
		        CASE WHEN $4::float8 IS NULL OR $5::float8 IS NULL THEN NULL
		             ELSE ST_SetSRID(ST_MakePoint($4::float8, $5::float8), 4326) END,
		        now())
		ON CONFLICT (name) DO UPDATE SET
			county_code = COALESCE(EXCLUDED.county_code, ref.uv_site.county_code),
			agency = COALESCE(NULLIF(EXCLUDED.agency, ''), ref.uv_site.agency),
			geom = COALESCE(EXCLUDED.geom, ref.uv_site.geom),
			updated_at = now()
		RETURNING site_id`, o.SiteName, county, o.Agency, o.Lng, o.Lat).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert UV site %s: %w", o.SiteName, err)
	}
	return id, nil
}
