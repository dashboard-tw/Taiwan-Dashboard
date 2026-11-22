package fuel

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultHistoryURL = "https://vipmbr.cpc.com.tw/cpcstn/listpricewebservice.asmx/getCPCMainProdListPrice_Historical?prodid=%d"

var Products = []struct {
	ID   int
	Name string
}{
	{1, "92無鉛汽油"},
	{2, "95無鉛汽油"},
	{3, "98無鉛汽油"},
	{4, "超級柴油"},
}

type Price struct {
	EffectiveAt time.Time
	Price       float64
	Unit        string
}

func ParseHistory(body []byte) ([]Price, error) {
	var doc struct {
		Rows []struct {
			Fields []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"diffgram>NewDataSet>tbTable"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode CPC prices: %w", err)
	}
	var out []Price
	for _, r := range doc.Rows {
		f := map[string]string{}
		for _, x := range r.Fields {
			f[x.XMLName.Local] = strings.TrimSpace(x.Value)
		}
		t, err := time.Parse(time.RFC3339, f["牌價生效時間"])
		if err != nil {
			continue
		}
		v, err := pipeline.ParseNumber(f["參考牌價"])
		if err != nil || v == nil || *v <= 0 || *v > 1000 {
			continue
		}
		out = append(out, Price{EffectiveAt: t, Price: *v, Unit: strings.ReplaceAll(f["計價單位"], " ", "")})
	}
	return out, nil
}

type Pipeline struct {
	URL string // fmt pattern taking the product id
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "fuel_price",
		Name:         "中油汽柴油參考牌價（每週）",
		SourceAgency: "台灣中油",
		SourceURL:    "https://vipmbr.cpc.com.tw/cpcstn/listpricewebservice.asmx",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   10 * 24 * time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, prod := range Products {
		url := fmt.Sprintf(p.URL, prod.ID)
		body, err := env.Fetch.Get(ctx, url, url)
		if err != nil {
			return res, err
		}
		prices, err := ParseHistory(body)
		if err != nil {
			return res, err
		}
		if len(prices) == 0 {
			return res, fmt.Errorf("%s 沒有任何牌價", prod.Name)
		}
		res.RowsFetched += len(prices)
		for _, x := range prices {
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.fuel_price (product, effective_at, price, unit) VALUES ($1, $2, $3, $4)
				ON CONFLICT (product, effective_at) DO UPDATE SET price = EXCLUDED.price, unit = EXCLUDED.unit
				WHERE (data.fuel_price.price, data.fuel_price.unit) IS DISTINCT FROM (EXCLUDED.price, EXCLUDED.unit)`,
				prod.Name, x.EffectiveAt, x.Price, pipeline.NullIfEmpty(x.Unit))
			if err != nil {
				return res, fmt.Errorf("insert %s %v: %w", prod.Name, x.EffectiveAt, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
			t := x.EffectiveAt
			if t.After(env.Now()) {
				t = env.Now()
			}
			res.ObserveTime(t)
		}
	}
	if res.RowsFetched == 0 {
		return res, errors.New("沒有任何牌價")
	}
	return res, tx.Commit()
}
