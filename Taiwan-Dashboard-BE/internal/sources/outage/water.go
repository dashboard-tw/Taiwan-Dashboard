package outage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultTWCURL    = "https://web.water.gov.tw/wateroffapi/openData/export/json"
	DefaultTaipeiURL = "https://webs.water.gov.taipei/apps/outage.asmx/outageOpenData"
)

type WaterOutage struct {
	CaseID             string
	County             string // county name as written by the source
	District           string
	Area               string
	Reason             string
	Kind               string // 計畫性、非計畫性
	Households         *int
	StartsAt, EndsAt   time.Time
	DailyFrom, DailyTo string // HH:MM, Taipei Water only
	LowPressureArea    string
	LowPressureReason  string
	URL                string
	PublishedOn        *time.Time
}

func emptyText(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "null", "[]", "[null]":
		return ""
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func decodeObjects(body []byte) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(pipeline.StripBOM(body)))
	dec.UseNumber()
	var out []map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode outage list: %w", err)
	}
	return out, nil
}

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func ParseTWC(body []byte) ([]WaterOutage, []string, error) {
	recs, err := decodeObjects(body)
	if err != nil {
		return nil, nil, err
	}
	var out []WaterOutage
	var warnings []string
	for i, r := range recs {
		o := WaterOutage{
			CaseID:            str(r, "案件編號"),
			County:            str(r, "影響縣市"),
			District:          str(r, "影響行政區"),
			Area:              emptyText(str(r, "停水地區")),
			Reason:            emptyText(str(r, "停水原因")),
			Kind:              str(r, "案件類型"),
			LowPressureArea:   emptyText(str(r, "降壓地區")),
			LowPressureReason: emptyText(str(r, "降壓原因")),
		}
		if o.CaseID == "" {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆沒有案件編號", i+1))
			continue
		}
		layouts := []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04:05", "2006/01/02 15:04"}
		if o.StartsAt, err = pipeline.ParseTaipeiTime(str(r, "案件日期時間"), layouts...); err != nil {
			warnings = append(warnings, fmt.Sprintf("案件 %s 的開始時間：%v", o.CaseID, err))
			continue
		}
		if o.EndsAt, err = pipeline.ParseTaipeiTime(str(r, "恢復日期時間"), layouts...); err != nil {
			warnings = append(warnings, fmt.Sprintf("案件 %s 的恢復時間：%v", o.CaseID, err))
			continue
		}
		if o.EndsAt.Before(o.StartsAt) {
			warnings = append(warnings, fmt.Sprintf("案件 %s 的恢復時間早於開始時間", o.CaseID))
			continue
		}
		if v, err := pipeline.ParseNumber(str(r, "影響戶數")); err != nil {
			warnings = append(warnings, fmt.Sprintf("案件 %s 的影響戶數：%v", o.CaseID, err))
		} else if v != nil && *v >= 0 {
			n := int(*v)
			o.Households = &n
		}
		if o.Area == "" {
			o.Area = o.District
		}
		out = append(out, o)
	}
	return out, warnings, nil
}

