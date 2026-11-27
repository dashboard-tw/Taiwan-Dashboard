package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

// 低溫特報、高溫資訊、濃霧特報不在 W-C0033-001 裡，要各自抓
var DefaultAdvisoryURLs = []string{
	cwa.Datastore + "W-C0033-004",
	cwa.Datastore + "W-C0033-005",
	cwa.Datastore + "W-C0033-010",
}

type Advisory struct {
	Event       string
	Effective   time.Time
	Onset       *time.Time
	Expires     *time.Time
	Headline    string
	Level       string
	Color       string
	Description string
	Instruction string
	Web         string
	Areas       []string // 新北市平溪區、彰化縣
}

func optRFC3339(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &t
}

func ParseAdvisories(body []byte) ([]Advisory, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Info []struct {
				Event       string `json:"event"`
				Effective   string `json:"effective"`
				Onset       string `json:"onset"`
				Expires     string `json:"expires"`
				Headline    string `json:"headline"`
				Description string `json:"description"`
				Instruction string `json:"instruction"`
				Web         string `json:"web"`
				Parameter   []struct {
					Name  string `json:"valueName"`
					Value string `json:"value"`
				} `json:"parameter"`
				Area []struct {
					Desc string `json:"areaDesc"`
				} `json:"area"`
			} `json:"info"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, fmt.Errorf("decode advisory: %w", err)
	}
	if file.Success != "true" {
		return nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []Advisory
	for _, in := range file.Records.Info {
		eff := optRFC3339(in.Effective)
		event := strings.TrimSpace(in.Event)
		if eff == nil || (event != "低溫" && event != "高溫" && event != "濃霧") {
			continue
		}
		a := Advisory{Event: event, Effective: *eff, Onset: optRFC3339(in.Onset), Expires: optRFC3339(in.Expires),
			Headline: strings.TrimSpace(in.Headline), Description: strings.TrimSpace(in.Description),
			Instruction: strings.TrimSpace(in.Instruction), Web: httpsOnly(in.Web)}
		for _, p := range in.Parameter {
			switch p.Name {
			case "severity_level":
				a.Level = strings.TrimSpace(p.Value)
			case "alert_color":
				a.Color = strings.TrimSpace(p.Value)
			}
		}
		if a.Level == "" && a.Color != "" {
			a.Level = a.Color + "燈號"
		}
		for _, ar := range in.Area {
			if d := strings.TrimSpace(ar.Desc); d != "" {
				a.Areas = append(a.Areas, d)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func httpsOnly(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "https://") {
		return s
	}
	return ""
}

type AdvisoryPipeline struct {
	URLs   []string
	APIKey string
}

func (p *AdvisoryPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "weather_advisory",
		Name:         "低溫特報、高溫資訊與濃霧特報",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/warning/W-C0033-004",
		UpdateFreq:   10 * time.Minute,
		// 平常沒有特報，資料本來就可能很久沒有新的
		StaleAfter: 365 * 24 * time.Hour,
	}
}

func (p *AdvisoryPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var list []Advisory
	for _, u := range p.URLs {
		body, err := cwa.Get(ctx, env, u, p.APIKey)
		if err != nil {
			return res, err
		}
		a, err := ParseAdvisories(body)
		if err != nil {
			return res, err
		}
		list = append(list, a...)
	}
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
	for _, a := range list {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.weather_advisory (event, effective, onset, expires, headline, level, color, description,
			                                   instruction, web)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (event, effective) DO UPDATE SET onset = EXCLUDED.onset, expires = EXCLUDED.expires,
				headline = EXCLUDED.headline, level = EXCLUDED.level, color = EXCLUDED.color,
				description = EXCLUDED.description, instruction = EXCLUDED.instruction, web = EXCLUDED.web
			WHERE (data.weather_advisory.expires, data.weather_advisory.level, data.weather_advisory.description)
			      IS DISTINCT FROM (EXCLUDED.expires, EXCLUDED.level, EXCLUDED.description)`,
			a.Event, a.Effective, a.Onset, a.Expires, pipeline.NullIfEmpty(a.Headline), pipeline.NullIfEmpty(a.Level),
			pipeline.NullIfEmpty(a.Color), pipeline.NullIfEmpty(a.Description), pipeline.NullIfEmpty(a.Instruction),
			pipeline.NullIfEmpty(a.Web))
		if err != nil {
			return res, fmt.Errorf("upsert advisory %s %v: %w", a.Event, a.Effective, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		if _, err := tx.ExecContext(ctx, `DELETE FROM data.weather_advisory_area WHERE event = $1 AND effective = $2`,
			a.Event, a.Effective); err != nil {
			return res, err
		}
		seen := map[[2]string]bool{}
		for _, desc := range a.Areas {
			code, ok := counties.CodeFromAddress(desc)
			if !ok {
				res.Warnf("%s特報的地區「%s」對不到縣市", a.Event, desc)
				continue
			}
			norm := pipeline.NormalizeCountyText(desc)
			town := ""
			for _, name := range countyNames(counties, code, norm) {
				if strings.HasPrefix(norm, name) {
					town = strings.TrimPrefix(norm, name)
					break
				}
			}
			k := [2]string{code, town}
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO data.weather_advisory_area (event, effective, county_code, town) VALUES ($1, $2, $3, $4)`,
				a.Event, a.Effective, code, town); err != nil {
				return res, fmt.Errorf("insert advisory area: %w", err)
			}
		}
		res.ObserveTime(a.Effective)
	}
	if len(list) == 0 {
		res.ObserveTime(env.Now())
	}
	return res, tx.Commit()
}

// countyNames returns the county's names that prefix the area text, longest first.
func countyNames(c *pipeline.Counties, code, area string) []string {
	var out []string
	for i := len([]rune(area)); i > 0; i-- {
		prefix := string([]rune(area)[:i])
		if got, ok := c.Code(prefix); ok && got == code {
			out = append(out, prefix)
		}
	}
	return out
}
