package economy

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultCPIURL          = "https://ws.dgbas.gov.tw/001/Upload/461/relfile/11525/230555/pr0101a1m.xml"
	DefaultUnemploymentURL = "https://ws.dgbas.gov.tw/001/Upload/461/relfile/11525/230038/mp0101a07.xml"
	DefaultCycleURL        = "https://ws.ndc.gov.tw/Download.ashx?u=LzAwMS9hZG1pbmlzdHJhdG9yLzEwL3JlbGZpbGUvNTc4MS82MzkyL2VhMjM1YmQ5LWQwNTItNGE2OS1hYmZjLWQ1Yzc4NWQzZDBlMi56aXA%3d&n=5pmv5rCj5oyH5qiZ5Y%2bK54eI6JmfLnppcA%3d%3d&icon=.zip"
)

type Point struct {
	Series string
	Month  time.Time
	Value  float64
}

var (
	reMonth     = regexp.MustCompile(`^(\d{4})M(\d{2})$`)
	reCPIPrefix = regexp.MustCompile(`^(?:[一二三四五六七八九十]+\.|\d+\.)`)
)

func parseMonth(s string) (time.Time, bool) {
	m := reMonth.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	if mo < 1 || mo > 12 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(mo), 1, 0, 0, 0, 0, time.UTC), true
}

func cpiItem(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "(指數基期"); i >= 0 {
		s = s[:i]
	}
	if s == "總指數" {
		return s, true
	}
	p := reCPIPrefix.FindString(s)
	if p == "" {
		return "", false
	}
	return strings.TrimSpace(s[len(p):]), s[len(p):] != ""
}

