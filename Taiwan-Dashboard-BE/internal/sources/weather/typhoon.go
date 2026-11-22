package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const (
	DefaultTyphoonURL        = cwa.Datastore + "W-C0034-005"
	DefaultTyphoonWarningURL = cwa.Datastore + "W-C0034-001"
)

type Fix struct {
	Forecast     bool
	IssuedAt     time.Time // analysis: the fix time; forecast: the initial time
	ForecastHour int
	ValidAt      time.Time
	Lon, Lat     float64
	Pressure     *int
	MaxWind      *int
	MaxGust      *int
	MovingSpeed  *int
	MovingDir    string
	Radius7      *int // km
	Radius10     *int
	Radius70     *int // 70% probability radius, forecasts only
	Note         string
}

type Cyclone struct {
	Year   int
	TdNo   int
	TyNo   *int
	NameEn string
	NameZh string
	Fixes  []Fix
}

func (c Cyclone) ID() string { return fmt.Sprintf("%d-%d", c.Year, c.TdNo) }

type rawText struct {
	Value string `json:"value"`
	Lang  string `json:"lang"`
}

type rawCircle struct {
	Radius string `json:"Radius"`
}

type rawFix struct {
	DateTime            string      `json:"DateTime"`
	InitialTime         string      `json:"InitialTime"`
	ForecastHour        string      `json:"ForecastHour"`
	Lon                 string      `json:"CoordinateLongitude"`
	Lat                 string      `json:"CoordinateLatitude"`
	MaxWindSpeed        string      `json:"MaxWindSpeed"`
	MaxGustSpeed        string      `json:"MaxGustSpeed"`
	Pressure            string      `json:"Pressure"`
	MovingSpeed         string      `json:"MovingSpeed"`
	MovingDirection     string      `json:"MovingDirection"`
	Circle15ms          *rawCircle  `json:"Circle15ms"`
	Circle25ms          *rawCircle  `json:"Circle25ms"`
	Radius70Probability string      `json:"Radius70PercentProbability"`
	StateTransfer       rawTextList `json:"StateTransfer"`
}

type rawTextList []rawText

