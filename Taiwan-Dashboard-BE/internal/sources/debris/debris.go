package debris

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://data.moa.gov.tw/Service/OpenData/TransService.aspx?UnitId=ofvJwoWC5wum&IsTransData=1"

type Alert struct {
	Type, ID, Name               string
	County, Town, Village, Level string // Level: yellow, red
	Updated                      *time.Time
}

func Parse(body []byte) ([]Alert, error) {
	var rows []struct {
		Type      string `json:"AlertType"`
		DebrisNo  string `json:"DebrisNo"`
		SlideID   string `json:"LandslideID"`
		SlideName string `json:"LandslideName"`
		County    string `json:"County"`
		Town      string `json:"Town"`
		Village   string `json:"Vill"`
		Level     string `json:"AlertLevel"`
		Updated   string `json:"LastUpdateDate"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode debris alerts: %w", err)
	}
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "-" {
			return ""
		}
		return s
	}
	var out []Alert
	for _, r := range rows {
		a := Alert{Type: strings.ToUpper(clean(r.Type)), County: clean(r.County), Town: clean(r.Town), Village: clean(r.Village)}
		switch a.Type {
		case "D":
			a.ID = clean(r.DebrisNo)
		case "L":
			a.ID, a.Name = clean(r.SlideID), clean(r.SlideName)
		}
		switch strings.ToLower(clean(r.Level)) {
		case "y":
			a.Level = "yellow"
		case "r":
			a.Level = "red"
		}
		if a.ID == "" || a.Level == "" || len(a.ID) > 40 {
			continue
		}
		if t, err := pipeline.ParseTaipeiTime(clean(r.Updated), "2006-01-02T15:04:05", "2006-01-02 15:04"); err == nil {
			a.Updated = &t
		}
		out = append(out, a)
	}
	return out, nil
}

type Pipeline struct {
	URL string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "debris_alert",
		Name:         "土石流及大規模崩塌警戒",
		SourceAgency: "農業部農村發展及水土保持署",
		SourceURL:    "https://data.gov.tw/dataset/148402",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	alerts, err := Parse(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(alerts)
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	now := env.Now()
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for _, a := range alerts {
		if seen[a.Type+a.ID] {
			continue
		}
		seen[a.Type+a.ID] = true
		var county any
		if c, ok := counties.Code(a.County); ok {
			county = c
		}
		r, err := tx.ExecContext(ctx, `
			UPDATE data.debris_alert SET last_seen_at = $3, updated_at = $4,
				level = CASE WHEN level = 'red' OR $5 = 'red' THEN 'red' ELSE 'yellow' END
			WHERE alert_type = $1 AND alert_id = $2 AND last_seen_at >= $3::timestamptz - interval '30 minutes'`,
			a.Type, a.ID, now, a.Updated, a.Level)
		if err != nil {
			return res, fmt.Errorf("update debris alert %s: %w", a.ID, err)
		}
		if n, _ := r.RowsAffected(); n > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.debris_alert (alert_type, alert_id, name, county_code, town, village, level, updated_at,
			                               first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
			a.Type, a.ID, pipeline.NullIfEmpty(a.Name), county, pipeline.NullIfEmpty(a.Town), pipeline.NullIfEmpty(a.Village),
			a.Level, a.Updated, now); err != nil {
			return res, fmt.Errorf("insert debris alert %s: %w", a.ID, err)
		}
		res.RowsInserted++
	}
	res.ObserveTime(now)
	return res, tx.Commit()
}
