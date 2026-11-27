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
	ShakemapURL  string
	ReportType   string
	Remark       string
	Counties     []CountyIntensity // one per county, the largest
	Stations     []Station
}

// Station is one seismograph's reading in a report.
type Station struct {
	ID          string
	Name        string
	County      string // county name as written by the source
	Area        string
	Lat, Lon    *float64
	Intensity   string
	Rank        *int
	PGA, PGV    *float64 // the values used for the intensity scale
	PGAEW       *float64
	PGANS       *float64
	PGAV        *float64
	PGVEW       *float64
	PGVNS       *float64
	PGVV        *float64
	DistanceKm  *float64
	BackAzimuth *float64
	WaveURL     string
	Status      string
}

type rawMotion struct {
	EW    json.Number `json:"EWComponent"`
	NS    json.Number `json:"NSComponent"`
	V     json.Number `json:"VComponent"`
	Scale json.Number `json:"IntScaleValue"`
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
	ShakemapURI    string      `json:"ShakemapImageURI"`
	ReportType     string      `json:"ReportType"`
	ReportRemark   string      `json:"ReportRemark"`
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
			EqStation     []struct {
				ID          string      `json:"StationID"`
				Name        string      `json:"StationName"`
				Status      string      `json:"InfoStatus"`
				BackAzimuth json.Number `json:"BackAzimuth"`
				Distance    json.Number `json:"EpicenterDistance"`
				Intensity   string      `json:"SeismicIntensity"`
				Lat         json.Number `json:"StationLatitude"`
				Lon         json.Number `json:"StationLongitude"`
				WaveURI     string      `json:"WaveImageURI"`
				PGA         rawMotion   `json:"pga"`
				PGV         rawMotion   `json:"pgv"`
			} `json:"EqStation"`
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
			ID:          reportID(raw.Web),
			Location:    strings.Join(strings.Fields(raw.EarthquakeInfo.Epicenter.Location), " "),
			Color:       strings.TrimSpace(raw.ReportColor),
			Content:     strings.TrimSpace(raw.ReportContent),
			URL:         httpsOrEmpty(raw.Web),
			ImageURL:    httpsOrEmpty(raw.ReportImageURI),
			ShakemapURL: httpsOrEmpty(raw.ShakemapURI),
			ReportType:  strings.TrimSpace(raw.ReportType),
			Remark:      strings.TrimSpace(raw.ReportRemark),
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

		// 測站列在各縣市的地區下；「最大震度 X 級地區」那幾筆沒有測站
		seenStation := map[string]bool{}
		for _, a := range raw.Intensity.ShakingArea {
			county := strings.TrimSpace(a.CountyName)
			if strings.Contains(county, "、") {
				county = ""
			}
			for _, st := range a.EqStation {
				id := strings.TrimSpace(st.ID)
				if id == "" || seenStation[id] {
					continue
				}
				seenStation[id] = true
				s := Station{ID: id, Name: strings.TrimSpace(st.Name), County: county,
					Area: strings.TrimSpace(a.AreaDesc), Intensity: strings.TrimSpace(st.Intensity),
					PGA: number(st.PGA.Scale), PGAEW: number(st.PGA.EW), PGANS: number(st.PGA.NS), PGAV: number(st.PGA.V),
					PGV: number(st.PGV.Scale), PGVEW: number(st.PGV.EW), PGVNS: number(st.PGV.NS), PGVV: number(st.PGV.V),
					DistanceKm: number(st.Distance), BackAzimuth: number(st.BackAzimuth),
					WaveURL: httpsOrEmpty(st.WaveURI), Status: strings.TrimSpace(st.Status)}
				if s.Name == "" {
					s.Name = id
				}
				if rank, ok := IntensityRank(st.Intensity); ok {
					s.Rank = &rank
				}
				lat, lon := number(st.Lat), number(st.Lon)
				if lat != nil && lon != nil && *lat >= 20 && *lat <= 27 && *lon >= 117 && *lon <= 123 {
					s.Lat, s.Lon = lat, lon
				}
				r.Stations = append(r.Stations, s)
			}
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
				 max_intensity, max_intensity_rank, report_color, content, report_url, image_url,
				 shakemap_url, report_type, report_remark)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			        CASE WHEN $9::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($9, $10), 4326) END,
			        $11, $12, $13, $14, $15, $16, $17, $18, $19)
			ON CONFLICT (report_id) DO UPDATE SET
				earthquake_no = EXCLUDED.earthquake_no, origin_time = EXCLUDED.origin_time,
				issued_at = EXCLUDED.issued_at, magnitude = EXCLUDED.magnitude, depth_km = EXCLUDED.depth_km,
				location = EXCLUDED.location, geom = EXCLUDED.geom, max_intensity = EXCLUDED.max_intensity,
				max_intensity_rank = EXCLUDED.max_intensity_rank, report_color = EXCLUDED.report_color,
				content = EXCLUDED.content, report_url = EXCLUDED.report_url, image_url = EXCLUDED.image_url,
				shakemap_url = EXCLUDED.shakemap_url, report_type = EXCLUDED.report_type,
				report_remark = EXCLUDED.report_remark, updated_at = now()
			WHERE (data.earthquake.issued_at, data.earthquake.magnitude, data.earthquake.depth_km,
			       data.earthquake.location, data.earthquake.max_intensity, data.earthquake.content,
			       data.earthquake.shakemap_url, data.earthquake.report_type, data.earthquake.report_remark)
			      IS DISTINCT FROM
			      (EXCLUDED.issued_at, EXCLUDED.magnitude, EXCLUDED.depth_km,
			       EXCLUDED.location, EXCLUDED.max_intensity, EXCLUDED.content,
			       EXCLUDED.shakemap_url, EXCLUDED.report_type, EXCLUDED.report_remark)
			RETURNING (xmax = 0)`,
			r.ID, p.Kind, r.EarthquakeNo, r.OriginTime, r.IssuedAt, r.Magnitude, r.DepthKm,
			pipeline.NullIfEmpty(r.Location), lon, lat, pipeline.NullIfEmpty(maxLabel), rank,
			pipeline.NullIfEmpty(r.Color), pipeline.NullIfEmpty(r.Content), pipeline.NullIfEmpty(r.URL),
			pipeline.NullIfEmpty(r.ImageURL), pipeline.NullIfEmpty(r.ShakemapURL), pipeline.NullIfEmpty(r.ReportType),
			pipeline.NullIfEmpty(r.Remark)).Scan(&inserted)
		unchanged := errors.Is(err, sql.ErrNoRows)
		if err != nil && !unchanged {
			return res, fmt.Errorf("upsert report %s: %w", r.ID, err)
		}
		if inserted {
			res.RowsInserted++
		}
		// 報告內容沒變時，只補還沒存過的測站資料
		if unchanged {
			var has bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM data.earthquake_station WHERE report_id = $1)`,
				r.ID).Scan(&has); err != nil {
				return res, err
			}
			if len(r.Stations) > 0 && !has {
				if err := saveStations(ctx, tx, counties, r); err != nil {
					return res, err
				}
			}
			continue
		}
		if err := saveStations(ctx, tx, counties, r); err != nil {
			return res, err
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

func saveStations(ctx context.Context, tx *sql.Tx, counties *pipeline.Counties, r Report) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.earthquake_station WHERE report_id = $1`, r.ID); err != nil {
		return err
	}
	for _, s := range r.Stations {
		var code any
		if c, ok := counties.Code(s.County); ok {
			code = c
		}
		var lon, lat any
		if s.Lon != nil {
			lon, lat = *s.Lon, *s.Lat
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.earthquake_station (report_id, station_id, name, county_code, area, geom, intensity,
				intensity_rank, pga_gal, pga_ew_gal, pga_ns_gal, pga_v_gal, pgv_kine, pgv_ew_kine, pgv_ns_kine,
				pgv_v_kine, distance_km, back_azimuth, wave_url, info_status)
			VALUES ($1, $2, $3, $4, $5,
			        CASE WHEN $6::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($6, $7::float8), 4326) END,
			        $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
			r.ID, s.ID, s.Name, code, pipeline.NullIfEmpty(s.Area), lon, lat, pipeline.NullIfEmpty(s.Intensity), s.Rank,
			s.PGA, s.PGAEW, s.PGANS, s.PGAV, s.PGV, s.PGVEW, s.PGVNS, s.PGVV, s.DistanceKm, s.BackAzimuth,
			pipeline.NullIfEmpty(s.WaveURL), pipeline.NullIfEmpty(s.Status)); err != nil {
			return fmt.Errorf("insert station %s %s: %w", r.ID, s.ID, err)
		}
	}
	return nil
}
