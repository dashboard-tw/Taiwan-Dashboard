package disease

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultDengueURL = "https://od.cdc.gov.tw/eic/Dengue_Daily_last12m.csv"
	DefaultILIURL    = "https://od.cdc.gov.tw/eic/NHI_Influenza_like_illness.csv"
	DefaultEVURL     = "https://od.cdc.gov.tw/eic/NHI_EnteroviralInfection.csv"
	DefaultCovidURL  = "https://od.cdc.gov.tw/eic/NHI_COVID-19.csv"
)

func readCSV(body []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return nil, errors.New("CSV 是空的")
	}
	return rows, nil
}

type DengueKey struct {
	Date         time.Time
	County, Town string
	Imported     bool
}

func ParseDengue(body []byte) (map[DengueKey]int, error) {
	rows, err := readCSV(body)
	if err != nil {
		return nil, fmt.Errorf("decode dengue: %w", err)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(h)] = i
	}
	for _, h := range []string{"發病日", "居住縣市", "居住鄉鎮", "是否境外移入", "確定病例數"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("dengue: 沒有欄位 %s", h)
		}
	}
	get := func(r []string, h string) string {
		if i := col[h]; i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	out := map[DengueKey]int{}
	for _, r := range rows[1:] {
		d, err := time.ParseInLocation("2006/01/02", get(r, "發病日"), time.UTC)
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(get(r, "確定病例數"))
		if err != nil || n <= 0 {
			continue
		}
		town := get(r, "居住鄉鎮")
		if town == "None" {
			town = ""
		}
		k := DengueKey{Date: d, County: get(r, "居住縣市"), Town: town, Imported: get(r, "是否境外移入") == "是"}
		out[k] += n
	}
	return out, nil
}

type DenguePipeline struct {
	URL string
}

func (p *DenguePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "dengue_daily",
		Name:         "登革熱每日確定病例",
		SourceAgency: "衛生福利部疾病管制署",
		SourceURL:    "https://data.cdc.gov.tw/dataset/dengue-daily-last12m",
		UpdateFreq:   6 * time.Hour,
		StaleAfter:   3 * 24 * time.Hour,
	}
}

