package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const DefaultWarningURL = cwa.Datastore + "W-C0033-001"

type CountyWarning struct {
	County       string // county name
	Phenomena    string // 海上陸上颱風、陸上強風、豪雨…
	Significance string // 警報、特報
	StartsAt     time.Time
	EndsAt       *time.Time
}

func (w CountyWarning) Label() string { return w.Phenomena + w.Significance }

func ParseWarnings(body []byte) ([]CountyWarning, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Location []struct {
				LocationName     string `json:"locationName"`
				HazardConditions struct {
					Hazards json.RawMessage `json:"hazards"`
				} `json:"hazardConditions"`
			} `json:"location"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, fmt.Errorf("decode weather warnings: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	type rawHazard struct {
		Info struct {
			Phenomena    string `json:"phenomena"`
			Significance string `json:"significance"`
		} `json:"info"`
		ValidTime struct {
			StartTime string `json:"startTime"`
			EndTime   string `json:"endTime"`
		} `json:"validTime"`
	}
	var out []CountyWarning
	var warnings []string
	for _, loc := range file.Records.Location {
		raw := bytes.TrimSpace(loc.HazardConditions.Hazards)
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var hazards []rawHazard
		if raw[0] == '{' {
			var one rawHazard
			if err := json.Unmarshal(raw, &one); err != nil {
				return nil, nil, fmt.Errorf("decode hazard of %s: %w", loc.LocationName, err)
			}
			hazards = []rawHazard{one}
		} else if err := json.Unmarshal(raw, &hazards); err != nil {
			return nil, nil, fmt.Errorf("decode hazards of %s: %w", loc.LocationName, err)
		}
		for _, h := range hazards {
			w := CountyWarning{
				County:       strings.TrimSpace(loc.LocationName),
				Phenomena:    strings.TrimSpace(h.Info.Phenomena),
				Significance: strings.TrimSpace(h.Info.Significance),
			}
			if w.Phenomena == "" {
				continue
			}
			t, err := pipeline.ParseTaipeiTime(h.ValidTime.StartTime, "2006-01-02 15:04:05", time.RFC3339)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s %s 的開始時間：%v", w.County, w.Label(), err))
				continue
			}
			w.StartsAt = t
			if t, err := pipeline.ParseTaipeiTime(h.ValidTime.EndTime, "2006-01-02 15:04:05", time.RFC3339); err == nil {
				w.EndsAt = &t
			}
			out = append(out, w)
		}
	}
	return out, warnings, nil
}

type WarningPipeline struct {
	URL    string
	APIKey string
}

func (p *WarningPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "weather_warning",
		Name:         "天氣警特報（各縣市）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/warning/W-C0033-001",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *WarningPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	list, warnings, err := ParseWarnings(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(list)

	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	now := env.Now()
	for _, w := range list {
		code, ok := counties.Code(w.County)
		if !ok {
			res.Warnf("縣市「%s」無法辨識", w.County)
			continue
		}
		var inserted bool
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO data.weather_warning
				(county_code, phenomena, significance, starts_at, ends_at, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6)
			ON CONFLICT (county_code, phenomena, significance, starts_at) DO UPDATE SET
				ends_at = EXCLUDED.ends_at, last_seen_at = EXCLUDED.last_seen_at
			RETURNING (xmax = 0)`,
			code, w.Phenomena, w.Significance, w.StartsAt, w.EndsAt, now).Scan(&inserted); err != nil {
			return res, fmt.Errorf("upsert warning %s %s: %w", w.County, w.Label(), err)
		}
		if inserted {
			res.RowsInserted++
		}
	}
	res.ObserveTime(now)
	return res, tx.Commit()
}
