package earthquake

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const (
	DefaultSignificantURL = cwa.Datastore + "E-A0015-001"
	DefaultLocalURL       = cwa.Datastore + "E-A0016-001"
)

var intensityRanks = map[string]int{
	"0級": 0, "1級": 1, "2級": 2, "3級": 3, "4級": 4,
	"5弱": 5, "5強": 6, "6弱": 7, "6強": 8, "7級": 9,
}

func IntensityRank(s string) (int, bool) {
	r, ok := intensityRanks[strings.TrimSpace(s)]
	return r, ok
}

type CountyIntensity struct {
	County    string // county name as written by the source
	Intensity string
	Rank      int
}

type Report struct {
	ID           string
	EarthquakeNo *int
	OriginTime   time.Time
	IssuedAt     *time.Time
	Magnitude    *float64
	DepthKm      *float64
	Location     string
	Lat, Lon     *float64
	Color        string
	Content      string
	URL          string
	ImageURL     string
	Counties     []CountyIntensity // one per county, the largest
}

func (r Report) MaxIntensity() (string, int) {
	label, rank := "", -1
	for _, c := range r.Counties {
		if c.Rank > rank {
			label, rank = c.Intensity, c.Rank
		}
	}
	return label, rank
}

type rawReport struct {
	EarthquakeNo   json.Number `json:"EarthquakeNo"`
	IssueTime      string      `json:"IssueTime"`
	ReportColor    string      `json:"ReportColor"`
	ReportContent  string      `json:"ReportContent"`
	ReportImageURI string      `json:"ReportImageURI"`
	Web            string      `json:"Web"`
	EarthquakeInfo struct {
		OriginTime string      `json:"OriginTime"`
		FocalDepth json.Number `json:"FocalDepth"`
		Epicenter  struct {
			Location  string      `json:"Location"`
			Latitude  json.Number `json:"EpicenterLatitude"`
			Longitude json.Number `json:"EpicenterLongitude"`
		} `json:"Epicenter"`
		EarthquakeMagnitude struct {
			MagnitudeValue json.Number `json:"MagnitudeValue"`
		} `json:"EarthquakeMagnitude"`
	} `json:"EarthquakeInfo"`
	Intensity struct {
		ShakingArea []struct {
			AreaDesc      string `json:"AreaDesc"`
			CountyName    string `json:"CountyName"`
			AreaIntensity string `json:"AreaIntensity"`
		} `json:"ShakingArea"`
	} `json:"Intensity"`
}

func number(n json.Number) *float64 {
	v, err := pipeline.ParseNumber(n.String())
	if err != nil {
		return nil
	}
	return v
}

func reportID(web string) string {
	u, err := url.Parse(strings.TrimSpace(web))
	if err != nil || u.Host == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	id := parts[len(parts)-1]
	for _, c := range id {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return id
}

func httpsOrEmpty(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "https://") {
		return s
	}
	return ""
}