func (p *DenguePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	cases, err := ParseDengue(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(cases)
	if len(cases) == 0 {
		return res, errors.New("登革熱病例是空的")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	var rows [][]any
	first, unknown := time.Time{}, 0
	for k, n := range cases {
		code, ok := counties.Code(k.County)
		if !ok {
			unknown += n
			continue
		}
		if first.IsZero() || k.Date.Before(first) {
			first = k.Date
		}
		rows = append(rows, []any{k.Date, code, k.Town, k.Imported, n})
	}
	if unknown > 0 {
		res.Warnf("%d 個病例對不到居住縣市", unknown)
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.dengue_daily WHERE onset_date >= $1`, first); err != nil {
		return res, err
	}
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx,
		`INSERT INTO data.dengue_daily (onset_date, county_code, town, imported, cases) VALUES`, rows, ""); err != nil {
		return res, fmt.Errorf("insert dengue: %w", err)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type Visit struct {
	Year, Week    int
	County        string // name as written in the file
	CountyCode    string // code as written in the file, may be empty
	Type          string // outpatient, emergency, inpatient
	Visits, Total int
}

var visitTypes = map[string]string{"門診": "outpatient", "急診": "emergency", "住院": "inpatient"}

func ParseVisits(body []byte, since int) ([]Visit, error) {
	rows, err := readCSV(body)
	if err != nil {
		return nil, fmt.Errorf("decode NHI visits: %w", err)
	}
	if h := rows[0]; len(h) < 7 || strings.TrimSpace(h[0]) != "年" || strings.TrimSpace(h[1]) != "週" {
		return nil, fmt.Errorf("NHI visits: 欄位不是預期的格式 %v", h)
	}
	type key struct {
		year, week  int
		county, typ string
	}
	sum := map[key]*Visit{}
	for _, r := range rows[1:] {
		if len(r) < 7 {
			continue
		}
		year, err1 := strconv.Atoi(strings.TrimSpace(r[0]))
		week, err2 := strconv.Atoi(strings.TrimSpace(r[1]))
		typ, ok := visitTypes[strings.TrimSpace(r[2])]
		visits, err3 := strconv.Atoi(strings.TrimSpace(r[5]))
		total, err4 := strconv.Atoi(strings.TrimSpace(r[6]))
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || !ok || year < since || week < 1 || week > 53 {
			continue
		}
		county := strings.TrimSpace(r[4])
		k := key{year, week, county, typ}
		v := sum[k]
		if v == nil {
			v = &Visit{Year: year, Week: week, County: county, Type: typ}
			if len(r) > 7 {
				v.CountyCode = strings.TrimSpace(r[7])
			}
			sum[k] = v
		}
		v.Visits += visits
		v.Total += total
	}
	out := make([]Visit, 0, len(sum))
	for _, v := range sum {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Year != b.Year {
			return a.Year < b.Year
		}
		if a.Week != b.Week {
			return a.Week < b.Week
		}
		return a.County+a.Type < b.County+b.Type
	})
	return out, nil
}

func WeekStart(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, (week-1)*7)
}

type VisitPipeline struct {
	URLs      map[string]string // disease (ili, ev, covid) → URL
	SinceYear int
}

var DefaultVisitURLs = map[string]string{"ili": DefaultILIURL, "ev": DefaultEVURL, "covid": DefaultCovidURL}

func (p *VisitPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "nhi_visit",
		Name:         "健保門急診與住院就診人次（類流感、腸病毒、新冠）",
		SourceAgency: "衛生福利部疾病管制署",
		SourceURL:    "https://data.cdc.gov.tw/dataset/hi-outpatient-emergency-visit-influenza-like-illness",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   15 * 24 * time.Hour,
	}
}

func (p *VisitPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	known := map[string]bool{}
	for _, d := range []string{"ili", "ev", "covid"} {
		known[d] = true
	}
	since := p.SinceYear
	if since == 0 {
		since = env.Now().Year() - 6
	}
	diseases := make([]string, 0, len(p.URLs))
	for d := range p.URLs {
		diseases = append(diseases, d)
	}
	sort.Strings(diseases)
	var latest time.Time
	stored := 0
	for _, d := range diseases {
		if !known[d] {
			return res, fmt.Errorf("unknown disease %q", d)
		}
		u := p.URLs[d]
		body, err := env.Fetch.Get(ctx, u, u)
		if err != nil {
			res.Warnf("%s：%v", d, err)
			continue
		}
		visits, err := ParseVisits(body, since)
		if err != nil {
			res.Warnf("%s：%v", d, err)
			continue
		}
		res.RowsFetched += len(visits)
		var rows [][]any
		unknown := 0
		for _, v := range visits {
			code, ok := counties.Code(v.County)
			if !ok {
				unknown++
				continue
			}
			start := WeekStart(v.Year, v.Week)
			if start.After(latest) {
				latest = start
			}
			rows = append(rows, []any{d, v.Year, v.Week, start, code, v.Type, v.Visits, v.Total})
		}
		if unknown > 0 {
			res.Warnf("%s：%d 列對不到縣市", d, unknown)
		}
		tx, err := env.DB.BeginTx(ctx, nil)
		if err != nil {
			return res, err
		}
		n, err := pipeline.InsertRows(ctx, tx, `INSERT INTO data.nhi_visit_week (disease, year, week, week_start, county_code,
			visit_type, visits, total) VALUES`, rows, `ON CONFLICT (disease, year, week, county_code, visit_type) DO UPDATE
			SET week_start = EXCLUDED.week_start, visits = EXCLUDED.visits, total = EXCLUDED.total`)
		if err == nil {
			err = tx.Commit()
		}
		tx.Rollback()
		if err != nil {
			return res, fmt.Errorf("%s: %w", d, err)
		}
		res.RowsInserted += n
		stored++
	}
	if stored == 0 {
		return res, errors.New("三種疾病的就診人次都沒有取得")
	}
	if !latest.IsZero() {
		res.ObserveTime(latest.AddDate(0, 0, 6).In(pipeline.Taipei))
	}
	return res, nil
}
