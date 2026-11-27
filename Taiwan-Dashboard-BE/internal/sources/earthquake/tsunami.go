package earthquake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const DefaultTsunamiURL = cwa.Datastore + "E-A0014-001"

type TsunamiReport struct {
	TsunamiNo  int
	Seq        int
	IssuedAt   time.Time
	ValidUntil *time.Time
	Type       string
	Color      string
	Content    string
	URL        string
	OriginTime *time.Time
	Source     string
	Location   string
	Lat, Lon   *float64
	Magnitude  *float64
	DepthKm    *float64
}

// reportSeq turns 第2報 into 2.
func reportSeq(s string) (int, bool) {
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
	n, err := strconv.Atoi(digits)
	return n, err == nil && n > 0
}

func optTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &t
}

func ParseTsunami(body []byte) ([]TsunamiReport, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Tsunami []struct {
				IssueTime string `json:"IssueTime"`
				ValidTime struct {
					EndTime string `json:"EndTime"`
				} `json:"ValidTime"`
				ReportColor    string      `json:"ReportColor"`
				ReportContent  string      `json:"ReportContent"`
				ReportNo       string      `json:"ReportNo"`
				ReportType     string      `json:"ReportType"`
				TsunamiNo      json.Number `json:"TsunamiNo"`
				Web            string      `json:"Web"`
				EarthquakeInfo struct {
					OriginTime string      `json:"OriginTime"`
					Source     string      `json:"Source"`
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
			} `json:"Tsunami"`
		} `json:"records"`
	}
	dec := json.NewDecoder(bytes.NewReader(pipeline.StripBOM(body)))
	dec.UseNumber()
	if err := dec.Decode(&file); err != nil {
		return nil, nil, fmt.Errorf("decode tsunami reports: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []TsunamiReport
	var warnings []string
	seen := map[[2]int]bool{}
	for i, raw := range file.Records.Tsunami {
		no, err := strconv.Atoi(raw.TsunamiNo.String())
		seq, ok := reportSeq(raw.ReportNo)
		issued := optTime(raw.IssueTime)
		if err != nil || !ok || issued == nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆海嘯資訊的編號或發布時間無法辨識", i+1))
			continue
		}
		if seen[[2]int{no, seq}] {
			continue
		}
		seen[[2]int{no, seq}] = true
		eq := raw.EarthquakeInfo
		r := TsunamiReport{
			TsunamiNo: no, Seq: seq, IssuedAt: *issued, ValidUntil: optTime(raw.ValidTime.EndTime),
			Type:       strings.TrimSpace(raw.ReportType),
			Color:      strings.TrimSpace(raw.ReportColor),
			Content:    strings.TrimSpace(raw.ReportContent),
			URL:        httpsOrEmpty(raw.Web),
			OriginTime: optTime(eq.OriginTime),
			Source:     strings.TrimSpace(eq.Source),
			Location:   strings.Join(strings.Fields(eq.Epicenter.Location), " "),
			Magnitude:  number(eq.EarthquakeMagnitude.MagnitudeValue),
			DepthKm:    number(eq.FocalDepth),
		}
		lat, lon := number(eq.Epicenter.Latitude), number(eq.Epicenter.Longitude)
		if lat != nil && lon != nil && *lat >= -90 && *lat <= 90 && *lon >= -180 && *lon <= 180 {
			r.Lat, r.Lon = lat, lon
		}
		out = append(out, r)
	}
	return out, warnings, nil
}

type TsunamiPipeline struct {
	URL    string
	APIKey string
}

func (p *TsunamiPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "tsunami",
		Name:         "海嘯資訊",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/earthquake/E-A0014-001",
		UpdateFreq:   2 * time.Minute,
		// 海嘯很少發生，資料本來就可能很久沒有新的
		StaleAfter: 365 * 24 * time.Hour,
	}
}

func (p *TsunamiPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	reports, warnings, err := ParseTsunami(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(reports)
	if len(reports) == 0 {
		res.ObserveTime(env.Now())
		return res, nil
	}
	rows := make([][]any, len(reports))
	for i, r := range reports {
		var lon, lat any
		if r.Lon != nil {
			lon, lat = *r.Lon, *r.Lat
		}
		rows[i] = []any{r.TsunamiNo, r.Seq, r.IssuedAt, r.ValidUntil, pipeline.NullIfEmpty(r.Type),
			pipeline.NullIfEmpty(r.Color), pipeline.NullIfEmpty(r.Content), pipeline.NullIfEmpty(r.URL), r.OriginTime,
			pipeline.NullIfEmpty(r.Source), pipeline.NullIfEmpty(r.Location), lon, lat, r.Magnitude, r.DepthKm}
		res.ObserveTime(r.IssuedAt)
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, row := range rows {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.tsunami (tsunami_no, report_seq, issued_at, valid_until, report_type, report_color, content,
			                          report_url, origin_time, source, location, geom, magnitude, depth_km)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			        CASE WHEN $12::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($12, $13::float8), 4326) END,
			        $14, $15)
			ON CONFLICT (tsunami_no, report_seq) DO UPDATE SET issued_at = EXCLUDED.issued_at,
				valid_until = EXCLUDED.valid_until, report_type = EXCLUDED.report_type,
				report_color = EXCLUDED.report_color, content = EXCLUDED.content, report_url = EXCLUDED.report_url,
				origin_time = EXCLUDED.origin_time, source = EXCLUDED.source, location = EXCLUDED.location,
				geom = EXCLUDED.geom, magnitude = EXCLUDED.magnitude, depth_km = EXCLUDED.depth_km
			WHERE (data.tsunami.issued_at, data.tsunami.valid_until, data.tsunami.report_color, data.tsunami.content,
			       data.tsunami.magnitude) IS DISTINCT FROM
			      (EXCLUDED.issued_at, EXCLUDED.valid_until, EXCLUDED.report_color, EXCLUDED.content, EXCLUDED.magnitude)`,
			row...)
		if err != nil {
			return res, fmt.Errorf("upsert tsunami %v-%v: %w", row[0], row[1], err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	return res, tx.Commit()
}
