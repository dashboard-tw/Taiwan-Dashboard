package power

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultSalesURL = "https://service.taipower.com.tw/data/opendata/apply/file/d007012/001.csv"

var salesSectors = map[string]string{"住宅": "residential", "服務業": "service", "農林漁牧": "agriculture", "工業": "industry"}

type Sale struct {
	Month  time.Time
	County string // as written, e.g. 台北市
	Sector string
	KWh    int64
}

func ParseSales(body []byte) ([]Sale, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read sales header: %w", err)
	}
	if len(head) < 4 || strings.TrimSpace(head[0]) != "年月" || strings.TrimSpace(head[1]) != "縣市" {
		return nil, fmt.Errorf("售電量的欄位不符：%v", head)
	}
	var out []Sale
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read sales: %w", err)
		}
		if len(rec) < 4 {
			continue
		}
		ym, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil || ym < 10001 {
			continue
		}
		sector, ok := salesSectors[strings.TrimSpace(rec[2])]
		if !ok {
			continue
		}
		kwh, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(rec[3]), ",", ""), 10, 64)
		if err != nil || kwh < 0 {
			continue
		}
		month := time.Date(ym/100+1911, time.Month(ym%100), 1, 0, 0, 0, 0, pipeline.Taipei)
		out = append(out, Sale{Month: month, County: strings.TrimSpace(rec[1]), Sector: sector, KWh: kwh})
	}
	return out, nil
}

type SalesPipeline struct {
	URL string
}

func (p *SalesPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_sales",
		Name:         "各縣市各部門售電量",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/29935",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   100 * 24 * time.Hour,
	}
}

func (p *SalesPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseSales(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("售電量是空的")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	unknown := map[string]bool{}
	for _, s := range list {
		code, ok := counties.Code(s.County)
		if !ok {
			unknown[s.County] = true
			continue
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_sales_month (month, county_code, sector, kwh) VALUES ($1, $2, $3, $4)
			ON CONFLICT (month, county_code, sector) DO UPDATE SET kwh = EXCLUDED.kwh, ingested_at = now()
			WHERE data.power_sales_month.kwh IS DISTINCT FROM EXCLUDED.kwh`,
			s.Month, code, s.Sector, s.KWh)
		if err != nil {
			return res, fmt.Errorf("upsert sales %s %s: %w", s.Month.Format("2006-01"), s.County, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(s.Month)
	}
	for c := range unknown {
		res.Warnf("對不到的縣市：%s", c)
	}
	return res, tx.Commit()
}