func ParseCPI(body []byte, since time.Time) ([]Point, error) {
	var doc struct {
		Obs []struct {
			Item  string `xml:"Item"`
			Time  string `xml:"TIME_PERIOD"`
			Type  string `xml:"TYPE"`
			Value string `xml:"Item_VALUE"`
		} `xml:"Obs"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode CPI: %w", err)
	}
	var out []Point
	for _, o := range doc.Obs {
		item, ok := cpiItem(o.Item)
		month, okm := parseMonth(o.Time)
		v, err := strconv.ParseFloat(strings.TrimSpace(o.Value), 64)
		if !ok || !okm || err != nil || month.Before(since) {
			continue
		}
		switch strings.TrimSpace(o.Type) {
		case "原始值":
			out = append(out, Point{"cpi:" + item, month, v})
		case "年增率(%)":
			out = append(out, Point{"cpi_yoy:" + item, month, v})
		}
	}
	return out, nil
}

func unemploymentName(tag string) string {
	tag = strings.TrimSuffix(tag, "_百分比")
	if strings.HasPrefix(tag, "age_") {
		a := strings.TrimPrefix(tag, "age_")
		if a == "65_over" {
			return "65歲以上"
		}
		return a + "歲"
	}
	if i := strings.Index(tag, "_"); i > 0 {
		return tag[:i]
	}
	return tag
}

func ParseUnemployment(body []byte, since time.Time) ([]Point, error) {
	d := xml.NewDecoder(bytes.NewReader(pipeline.StripBOM(body)))
	var out []Point
	var month time.Time
	var ok bool
	var tag string
	depth := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode unemployment: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			tag = t.Name.Local
			if depth == 2 {
				ok = false
			}
		case xml.EndElement:
			depth--
			tag = ""
		case xml.CharData:
			if depth != 3 || tag == "" {
				continue
			}
			s := strings.TrimSpace(string(t))
			if tag == "年月別_Year_and_month" {
				month, ok = parseMonth(s)
				continue
			}
			v, err := strconv.ParseFloat(s, 64)
			if !ok || err != nil || month.Before(since) {
				continue
			}
			out = append(out, Point{"unemployment:" + unemploymentName(tag), month, v})
		}
	}
	return out, nil
}

var cycleColumns = map[string]string{
	"景氣對策信號綜合分數": "signal_score",
	"領先指標不含趨勢指數": "leading",
	"同時指標不含趨勢指數": "coincident",
	"落後指標不含趨勢指數": "lagging",
}

func ParseCycle(body []byte, since time.Time) ([]Point, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("open business cycle zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "景氣指標與燈號.csv" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 10<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(b)))
		r.FieldsPerRecord = -1
		rows, err := r.ReadAll()
		if err != nil || len(rows) < 2 {
			return nil, fmt.Errorf("decode business cycle CSV: %v", err)
		}
		cols := map[int]string{}
		for i, h := range rows[0] {
			if s, ok := cycleColumns[strings.TrimSpace(h)]; ok {
				cols[i] = s
			}
		}
		if len(cols) != len(cycleColumns) {
			return nil, fmt.Errorf("business cycle CSV: 欄位不是預期的格式 %v", rows[0])
		}
		var out []Point
		for _, row := range rows[1:] {
			d := strings.TrimSpace(row[0])
			if len(d) != 6 {
				continue
			}
			month, ok := parseMonth(d[:4] + "M" + d[4:])
			if !ok || month.Before(since) {
				continue
			}
			for i, s := range cols {
				if i >= len(row) {
					continue
				}
				if v, err := strconv.ParseFloat(strings.TrimSpace(row[i]), 64); err == nil {
					out = append(out, Point{s, month, v})
				}
			}
		}
		return out, nil
	}
	return nil, errors.New("business cycle zip: 沒有 景氣指標與燈號.csv")
}

type IndicatorPipeline struct {
	CPIURL, UnemploymentURL, CycleURL string
	SinceYear                         int // zero is 2010
}

func (p *IndicatorPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "econ_indicator",
		Name:         "經濟指標（消費者物價、失業率、景氣對策信號）",
		SourceAgency: "行政院主計總處、國家發展委員會",
		SourceURL:    "https://data.gov.tw/dataset/6019",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

func (p *IndicatorPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	since := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	if p.SinceYear > 0 {
		since = time.Date(p.SinceYear, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	sources := []struct {
		name, url string
		parse     func([]byte, time.Time) ([]Point, error)
	}{
		{"消費者物價", p.CPIURL, ParseCPI},
		{"失業率", p.UnemploymentURL, ParseUnemployment},
		{"景氣指標", p.CycleURL, ParseCycle},
	}
	var points []Point
	for _, s := range sources {
		body, err := env.Fetch.Get(ctx, s.url, s.url)
		if err == nil {
			var list []Point
			if list, err = s.parse(body, since); err == nil && len(list) == 0 {
				err = errors.New("沒有資料")
			}
			points = append(points, list...)
		}
		if err != nil {
			res.Warnf("%s：%v", s.name, err)
		}
	}
	res.RowsFetched = len(points)
	if len(points) == 0 {
		return res, errors.New("經濟指標都沒有取得")
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Series != points[j].Series {
			return points[i].Series < points[j].Series
		}
		return points[i].Month.Before(points[j].Month)
	})
	var rows [][]any
	var latest time.Time
	for i, pt := range points {
		if i > 0 && pt.Series == points[i-1].Series && pt.Month.Equal(points[i-1].Month) {
			continue
		}
		if pt.Month.After(latest) {
			latest = pt.Month
		}
		rows = append(rows, []any{pt.Series, pt.Month, pt.Value})
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.econ_month (series, month, value) VALUES`, rows,
		`ON CONFLICT (series, month) DO UPDATE SET value = EXCLUDED.value WHERE data.econ_month.value <> EXCLUDED.value`); err != nil {
		return res, fmt.Errorf("upsert econ_month: %w", err)
	}
	res.ObserveTime(time.Date(latest.Year(), latest.Month(), 1, 0, 0, 0, 0, pipeline.Taipei).AddDate(0, 1, -1))
	return res, tx.Commit()
}

