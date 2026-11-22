package agri

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultPoultryURL = "https://data.moa.gov.tw/Service/OpenData/FromM/PoultryTransBoiledChickenData.aspx?IsTransData=1&UnitId=056"
	DefaultHogURL     = "https://data.moa.gov.tw/Service/OpenData/FromM/AnimalTransData.aspx?IsTransData=1&UnitId=026"
)

type Poultry struct {
	Date                                      time.Time
	EggFarm, EggTransport                     *float64
	BroilerLarge, BroilerSmall, BroilerRetail *float64
}

func price(raw json.RawMessage) *float64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || v > 1000 {
		return nil
	}
	return &v
}

func ParsePoultry(body []byte) ([]Poultry, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode poultry prices: %w", err)
	}
	var out []Poultry
	for _, r := range rows {
		var day string
		if err := json.Unmarshal(r["日期"], &day); err != nil {
			continue
		}
		d, err := time.Parse("2006/01/02", strings.TrimSpace(day))
		if err != nil {
			continue
		}
		p := Poultry{Date: d, EggFarm: price(r["雞蛋(產地價)"]), EggTransport: price(r["雞蛋(大運輸價)"]),
			BroilerLarge: price(r["白肉雞(2.0Kg以上)"]), BroilerSmall: price(r["白肉雞(1.75-1.95Kg)"]),
			BroilerRetail: price(r["白肉雞(門市價高屏)"])}
		if p.EggFarm == nil && p.EggTransport == nil && p.BroilerLarge == nil && p.BroilerSmall == nil && p.BroilerRetail == nil {
			continue
		}
		out = append(out, p)
	}
	dropSpikes(out)
	return out, nil
}

func dropSpikes(days []Poultry) {
	sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })
	for _, field := range []func(*Poultry) **float64{
		func(p *Poultry) **float64 { return &p.EggFarm },
		func(p *Poultry) **float64 { return &p.EggTransport },
	} {
		var last *float64
		for i := range days {
			v := field(&days[i])
			if *v == nil {
				continue
			}
			if last != nil && math.Abs(**v-*last) > 0.4**last {
				*v = nil
				continue
			}
			last = *v
		}
	}
}

type PoultryPipeline struct {
	URL string
}

func (p *PoultryPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "poultry_price",
		Name:         "家禽交易行情（雞蛋、白肉雞）",
		SourceAgency: "農業部畜牧司",
		SourceURL:    "https://data.gov.tw/dataset/7536",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   7 * 24 * time.Hour,
	}
}

func (p *PoultryPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParsePoultry(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("家禽行情是空的")
	}
	seen := map[time.Time]bool{}
	var rows [][]any
	var latest time.Time
	for _, x := range list {
		if seen[x.Date] {
			continue
		}
		seen[x.Date] = true
		if x.Date.After(latest) {
			latest = x.Date
		}
		rows = append(rows, []any{x.Date, x.EggFarm, x.EggTransport, x.BroilerLarge, x.BroilerSmall, x.BroilerRetail})
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.poultry_price (day, egg_farm, egg_transport,
		broiler_large, broiler_small, broiler_retail) VALUES`, rows, `ON CONFLICT (day) DO UPDATE SET
		egg_farm = EXCLUDED.egg_farm, egg_transport = EXCLUDED.egg_transport, broiler_large = EXCLUDED.broiler_large,
		broiler_small = EXCLUDED.broiler_small, broiler_retail = EXCLUDED.broiler_retail
		WHERE (data.poultry_price.egg_farm, data.poultry_price.egg_transport, data.poultry_price.broiler_large,
		       data.poultry_price.broiler_small, data.poultry_price.broiler_retail)
		      IS DISTINCT FROM (EXCLUDED.egg_farm, EXCLUDED.egg_transport, EXCLUDED.broiler_large,
		                        EXCLUDED.broiler_small, EXCLUDED.broiler_retail)`); err != nil {
		return res, fmt.Errorf("upsert poultry prices: %w", err)
	}
	res.ObserveTime(time.Date(latest.Year(), latest.Month(), latest.Day(), 0, 0, 0, 0, pipeline.Taipei))
	return res, tx.Commit()
}

type Hog struct {
	Date          time.Time
	Market        string
	Head          int
	Price, Weight *float64 // 規格豬
}

func ParseHogs(body []byte) ([]Hog, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode hog trade: %w", err)
	}
	str := func(r map[string]json.RawMessage, k string) string {
		return strings.Trim(strings.TrimSpace(string(r[k])), `"`)
	}
	var out []Hog
	for _, r := range rows {
		ds := str(r, "交易日期")
		if len(ds) != 7 {
			continue
		}
		y, err1 := strconv.Atoi(ds[:3])
		m, err2 := strconv.Atoi(ds[3:5])
		d, err3 := strconv.Atoi(ds[5:])
		head, err4 := strconv.Atoi(str(r, "成交頭數-總數"))
		market := str(r, "市場名稱")
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || head <= 0 || market == "" || m < 1 || m > 12 || d < 1 || d > 31 {
			continue
		}
		h := Hog{Date: time.Date(y+1911, time.Month(m), d, 0, 0, 0, 0, time.UTC), Market: market, Head: head}
		if v, err := strconv.ParseFloat(str(r, "規格豬-平均價格"), 64); err == nil && v > 0 && v < 1000 {
			h.Price = &v
		}
		if v, err := strconv.ParseFloat(str(r, "規格豬-平均重量"), 64); err == nil && v > 0 && v < 1000 {
			h.Weight = &v
		}
		out = append(out, h)
	}
	return out, nil
}

type HogPipeline struct {
	URL string
}

func (p *HogPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "hog_trade",
		Name:         "毛豬交易行情",
		SourceAgency: "農業部畜牧司",
		SourceURL:    "https://data.gov.tw/dataset/7296",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   7 * 24 * time.Hour,
	}
}

func (p *HogPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseHogs(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("毛豬行情是空的")
	}
	type key struct {
		day    time.Time
		market string
	}
	seen := map[key]bool{}
	var rows [][]any
	var latest time.Time
	for _, h := range list {
		k := key{h.Date, h.Market}
		if seen[k] {
			continue
		}
		seen[k] = true
		if h.Date.After(latest) {
			latest = h.Date
		}
		rows = append(rows, []any{h.Date, h.Market, h.Head, h.Price, h.Weight})
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.hog_trade (day, market, head, price, weight_kg)
		VALUES`, rows, `ON CONFLICT (day, market) DO UPDATE SET head = EXCLUDED.head, price = EXCLUDED.price,
		weight_kg = EXCLUDED.weight_kg
		WHERE (data.hog_trade.head, data.hog_trade.price, data.hog_trade.weight_kg)
		      IS DISTINCT FROM (EXCLUDED.head, EXCLUDED.price, EXCLUDED.weight_kg)`); err != nil {
		return res, fmt.Errorf("upsert hog trade: %w", err)
	}
	res.ObserveTime(time.Date(latest.Year(), latest.Month(), latest.Day(), 0, 0, 0, 0, pipeline.Taipei))
	return res, tx.Commit()
}
