package agri

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://data.moa.gov.tw/Service/OpenData/FromM/FarmTransData.aspx"

var categories = map[string]string{"N04": "vegetable", "N05": "fruit", "N06": "flower"}

type Price struct {
	Date           time.Time
	Category, Crop string
	AvgPrice       float64
	Volume         float64
	Markets        int
}

func rocDate(t time.Time) string {
	return fmt.Sprintf("%d.%02d.%02d", t.Year()-1911, t.Month(), t.Day())
}

func Parse(body []byte) ([]Price, error) {
	var rows []struct {
		Date     string   `json:"交易日期"`
		Kind     *string  `json:"種類代碼"`
		CropCode string   `json:"作物代號"`
		Crop     *string  `json:"作物名稱"`
		Market   string   `json:"市場代號"`
		Avg      *float64 `json:"平均價"`
		Volume   *float64 `json:"交易量"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode farm prices: %w", err)
	}
	type key struct{ date, cat, crop string }
	type acc struct {
		value, volume float64
		markets       map[string]bool
		date          time.Time
	}
	sums := map[key]*acc{}
	var order []key
	for _, r := range rows {
		if r.Kind == nil || r.Crop == nil || r.Avg == nil || r.Volume == nil || r.CropCode == "rest" {
			continue
		}
		cat, ok := categories[*r.Kind]
		if !ok || *r.Volume <= 0 || *r.Avg <= 0 {
			continue
		}
		crop := strings.TrimSpace(strings.SplitN(*r.Crop, "-", 2)[0])
		parts := strings.Split(strings.TrimSpace(r.Date), ".")
		if crop == "" || len(parts) != 3 {
			continue
		}
		var y, m, d int
		if _, err := fmt.Sscanf(r.Date, "%d.%d.%d", &y, &m, &d); err != nil || m < 1 || m > 12 || d < 1 || d > 31 {
			continue
		}
		k := key{r.Date, cat, crop}
		a := sums[k]
		if a == nil {
			a = &acc{markets: map[string]bool{}, date: time.Date(y+1911, time.Month(m), d, 0, 0, 0, 0, pipeline.Taipei)}
			sums[k] = a
			order = append(order, k)
		}
		a.value += *r.Avg * *r.Volume
		a.volume += *r.Volume
		a.markets[r.Market] = true
	}
	out := make([]Price, 0, len(order))
	for _, k := range order {
		a := sums[k]
		out = append(out, Price{Date: a.date, Category: k.cat, Crop: k.crop, AvgPrice: a.value / a.volume,
			Volume: a.volume, Markets: len(a.markets)})
	}
	return out, nil
}

type Pipeline struct {
	URL string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "farm_price",
		Name:         "果菜花卉批發市場交易行情",
		SourceAgency: "農業部",
		SourceURL:    "https://data.gov.tw/dataset/8066",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   7 * 24 * time.Hour,
	}
}

const (
	keepDays     = 90
	recentDays   = 3
	backfillDays = 15
)

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	have := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT DISTINCT to_char(trade_date, 'YYYY-MM-DD') FROM data.farm_price_daily
		WHERE trade_date >= current_date - $1::int`, keepDays)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return res, err
		}
		have[d] = true
	}
	rows.Close()

	today := pipeline.TaipeiDate(env.Now())
	var days []time.Time
	backfill := 0
	for i := 0; i < keepDays; i++ {
		d := today.AddDate(0, 0, -i)
		if i < recentDays {
			days = append(days, d)
		} else if !have[d.Format("2006-01-02")] && backfill < backfillDays {
			days = append(days, d)
			backfill++
		}
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, d := range days {
		q := url.Values{"StartDate": {rocDate(d)}, "EndDate": {rocDate(d)}}
		u := p.URL + "?" + q.Encode()
		body, err := env.Fetch.Get(ctx, u, u)
		if err != nil {
			return res, err
		}
		list, err := Parse(body)
		if err != nil {
			return res, fmt.Errorf("%s: %w", d.Format("2006-01-02"), err)
		}
		res.RowsFetched += len(list)
		for _, x := range list {
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.farm_price_daily (trade_date, category, crop, avg_price, volume, markets)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (trade_date, category, crop) DO UPDATE SET
					avg_price = EXCLUDED.avg_price, volume = EXCLUDED.volume, markets = EXCLUDED.markets, ingested_at = now()
				WHERE (data.farm_price_daily.avg_price, data.farm_price_daily.volume)
				      IS DISTINCT FROM (EXCLUDED.avg_price, EXCLUDED.volume)`,
				x.Date, x.Category, x.Crop, round2(x.AvgPrice), x.Volume, x.Markets)
			if err != nil {
				return res, fmt.Errorf("upsert farm price %s %s: %w", x.Date.Format("2006-01-02"), x.Crop, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
			res.ObserveTime(x.Date)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.farm_price_daily WHERE trade_date < current_date - $1::int`, keepDays*4); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