var (
	rocDay      = regexp.MustCompile(`(\d{2,3})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日`)
	dailyWindow = regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*[~～-]\s*(\d{1,2}):(\d{2})$`)
	rocPubDate  = regexp.MustCompile(`^(\d{2,3})-(\d{1,2})-(\d{1,2})$`)
)

func rocYMD(y, m, d string) (time.Time, bool) {
	yy, _ := strconv.Atoi(y)
	mm, _ := strconv.Atoi(m)
	dd, _ := strconv.Atoi(d)
	t := time.Date(yy+1911, time.Month(mm), dd, 0, 0, 0, 0, pipeline.Taipei)
	return t, t.Month() == time.Month(mm) && t.Day() == dd
}

func ParseTaipei(body []byte) ([]WaterOutage, []string, error) {
	recs, err := decodeObjects(body)
	if err != nil {
		return nil, nil, err
	}
	var out []WaterOutage
	var warnings []string
	for i, r := range recs {
		o := WaterOutage{
			CaseID: str(r, "SW_No"),
			Area:   str(r, "SW_Area"),
			Reason: str(r, "Description"),
			URL:    str(r, "MapUrl"),
		}
		if o.CaseID == "" {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆沒有公告編號", i+1))
			continue
		}
		if o.Area == "" {
			o.Area = str(r, "Title")
		}
		if o.Reason == o.Area {
			o.Reason = ""
		}
		if !strings.HasPrefix(o.URL, "https://") {
			o.URL = ""
		}
		days := rocDay.FindAllStringSubmatch(str(r, "Date"), -1)
		if len(days) == 0 {
			warnings = append(warnings, fmt.Sprintf("公告 %s 的日期「%s」無法解析", o.CaseID, str(r, "Date")))
			continue
		}
		first, ok1 := rocYMD(days[0][1], days[0][2], days[0][3])
		last := first
		ok2 := true
		if len(days) > 1 {
			last, ok2 = rocYMD(days[len(days)-1][1], days[len(days)-1][2], days[len(days)-1][3])
		}
		if !ok1 || !ok2 || last.Before(first) {
			warnings = append(warnings, fmt.Sprintf("公告 %s 的日期「%s」不合理", o.CaseID, str(r, "Date")))
			continue
		}
		o.StartsAt, o.EndsAt = first, last.AddDate(0, 0, 1)
		if m := dailyWindow.FindStringSubmatch(str(r, "Time")); m != nil {
			fh, _ := strconv.Atoi(m[1])
			fm, _ := strconv.Atoi(m[2])
			th, _ := strconv.Atoi(m[3])
			tm, _ := strconv.Atoi(m[4])
			if fh < 24 && th <= 24 && fm < 60 && tm < 60 && fh*60+fm < th*60+tm {
				o.DailyFrom, o.DailyTo = fmt.Sprintf("%02d:%s", fh, m[2]), fmt.Sprintf("%02d:%s", th, m[4])
				o.StartsAt = first.Add(time.Duration(fh)*time.Hour + time.Duration(fm)*time.Minute)
				o.EndsAt = last.Add(time.Duration(th)*time.Hour + time.Duration(tm)*time.Minute)
			}
		} else if str(r, "Time") != "" {
			warnings = append(warnings, fmt.Sprintf("公告 %s 的時段「%s」無法解析，以整天計算", o.CaseID, str(r, "Time")))
		}
		if m := rocPubDate.FindStringSubmatch(str(r, "PubDate")); m != nil {
			if d, ok := rocYMD(m[1], m[2], m[3]); ok {
				o.PublishedOn = &d
			}
		}
		out = append(out, o)
	}
	return out, warnings, nil
}

type WaterPipeline struct {
	Source string // twc or taipei
	URL    string
}

func (p *WaterPipeline) Dataset() pipeline.Dataset {
	if p.Source == "taipei" {
		return pipeline.Dataset{
			ID:           "water_outage_taipei",
			Name:         "臺北自來水停水公告",
			SourceAgency: "臺北自來水事業處",
			SourceURL:    "https://data.gov.tw/dataset/128236",
			UpdateFreq:   30 * time.Minute,
			StaleAfter:   3 * time.Hour,
		}
	}
	return pipeline.Dataset{
		ID:           "water_outage",
		Name:         "台灣自來水停水資訊",
		SourceAgency: "台灣自來水公司",
		SourceURL:    "https://data.gov.tw/dataset/6050",
		UpdateFreq:   30 * time.Minute,
		StaleAfter:   3 * time.Hour,
	}
}

func (p *WaterPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	var outages []WaterOutage
	var warnings []string
	if p.Source == "taipei" {
		outages, warnings, err = ParseTaipei(body)
	} else {
		outages, warnings, err = ParseTWC(body)
	}
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(outages)

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
	var noCounty []string
	for _, o := range outages {
		county := ""
		if c, ok := counties.Code(o.County); ok {
			county = c
		} else if c, ok := counties.CodeFromAddress(o.Area); ok {
			county = c
		} else {
			noCounty = append(noCounty, o.CaseID)
		}
		var from, to any
		if o.DailyFrom != "" {
			from, to = o.DailyFrom, o.DailyTo
		}
		var inserted bool
		err := tx.QueryRowContext(ctx, `
			INSERT INTO data.water_outage
				(source, case_id, county_code, district, area, reason, kind, households, starts_at, ends_at,
				 daily_from, daily_to, low_pressure_area, low_pressure_reason, url, published_on,
				 first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::time, $12::time, $13, $14, $15, $16, $17, $17)
			ON CONFLICT (source, case_id) DO UPDATE SET
				county_code = EXCLUDED.county_code, district = EXCLUDED.district, area = EXCLUDED.area,
				reason = EXCLUDED.reason, kind = EXCLUDED.kind, households = EXCLUDED.households,
				starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at,
				daily_from = EXCLUDED.daily_from, daily_to = EXCLUDED.daily_to,
				low_pressure_area = EXCLUDED.low_pressure_area, low_pressure_reason = EXCLUDED.low_pressure_reason,
				url = EXCLUDED.url, published_on = EXCLUDED.published_on,
				last_seen_at = EXCLUDED.last_seen_at,
				updated_at = CASE WHEN (data.water_outage.county_code, data.water_outage.district, data.water_outage.area,
				                        data.water_outage.reason, data.water_outage.kind, data.water_outage.households,
				                        data.water_outage.starts_at, data.water_outage.ends_at,
				                        data.water_outage.daily_from, data.water_outage.daily_to,
				                        data.water_outage.low_pressure_area, data.water_outage.low_pressure_reason)
				                       IS DISTINCT FROM
				                       (EXCLUDED.county_code, EXCLUDED.district, EXCLUDED.area, EXCLUDED.reason,
				                        EXCLUDED.kind, EXCLUDED.households, EXCLUDED.starts_at, EXCLUDED.ends_at,
				                        EXCLUDED.daily_from, EXCLUDED.daily_to,
				                        EXCLUDED.low_pressure_area, EXCLUDED.low_pressure_reason)
				                  THEN now() ELSE data.water_outage.updated_at END
			RETURNING (xmax = 0)`,
			p.Source, o.CaseID, pipeline.NullIfEmpty(county), pipeline.NullIfEmpty(o.District), o.Area,
			pipeline.NullIfEmpty(o.Reason), pipeline.NullIfEmpty(o.Kind), o.Households, o.StartsAt, o.EndsAt,
			from, to, pipeline.NullIfEmpty(o.LowPressureArea), pipeline.NullIfEmpty(o.LowPressureReason),
			pipeline.NullIfEmpty(o.URL), o.PublishedOn, now).Scan(&inserted)
		if err != nil {
			return res, fmt.Errorf("upsert outage %s: %w", o.CaseID, err)
		}
		if inserted {
			res.RowsInserted++
		}
	}
	if len(noCounty) > 0 {
		res.Warnf("%d 筆停水公告無法判斷縣市：%s", len(noCounty), strings.Join(noCounty, "、"))
	}
	res.ObserveTime(now)
	return res, tx.Commit()
}
