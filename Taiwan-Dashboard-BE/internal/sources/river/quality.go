package river

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultQualityURL = "https://data.moenv.gov.tw/api/v2/wqx_p_01"

type Sample struct {
	SiteID, Name, County, Township, Basin, River string
	SampledAt                                    time.Time
	RPI                                          float64
	Lon, Lat                                     *float64
}

func ParseQuality(body []byte) ([]Sample, int, error) {
	var rows []map[string]string
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		if s := strings.TrimSpace(string(body)); strings.Contains(s, "查無資料") {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("decode water quality: %w", err)
	}
	var out []Sample
	for _, r := range rows {
		if r["itemengabbreviation"] != "RPI" {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(r["itemvalue"]), 64)
		if err != nil || v < 1 || v > 10 {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(r["sampledate"]), pipeline.Taipei)
		if err != nil {
			continue
		}
		s := Sample{SiteID: strings.TrimSpace(r["siteid"]), Name: strings.TrimSpace(r["sitename"]), County: r["county"],
			Township: r["township"], Basin: r["basin"], River: r["river"], SampledAt: at, RPI: v}
		if s.SiteID == "" || s.Name == "" || len(s.SiteID) > 20 {
			continue
		}
		lon, err1 := pipeline.ParseNumber(r["twd97lon"])
		lat, err2 := pipeline.ParseNumber(r["twd97lat"])
		if err1 == nil && err2 == nil && lon != nil && lat != nil && *lat > 21 && *lat < 27 && *lon > 118 && *lon < 123 {
			s.Lon, s.Lat = lon, lat
		}
		out = append(out, s)
	}
	return out, len(rows), nil
}

type QualityPipeline struct {
	URL    string
	APIKey string // MOENV_API_KEY
}

func (p *QualityPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "river_quality",
		Name:         "河川水質（河川污染指數）",
		SourceAgency: "環境部",
		SourceURL:    "https://data.moenv.gov.tw/dataset/detail/WQX_P_01",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   120 * 24 * time.Hour,
	}
}

const qualityPages = 30

func (p *QualityPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.APIKey == "" {
		return res, fmt.Errorf("%w：未設定 MOENV_API_KEY", pipeline.ErrNotConfigured)
	}
	since := env.Now().In(pipeline.Taipei).AddDate(0, -13, 0).Format("2006-01-02 15:04:05")
	var all []Sample
	for page := 0; page < qualityPages; page++ {
		offset := strconv.Itoa(page * 1000)
		q := url.Values{"format": {"JSON"}, "limit": {"1000"}, "offset": {offset}, "api_key": {p.APIKey},
			"filters": {"itemengabbreviation,EQ,RPI|sampledate,GR," + since}}
		redacted := p.URL + "?format=JSON&limit=1000&offset=" + offset + "&api_key=***"
		body, err := env.Fetch.Get(ctx, p.URL+"?"+q.Encode(), redacted)
		if err != nil {
			return res, err
		}
		list, n, err := ParseQuality(body)
		if err != nil {
			return res, err
		}
		all = append(all, list...)
		if n < 1000 {
			break
		}
	}
	res.RowsFetched = len(all)
	if len(all) == 0 {
		return res, errors.New("最近一年沒有河川污染指數")
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
	sites := map[string]bool{}
	unknown := 0
	for _, s := range all {
		if !sites[s.SiteID] {
			sites[s.SiteID] = true
			var county, lon, lat any
			if c, ok := counties.Code(s.County); ok {
				county = c
			} else {
				unknown++
			}
			if s.Lon != nil {
				lon, lat = *s.Lon, *s.Lat
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO ref.river_quality_site (site_id, name, county_code, township, basin, river, geom)
				VALUES ($1, $2, $3, $4, $5, $6,
				        CASE WHEN $7::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($7, $8), 4326) END)
				ON CONFLICT (site_id) DO UPDATE SET
					name = EXCLUDED.name, county_code = EXCLUDED.county_code, township = EXCLUDED.township,
					basin = EXCLUDED.basin, river = EXCLUDED.river, geom = EXCLUDED.geom, last_seen_at = now()`,
				s.SiteID, s.Name, county, pipeline.NullIfEmpty(s.Township), pipeline.NullIfEmpty(s.Basin),
				pipeline.NullIfEmpty(s.River), lon, lat); err != nil {
				return res, fmt.Errorf("upsert water quality site %s: %w", s.SiteID, err)
			}
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.river_quality (site_id, sampled_at, rpi) VALUES ($1, $2, $3)
			ON CONFLICT (site_id, sampled_at) DO UPDATE SET rpi = EXCLUDED.rpi, ingested_at = now()
			WHERE data.river_quality.rpi IS DISTINCT FROM EXCLUDED.rpi`,
			s.SiteID, s.SampledAt, s.RPI)
		if err != nil {
			return res, fmt.Errorf("upsert water quality %s: %w", s.SiteID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(s.SampledAt)
	}
	if unknown > 0 {
		res.Warnf("%d 個水質測站對不到縣市", unknown)
	}
	return res, tx.Commit()
}
