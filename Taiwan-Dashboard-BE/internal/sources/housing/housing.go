package housing

import (
	"archive/zip"
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

const DefaultURL = "https://plvr.land.moi.gov.tw/DownloadSeason?season=%s&type=zip&fileName=lvr_landcsv.zip"

type Deal struct {
	Serial, Kind, Season, District, Target, BuildingType, MainUse, Address string
	Date                                                                   time.Time
	TotalPrice                                                             *int64
	UnitPrice, Area                                                        *float64
	Rooms                                                                  *int
	Special                                                                bool
	Letter                                                                 string // county letter of the file
}

var kinds = map[string]string{"a": "sale", "b": "presale", "c": "rent"}

var letterCounty = map[string]string{
	"a": "63000", "b": "66000", "c": "10017", "d": "67000", "e": "64000", "f": "65000", "g": "10002",
	"h": "68000", "i": "10020", "j": "10004", "k": "10005", "m": "10008", "n": "10007", "o": "10018",
	"p": "10009", "q": "10010", "t": "10013", "u": "10015", "v": "10014", "w": "09020", "x": "10016",
	"z": "09007",
}

var specialWords = []string{"親友", "員工", "共有人", "特殊關係"}

func rocDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 6 || len(s) > 7 {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return time.Time{}, false
	}
	y, m, d := n/10000, n/100%100, n%100
	if y < 80 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y+1911, time.Month(m), d, 0, 0, 0, 0, pipeline.Taipei), true
}

func ParseFile(body []byte, kind, season, letter string) ([]Deal, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	dateCol, areaCol, priceCol := "交易年月日", "建物移轉總面積平方公尺", "總價元"
	if kind == "rent" {
		dateCol, areaCol, priceCol = "租賃年月日", "建物總面積平方公尺", "總額元"
	}
	for _, need := range []string{"編號", dateCol, priceCol} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("實價登錄少了欄位 %s", need)
		}
	}
	var out []Deal
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read deals: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		date, ok := rocDate(get(dateCol))
		serial := get("編號")
		if !ok || serial == "" || len(serial) > 40 {
			continue
		}
		d := Deal{Serial: serial, Kind: kind, Season: season, Date: date, District: get("鄉鎮市區"), Target: get("交易標的"),
			BuildingType: get("建物型態"), MainUse: get("主要用途"), Address: get("土地位置建物門牌"), Letter: letter}
		if v, err := strconv.ParseInt(get(priceCol), 10, 64); err == nil && v > 0 {
			d.TotalPrice = &v
		}
		if v, err := strconv.ParseFloat(get("單價元平方公尺"), 64); err == nil && v > 0 {
			d.UnitPrice = &v
		}
		if v, err := strconv.ParseFloat(get(areaCol), 64); err == nil && v > 0 {
			d.Area = &v
		}
		if v, err := strconv.Atoi(get("建物現況格局-房")); err == nil && v >= 0 && v < 100 {
			d.Rooms = &v
		}
		note := get("備註")
		for _, w := range specialWords {
			if strings.Contains(note, w) {
				d.Special = true
				break
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func ParseSeason(body []byte, season string) ([]Deal, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, nil
	}
	var out []Deal
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if len(name) != len("x_lvr_land_a.csv") || !strings.HasSuffix(name, ".csv") || name[1:11] != "_lvr_land_" {
			continue
		}
		kind, ok := kinds[name[11:12]]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 100<<20))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("unzip %s: %w", f.Name, err)
		}
		list, err := ParseFile(b, kind, season, name[:1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		out = append(out, list...)
	}
	return out, nil
}

func Season(t time.Time) string {
	t = t.In(pipeline.Taipei)
	return fmt.Sprintf("%dS%d", t.Year()-1911, (int(t.Month())-1)/3+1)
}

type Pipeline struct {
	URL string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "real_estate",
		Name:         "不動產實價登錄（買賣、預售屋、租賃）",
		SourceAgency: "內政部",
		SourceURL:    "https://plvr.land.moi.gov.tw/DownloadOpenData",
		UpdateFreq:   90 * 24 * time.Hour,
		StaleAfter:   130 * 24 * time.Hour,
	}
}