func Parse(body []byte) ([]Report, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Earthquake []rawReport `json:"Earthquake"`
		} `json:"records"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&file); err != nil {
		return nil, nil, fmt.Errorf("decode earthquake reports: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}

	var out []Report
	var warnings []string
	for i, raw := range file.Records.Earthquake {
		r := Report{
			ID:       reportID(raw.Web),
			Location: strings.Join(strings.Fields(raw.EarthquakeInfo.Epicenter.Location), " "),
			Color:    strings.TrimSpace(raw.ReportColor),
			Content:  strings.TrimSpace(raw.ReportContent),
			URL:      httpsOrEmpty(raw.Web),
			ImageURL: httpsOrEmpty(raw.ReportImageURI),
		}
		if r.ID == "" {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆報告的網址「%s」沒有報告編號", i+1, raw.Web))
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.EarthquakeInfo.OriginTime))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("報告 %s 的發震時間：%v", r.ID, err))
			continue
		}
		r.OriginTime = t
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.IssueTime)); err == nil {
			r.IssuedAt = &t
		}
		if n, err := strconv.Atoi(raw.EarthquakeNo.String()); err == nil {
			r.EarthquakeNo = &n
		}
		r.Magnitude = number(raw.EarthquakeInfo.EarthquakeMagnitude.MagnitudeValue)
		r.DepthKm = number(raw.EarthquakeInfo.FocalDepth)
		lat, lon := number(raw.EarthquakeInfo.Epicenter.Latitude), number(raw.EarthquakeInfo.Epicenter.Longitude)
		if lat != nil && lon != nil && *lat >= 18 && *lat <= 28 && *lon >= 116 && *lon <= 126 {
			r.Lat, r.Lon = lat, lon
		} else if lat != nil || lon != nil {
			warnings = append(warnings, fmt.Sprintf("報告 %s 的震央座標超出範圍", r.ID))
		}

		best := map[string]CountyIntensity{}
		var order []string
		for _, a := range raw.Intensity.ShakingArea {
			desc := strings.TrimSpace(a.AreaDesc)
			if strings.HasPrefix(desc, "最大震度") {
				continue
			}
			county := strings.TrimSpace(a.CountyName)
			if county == "" || strings.Contains(county, "、") {
				continue
			}
			rank, ok := IntensityRank(a.AreaIntensity)
			if !ok {
				warnings = append(warnings, fmt.Sprintf("報告 %s 的震度「%s」無法辨識", r.ID, a.AreaIntensity))
				continue
			}
			if prev, seen := best[county]; !seen || rank > prev.Rank {
				if !seen {
					order = append(order, county)
				}
				best[county] = CountyIntensity{County: county, Intensity: strings.TrimSpace(a.AreaIntensity), Rank: rank}
			}
		}
		for _, c := range order {
			r.Counties = append(r.Counties, best[c])
		}
		out = append(out, r)
	}
	return out, warnings, nil
}

type Pipeline struct {
	Kind   string // significant or local
	URL    string
	APIKey string // CWA_API_KEY
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	if p.Kind == "local" {
		return pipeline.Dataset{
			ID:           "earthquake_local",
			Name:         "小區域有感地震報告",
			SourceAgency: "交通部中央氣象署",
			SourceURL:    "https://data.gov.tw/dataset/17202",
			UpdateFreq:   10 * time.Minute,
			StaleAfter:   2 * time.Hour,
		}
	}
	return pipeline.Dataset{
		ID:           "earthquake",
		Name:         "顯著有感地震報告",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://data.gov.tw/dataset/6068",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   2 * time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	reports, warnings, err := Parse(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(reports)

	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, r := range reports {
		maxLabel, maxRank := r.MaxIntensity()
		var rank any
		if maxRank >= 0 {
			rank = maxRank
		}
		var lon, lat any
		if r.Lon != nil {
			lon, lat = *r.Lon, *r.Lat
		}
		var inserted bool
		err := tx.QueryRowContext(ctx, `
			INSERT INTO data.earthquake
				(report_id, kind, earthquake_no, origin_time, issued_at, magnitude, depth_km, location, geom,
				 max_intensity, max_intensity_rank, report_color, content, report_url, image_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			        CASE WHEN $9::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($9, $10), 4326) END,
			        $11, $12, $13, $14, $15, $16)
			ON CONFLICT (report_id) DO UPDATE SET
				earthquake_no = EXCLUDED.earthquake_no, origin_time = EXCLUDED.origin_time,
				issued_at = EXCLUDED.issued_at, magnitude = EXCLUDED.magnitude, depth_km = EXCLUDED.depth_km,
				location = EXCLUDED.location, geom = EXCLUDED.geom, max_intensity = EXCLUDED.max_intensity,
				max_intensity_rank = EXCLUDED.max_intensity_rank, report_color = EXCLUDED.report_color,
				content = EXCLUDED.content, report_url = EXCLUDED.report_url, image_url = EXCLUDED.image_url,
				updated_at = now()
			WHERE (data.earthquake.issued_at, data.earthquake.magnitude, data.earthquake.depth_km,
			       data.earthquake.location, data.earthquake.max_intensity, data.earthquake.content)
			      IS DISTINCT FROM
			      (EXCLUDED.issued_at, EXCLUDED.magnitude, EXCLUDED.depth_km,
			       EXCLUDED.location, EXCLUDED.max_intensity, EXCLUDED.content)
			RETURNING (xmax = 0)`,
			r.ID, p.Kind, r.EarthquakeNo, r.OriginTime, r.IssuedAt, r.Magnitude, r.DepthKm,
			pipeline.NullIfEmpty(r.Location), lon, lat, pipeline.NullIfEmpty(maxLabel), rank,
			pipeline.NullIfEmpty(r.Color), pipeline.NullIfEmpty(r.Content), pipeline.NullIfEmpty(r.URL),
			pipeline.NullIfEmpty(r.ImageURL)).Scan(&inserted)
		if errors.Is(err, sql.ErrNoRows) {
			continue // 內容沒有變動
		}
		if err != nil {
			return res, fmt.Errorf("upsert report %s: %w", r.ID, err)
		}
		if inserted {
			res.RowsInserted++
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM data.earthquake_intensity WHERE report_id = $1`, r.ID); err != nil {
			return res, err
		}
		for _, c := range r.Counties {
			code, ok := counties.Code(c.County)
			if !ok {
				res.Warnf("報告 %s 的縣市「%s」無法辨識", r.ID, c.County)
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO data.earthquake_intensity (report_id, county_code, intensity, intensity_rank)
				VALUES ($1, $2, $3, $4)`, r.ID, code, c.Intensity, c.Rank); err != nil {
				return res, fmt.Errorf("insert intensity %s %s: %w", r.ID, c.County, err)
			}
		}
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