const DefaultIncomeURL = "https://www.fia.gov.tw/WEB/fia/ias/ias%d/%d_165-9.csv"

type Town struct {
	Area         string // 臺北市松山區
	Households   int
	Income       int64
	Mean, Median int
}

func ParseIncome(body []byte) ([]Town, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("decode income CSV: %v", err)
	}
	h := rows[0]
	if len(h) < 6 || strings.TrimSpace(h[1]) != "村里" || strings.TrimSpace(h[5]) != "中位數" {
		return nil, fmt.Errorf("income CSV: 欄位不是預期的格式 %v", h)
	}
	var out []Town
	for _, row := range rows[1:] {
		if len(row) < 6 || strings.TrimSpace(row[1]) != "合計" {
			continue
		}
		n, err1 := strconv.Atoi(strings.TrimSpace(row[2]))
		income, err2 := strconv.ParseInt(strings.TrimSpace(row[3]), 10, 64)
		mean, err3 := strconv.Atoi(strings.TrimSpace(row[4]))
		median, err4 := strconv.Atoi(strings.TrimSpace(row[5]))
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || n <= 0 {
			continue
		}
		out = append(out, Town{Area: strings.TrimSpace(row[0]), Households: n, Income: income, Mean: mean, Median: median})
	}
	return out, nil
}

type IncomePipeline struct {
	URL   string // DefaultIncomeURL
	Years int    // zero is 6
}

func (p *IncomePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "income_town",
		Name:         "綜合所得稅所得總額（各鄉鎮市區）",
		SourceAgency: "財政部財政資訊中心",
		SourceURL:    "https://data.gov.tw/dataset/103066",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   4 * 365 * 24 * time.Hour, // 來源大約落後兩年半
	}
}

func (p *IncomePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	years := p.Years
	if years == 0 {
		years = 6
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	have := map[int]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT DISTINCT year FROM data.income_town`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			rows.Close()
			return res, err
		}
		have[y] = true
	}
	rows.Close()
	now := env.Now().Year()
	latest := 0
	for y := now - years; y < now; y++ {
		if have[y] {
			latest = y
			continue
		}
		roc := y - 1911
		u := fmt.Sprintf(p.URL, roc, roc)
		body, err := env.Fetch.Get(ctx, u, u)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			continue // 還沒公布
		}
		var towns []Town
		if err == nil {
			towns, err = ParseIncome(body)
		}
		if err != nil {
			res.Warnf("%d 年度：%v", roc, err)
			continue
		}
		res.RowsFetched += len(towns)
		var batch [][]any
		unknown := 0
		for _, t := range towns {
			code, ok := counties.CodeFromAddress(t.Area)
			name := []rune(t.Area)
			if !ok || len(name) <= 3 {
				unknown++
				continue
			}
			town := string(name[3:])
			if town == "其他" {
				continue
			}
			batch = append(batch, []any{y, code, town, t.Households, t.Income, t.Mean, t.Median})
		}
		if unknown > 0 {
			res.Warnf("%d 年度：%d 個鄉鎮對不到縣市", roc, unknown)
		}
		tx, err := env.DB.BeginTx(ctx, nil)
		if err != nil {
			return res, err
		}
		n, err := pipeline.InsertRows(ctx, tx, `INSERT INTO data.income_town (year, county_code, town, households, income_k,
			mean_k, median_k) VALUES`, batch, `ON CONFLICT (year, county_code, town) DO NOTHING`)
		if err == nil {
			err = tx.Commit()
		}
		tx.Rollback()
		if err != nil {
			return res, fmt.Errorf("%d 年度：%w", roc, err)
		}
		res.RowsInserted += n
		latest = y
	}
	if latest == 0 {
		return res, errors.New("最近幾年的所得資料都沒有取得")
	}
	res.ObserveTime(time.Date(latest, 12, 31, 0, 0, 0, 0, pipeline.Taipei))
	return res, nil
}