func (l *rawTextList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '{' {
		var one rawText
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*l = rawTextList{one}
		return nil
	}
	var many []rawText
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

func (l rawTextList) zh() string {
	for _, t := range l {
		if strings.HasPrefix(t.Lang, "zh") {
			return strings.TrimSpace(t.Value)
		}
	}
	return ""
}

type rawFixList []rawFix

func (l *rawFixList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '{' {
		var one rawFix
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*l = rawFixList{one}
		return nil
	}
	var many []rawFix
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

func optInt(s string) *int {
	v, err := pipeline.ParseNumber(s)
	if err != nil || v == nil || *v < 0 {
		return nil
	}
	n := int(*v + 0.5)
	return &n
}

func circle(c *rawCircle) *int {
	if c == nil {
		return nil
	}
	return optInt(c.Radius)
}

func ParseTyphoons(body []byte) ([]Cyclone, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			TropicalCyclones struct {
				TropicalCyclone json.RawMessage `json:"TropicalCyclone"`
			} `json:"TropicalCyclones"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, fmt.Errorf("decode typhoon tracks: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	type rawCyclone struct {
		Year           string `json:"Year"`
		TyphoonName    string `json:"TyphoonName"`
		CwaTyphoonName string `json:"CwaTyphoonName"`
		CwaTdNo        string `json:"CwaTdNo"`
		CwaTyNo        string `json:"CwaTyNo"`
		AnalysisData   struct {
			Fix rawFixList `json:"Fix"`
		} `json:"AnalysisData"`
		ForecastData struct {
			Fix rawFixList `json:"Fix"`
		} `json:"ForecastData"`
	}
	var raws []rawCyclone
	if raw := bytes.TrimSpace(file.Records.TropicalCyclones.TropicalCyclone); len(raw) > 0 && string(raw) != "null" {
		if raw[0] == '{' {
			var one rawCyclone
			if err := json.Unmarshal(raw, &one); err != nil {
				return nil, nil, fmt.Errorf("decode tropical cyclone: %w", err)
			}
			raws = []rawCyclone{one}
		} else if err := json.Unmarshal(raw, &raws); err != nil {
			return nil, nil, fmt.Errorf("decode tropical cyclones: %w", err)
		}
	}

	var out []Cyclone
	var warnings []string
	for _, r := range raws {
		year, err1 := strconv.Atoi(strings.TrimSpace(r.Year))
		td, err2 := strconv.Atoi(strings.TrimSpace(r.CwaTdNo))
		if err1 != nil || err2 != nil {
			warnings = append(warnings, fmt.Sprintf("熱帶氣旋「%s」沒有年份或編號", r.TyphoonName))
			continue
		}
		c := Cyclone{Year: year, TdNo: td, TyNo: optInt(r.CwaTyNo),
			NameEn: strings.TrimSpace(r.TyphoonName), NameZh: strings.TrimSpace(r.CwaTyphoonName)}
		add := func(f rawFix, forecast bool) {
			lon, errLon := strconv.ParseFloat(strings.TrimSpace(f.Lon), 64)
			lat, errLat := strconv.ParseFloat(strings.TrimSpace(f.Lat), 64)
			if errLon != nil || errLat != nil || lon < 90 || lon > 200 || lat < -10 || lat > 60 {
				warnings = append(warnings, fmt.Sprintf("%s 的座標「%s, %s」無法使用", c.ID(), f.Lon, f.Lat))
				return
			}
			fx := Fix{Forecast: forecast, Lon: lon, Lat: lat,
				Pressure: optInt(f.Pressure), MaxWind: optInt(f.MaxWindSpeed), MaxGust: optInt(f.MaxGustSpeed),
				MovingSpeed: optInt(f.MovingSpeed), MovingDir: strings.TrimSpace(f.MovingDirection),
				Radius7: circle(f.Circle15ms), Radius10: circle(f.Circle25ms), Note: f.StateTransfer.zh()}
			ts := f.DateTime
			if forecast {
				ts = f.InitialTime
				fx.Radius70 = optInt(f.Radius70Probability)
				if h := optInt(f.ForecastHour); h != nil {
					fx.ForecastHour = *h
				}
			}
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s 的時間「%s」無法解析", c.ID(), ts))
				return
			}
			fx.IssuedAt = t
			fx.ValidAt = t.Add(time.Duration(fx.ForecastHour) * time.Hour)
			c.Fixes = append(c.Fixes, fx)
		}
		for _, f := range r.AnalysisData.Fix {
			add(f, false)
		}
		for _, f := range r.ForecastData.Fix {
			add(f, true)
		}
		out = append(out, c)
	}
	return out, warnings, nil
}

type TyphoonPipeline struct {
	URL    string
	APIKey string
}

func (p *TyphoonPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "typhoon_track",
		Name:         "颱風路徑（熱帶氣旋）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/warning/W-C0034-005",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

func (p *TyphoonPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	cyclones, warnings, err := ParseTyphoons(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	now := env.Now()

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, c := range cyclones {
		res.RowsFetched += len(c.Fixes)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.typhoon (typhoon_id, year, td_no, ty_no, name_en, name_zh, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
			ON CONFLICT (typhoon_id) DO UPDATE SET
				ty_no = coalesce(EXCLUDED.ty_no, data.typhoon.ty_no),
				name_en = coalesce(EXCLUDED.name_en, data.typhoon.name_en),
				name_zh = coalesce(EXCLUDED.name_zh, data.typhoon.name_zh),
				last_seen_at = EXCLUDED.last_seen_at`,
			c.ID(), c.Year, c.TdNo, c.TyNo, pipeline.NullIfEmpty(c.NameEn), pipeline.NullIfEmpty(c.NameZh), now); err != nil {
			return res, fmt.Errorf("upsert typhoon %s: %w", c.ID(), err)
		}
		for _, f := range c.Fixes {
			kind := "analysis"
			if f.Forecast {
				kind = "forecast"
			}
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.typhoon_fix
					(typhoon_id, kind, issued_at, forecast_hour, valid_at, geom, pressure_hpa, max_wind_ms, max_gust_ms,
					 moving_speed_kmh, moving_dir, radius7_km, radius10_km, radius70_km, note)
				VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($6, $7), 4326), $8, $9, $10, $11, $12, $13, $14, $15, $16)
				ON CONFLICT (typhoon_id, kind, issued_at, forecast_hour) DO UPDATE SET
					geom = EXCLUDED.geom, pressure_hpa = EXCLUDED.pressure_hpa, max_wind_ms = EXCLUDED.max_wind_ms,
					max_gust_ms = EXCLUDED.max_gust_ms, moving_speed_kmh = EXCLUDED.moving_speed_kmh,
					moving_dir = EXCLUDED.moving_dir, radius7_km = EXCLUDED.radius7_km,
					radius10_km = EXCLUDED.radius10_km, radius70_km = EXCLUDED.radius70_km, note = EXCLUDED.note
				WHERE (data.typhoon_fix.geom, data.typhoon_fix.pressure_hpa, data.typhoon_fix.max_wind_ms,
				       data.typhoon_fix.radius7_km, data.typhoon_fix.radius10_km, data.typhoon_fix.radius70_km)
				      IS DISTINCT FROM
				      (EXCLUDED.geom, EXCLUDED.pressure_hpa, EXCLUDED.max_wind_ms,
				       EXCLUDED.radius7_km, EXCLUDED.radius10_km, EXCLUDED.radius70_km)`,
				c.ID(), kind, f.IssuedAt, f.ForecastHour, f.ValidAt, f.Lon, f.Lat, f.Pressure, f.MaxWind, f.MaxGust,
				f.MovingSpeed, pipeline.NullIfEmpty(f.MovingDir), f.Radius7, f.Radius10, f.Radius70, pipeline.NullIfEmpty(f.Note))
			if err != nil {
				return res, fmt.Errorf("upsert fix %s: %w", c.ID(), err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
	}
	res.ObserveTime(now)
	return res, tx.Commit()
}

type TyphoonWarning struct {
	EffectiveAt time.Time
	ExpiresAt   *time.Time
	TyNo        int
	WarningNo   *int
	NameZh      string
	NameEn      string
	Alert       string
	Sections    []Section
	Areas       []string
	URL         string
}

type Section struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

func ParseTyphoonWarnings(body []byte) ([]TyphoonWarning, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Info json.RawMessage `json:"info"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, fmt.Errorf("decode typhoon warning: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	type rawInfo struct {
		Effective   string `json:"effective"`
		Expires     string `json:"expires"`
		Headline    string `json:"headline"`
		Web         string `json:"web"`
		Description struct {
			Section     []Section `json:"section"`
			TyphoonInfo []struct {
				Section []struct {
					Title    string          `json:"title"`
					Value    json.RawMessage `json:"value"`
					Analysis json.RawMessage `json:"analysis"`
					NameEn   string          `json:"typhoon_name"`
					NameZh   string          `json:"cwa_typhoon_name"`
				} `json:"section"`
			} `json:"typhoon-info"`
		} `json:"description"`
		Parameter []struct {
			ValueName string `json:"valueName"`
			Value     string `json:"value"`
		} `json:"parameter"`
		Area []struct {
			AreaDesc string `json:"areaDesc"`
		} `json:"area"`
	}
	var infos []rawInfo
	if raw := bytes.TrimSpace(file.Records.Info); len(raw) > 0 && string(raw) != "null" {
		if raw[0] == '{' {
			var one rawInfo
			if err := json.Unmarshal(raw, &one); err != nil {
				return nil, nil, fmt.Errorf("decode warning info: %w", err)
			}
			infos = []rawInfo{one}
		} else if err := json.Unmarshal(raw, &infos); err != nil {
			return nil, nil, fmt.Errorf("decode warning infos: %w", err)
		}
	}

	var out []TyphoonWarning
	var warnings []string
	for _, in := range infos {
		w := TyphoonWarning{Alert: strings.TrimSpace(in.Headline), URL: strings.TrimSpace(in.Web), Sections: in.Description.Section}
		if !strings.HasPrefix(w.URL, "https://") {
			w.URL = ""
		}
		for _, p := range in.Parameter {
			if p.ValueName == "severity_level" && strings.TrimSpace(p.Value) != "" {
				w.Alert = strings.TrimSpace(p.Value)
			}
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(in.Effective))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("颱風警報的發布時間「%s」無法解析", in.Effective))
			continue
		}
		w.EffectiveAt = t
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(in.Expires)); err == nil {
			w.ExpiresAt = &t
		}
		for _, ti := range in.Description.TyphoonInfo {
			for _, s := range ti.Section {
				var v string
				_ = json.Unmarshal(s.Value, &v)
				switch s.Title {
				case "颱風編號":
					if n := optInt(v); n != nil {
						w.TyNo = *n
					}
				case "警報報數":
					w.WarningNo = optInt(v)
				}
				if s.NameZh != "" {
					w.NameZh, w.NameEn = s.NameZh, s.NameEn
				}
			}
		}
		if w.TyNo == 0 {
			warnings = append(warnings, fmt.Sprintf("%s 的颱風警報沒有颱風編號", t.Format(time.DateTime)))
			continue
		}
		for _, a := range in.Area {
			if d := strings.TrimSpace(a.AreaDesc); d != "" {
				w.Areas = append(w.Areas, d)
			}
		}
		if w.Sections == nil {
			w.Sections = []Section{}
		}
		out = append(out, w)
	}
	return out, warnings, nil
}

type TyphoonWarningPipeline struct {
	URL    string
	APIKey string
}

func (p *TyphoonWarningPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "typhoon_warning",
		Name:         "颱風警報",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/warning/W-C0034-001",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

func (p *TyphoonWarningPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	list, warnings, err := ParseTyphoonWarnings(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(list)
	for _, w := range list {
		sections, err := json.Marshal(w.Sections)
		if err != nil {
			return res, err
		}
		if w.Areas == nil {
			w.Areas = []string{}
		}
		r, err := env.DB.ExecContext(ctx, `
			INSERT INTO data.typhoon_warning
				(effective_at, ty_no, warning_no, name_zh, name_en, alert, expires_at, sections, areas, url)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (ty_no, effective_at) DO NOTHING`,
			w.EffectiveAt, w.TyNo, w.WarningNo, pipeline.NullIfEmpty(w.NameZh), pipeline.NullIfEmpty(w.NameEn),
			w.Alert, w.ExpiresAt, string(sections), w.Areas, pipeline.NullIfEmpty(w.URL))
		if err != nil {
			return res, fmt.Errorf("insert typhoon warning: %w", err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, nil
}
