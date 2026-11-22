package food

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://data.fda.gov.tw/opendata/exportDataList.do?method=ExportData&InfoId=52&logType=2"

type Rejection struct {
	ID                                        string
	Published, Received                       *time.Time
	Origin, Product, Reason, HSCode, Disposal string
}

func parseDate(s string) *time.Time {
	t, err := time.ParseInLocation("2006/01/02", strings.TrimSpace(s), pipeline.Taipei)
	if err != nil {
		return nil
	}
	return &t
}

func Parse(body []byte) ([]Rejection, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read rejection header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"產地", "主旨", "原因", "發布日期"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("邊境查驗不合格少了欄位 %s", need)
		}
	}
	seen := map[string]bool{}
	var out []Rejection
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read rejections: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		x := Rejection{Published: parseDate(get("發布日期")), Received: parseDate(get("報驗受理日期")),
			Origin: get("產地"), Product: pipeline.Truncate(get("主旨"), 200), Reason: pipeline.Truncate(get("原因"), 200),
			HSCode: get("貨品分類號列"), Disposal: pipeline.Truncate(get("處置情形"), 200)}
		if x.Published == nil || x.Origin == "" || x.Product == "" || x.Reason == "" {
			continue
		}
		sum := sha1.Sum([]byte(x.Published.Format("2006-01-02") + "|" + x.Origin + "|" + x.Product + "|" + x.Reason + "|" +
			get("進口商名稱") + "|" + get("重量")))
		x.ID = hex.EncodeToString(sum[:10])
		if seen[x.ID] {
			continue
		}
		seen[x.ID] = true
		out = append(out, x)
	}
	return out, nil
}

type Pipeline struct {
	URL string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "food_rejection",
		Name:         "邊境查驗不合格食品",
		SourceAgency: "衛生福利部食品藥物管理署",
		SourceURL:    "https://data.fda.gov.tw/",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   14 * 24 * time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := Parse(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("邊境查驗不合格是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, x := range list {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.food_rejection (rejection_id, published_on, received_on, origin, product, reason, hs_code, disposal)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (rejection_id) DO UPDATE SET last_seen_at = now()`,
			x.ID, x.Published, x.Received, x.Origin, x.Product, x.Reason, pipeline.NullIfEmpty(x.HSCode),
			pipeline.NullIfEmpty(x.Disposal))
		if err != nil {
			return res, fmt.Errorf("upsert rejection %s: %w", x.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(*x.Published)
	}
	return res, tx.Commit()
}