const (
	keepSeasons  = 4
	fetchSeasons = 2
	batchRows    = 500
)

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	url := p.URL
	if url == "" {
		url = DefaultURL
	}
	have := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT DISTINCT season FROM data.real_estate_deal`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return res, err
		}
		have[s] = true
	}
	rows.Close()
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}

	now := env.Now()
	var published []string
	fetched, refreshed := 0, false
	for i := 0; i < keepSeasons+2 && len(published) < keepSeasons; i++ {
		season := Season(now.AddDate(0, -3*i, 0))
		if (!refreshed || !have[season]) && fetched < fetchSeasons {
			u := fmt.Sprintf(url, season)
			body, err := env.Fetch.Get(ctx, u, u)
			var he *pipeline.HTTPError
			if err != nil && !(errors.As(err, &he) && he.Status == 404) {
				return res, err
			}
			deals, err := ParseSeason(body, season)
			if err != nil {
				return res, fmt.Errorf("%s: %w", season, err)
			}
			if len(deals) > 0 {
				n, err := p.store(ctx, env, counties, deals)
				if err != nil {
					return res, fmt.Errorf("%s: %w", season, err)
				}
				fetched++
				refreshed = true
				published = append(published, season)
				res.RowsFetched += len(deals)
				res.RowsInserted += n
				for _, d := range deals {
					if d.Kind == "sale" {
						res.ObserveTime(d.Date)
					}
				}
				continue
			}
		}
		if have[season] {
			refreshed = true
			published = append(published, season)
		}
	}
	if len(published) == 0 {
		return res, errors.New("找不到最近幾季的實價登錄")
	}
	if len(published) >= keepSeasons {
		if _, err := env.DB.ExecContext(ctx, `DELETE FROM data.real_estate_deal WHERE NOT (season = ANY ($1::text[]))`,
			"{"+strings.Join(published, ",")+"}"); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (p *Pipeline) store(ctx context.Context, env pipeline.Env, counties *pipeline.Counties, deals []Deal) (int, error) {
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	const cols = 14
	inserted := 0
	seen := map[string]bool{}
	var args []any
	flush := func() error {
		if len(args) == 0 {
			return nil
		}
		var b strings.Builder
		b.WriteString(`INSERT INTO data.real_estate_deal (serial, kind, season, deal_date, county_code, district, target,
			building_type, main_use, total_price, unit_price, area_m2, rooms, special) VALUES `)
		for i := 0; i < len(args)/cols; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(")
			for j := 0; j < cols; j++ {
				if j > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, "$%d", i*cols+j+1)
			}
			b.WriteString(")")
		}
		b.WriteString(` ON CONFLICT (serial) DO UPDATE SET kind = EXCLUDED.kind, season = EXCLUDED.season,
			deal_date = EXCLUDED.deal_date, county_code = EXCLUDED.county_code, district = EXCLUDED.district,
			target = EXCLUDED.target, building_type = EXCLUDED.building_type, main_use = EXCLUDED.main_use,
			total_price = EXCLUDED.total_price, unit_price = EXCLUDED.unit_price, area_m2 = EXCLUDED.area_m2,
			rooms = EXCLUDED.rooms, special = EXCLUDED.special, ingested_at = now()`)
		r, err := tx.ExecContext(ctx, b.String(), args...)
		if err != nil {
			return err
		}
		affected, _ := r.RowsAffected()
		inserted += int(affected)
		args = args[:0]
		return nil
	}
	for _, d := range deals {
		if seen[d.Serial] {
			continue
		}
		seen[d.Serial] = true
		var county any
		if c, ok := counties.CodeFromAddress(d.Address); ok {
			county = c
		} else if c, ok := letterCounty[d.Letter]; ok {
			county = c
		}
		args = append(args, d.Serial, d.Kind, d.Season, d.Date, county, pipeline.NullIfEmpty(d.District),
			pipeline.NullIfEmpty(d.Target), pipeline.NullIfEmpty(d.BuildingType), pipeline.NullIfEmpty(d.MainUse),
			d.TotalPrice, d.UnitPrice, d.Area, d.Rooms, d.Special)
		if len(args) >= batchRows*cols {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := flush(); err != nil {
		return 0, err
	}
	return inserted, tx.Commit()
}
