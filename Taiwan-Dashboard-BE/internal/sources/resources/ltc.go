package resources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultLTCURL = "https://ltcpap.mohw.gov.tw/publish/abc.csv"

type LTCFacility struct {
	ID, Name, ABC, County, Town, Address string
	Services                             []string
	Beds, Residents                      *int
	ContractEnd                          *time.Time
	Lon, Lat                             *float64
}

func ParseLTC(body []byte) ([]LTCFacility, error) {
	rows, err := readCSV(body)
	if err != nil {
		return nil, err
	}
	byID := map[string]*LTCFacility{}
	var order []string
	num := func(s string) *int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v < 0 || v > 100000 {
			return nil
		}
		return &v
	}
	for _, r := range rows {
		id := r["機構代碼"]
		if id == "" || len(id) > 20 || r["機構名稱"] == "" {
			continue
		}
		f, ok := byID[id]
		if !ok {
			f = &LTCFacility{ID: id, Name: r["機構名稱"], County: r["縣市"], Town: r["區"], Address: r["地址全址"],
				Beds: num(r["開放床數"]), Residents: num(r["現有住民"])}
			if abc := strings.TrimSpace(r["O_ABC"]); abc == "A" || abc == "B" || abc == "C" {
				f.ABC = abc
			}
			lon, err1 := pipeline.ParseNumber(r["經度"])
			lat, err2 := pipeline.ParseNumber(r["緯度"])
			if err1 == nil && err2 == nil && lon != nil && lat != nil && *lat > 20 && *lat < 27 && *lon > 116 && *lon < 123 {
				f.Lon, f.Lat = lon, lat
			}
			byID[id] = f
			order = append(order, id)
		}
		if s := strings.TrimSpace(r["特約服務項目"]); s != "" {
			f.Services = append(f.Services, s)
		}
		if t := parseYMD(r["特約迄日"]); t != nil && (f.ContractEnd == nil || t.After(*f.ContractEnd)) {
			f.ContractEnd = t
		}
	}
	out := make([]LTCFacility, 0, len(order))
	for _, id := range order {
		f := byID[id]
		f.Services = uniqueSorted(f.Services)
		out = append(out, *f)
	}
	return out, nil
}

func uniqueSorted(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

type LTCPipeline struct {
	URL string
}

func (p *LTCPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "ltc_facility",
		Name:         "長照服務據點（ABC 據點）",
		SourceAgency: "衛生福利部",
		SourceURL:    "https://data.gov.tw/dataset/88270",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   40 * 24 * time.Hour,
	}
}

func (p *LTCPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseLTC(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("長照據點是空的")
	}
	known, err := countyCodes(ctx, env)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, f := range list {
		var county, lon, lat any
		if known[f.County] {
			county = f.County
		}
		if f.Lon != nil {
			lon, lat = *f.Lon, *f.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.ltc_facility (facility_id, name, abc, county_code, town_code, address, services, beds,
			                              residents, contract_end, geom)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			        CASE WHEN $11::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($11, $12), 4326) END)
			ON CONFLICT (facility_id) DO UPDATE SET
				name = EXCLUDED.name, abc = EXCLUDED.abc, county_code = EXCLUDED.county_code, town_code = EXCLUDED.town_code,
				address = EXCLUDED.address, services = EXCLUDED.services, beds = EXCLUDED.beds,
				residents = EXCLUDED.residents, contract_end = EXCLUDED.contract_end, geom = EXCLUDED.geom,
				last_seen_at = now()`,
			f.ID, f.Name, pipeline.NullIfEmpty(f.ABC), county, pipeline.NullIfEmpty(f.Town), pipeline.NullIfEmpty(f.Address),
			pipeline.NullIfEmpty(strings.Join(f.Services, "、")), f.Beds, f.Residents, f.ContractEnd, lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert ltc facility %s: %w", f.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
