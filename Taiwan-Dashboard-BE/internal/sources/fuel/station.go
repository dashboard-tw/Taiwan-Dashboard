package fuel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultStationURL = "https://vipmbr.cpc.com.tw/opendata/getstationinfo"

type Station struct {
	ID, Name, Kind, County, Town, Address, Hours   string
	Operating, SelfService, Freeway, Has98, Diesel bool
	Lon, Lat                                       *float64
}

type flag bool

func (f *flag) UnmarshalJSON(b []byte) error {
	*f = flag(strings.Trim(string(b), `"`) == "1")
	return nil
}

func ParseStations(body []byte) ([]Station, error) {
	var rows []struct {
		ID      string   `json:"站代號"`
		Kind    string   `json:"類別"`
		Name    string   `json:"站名"`
		County  string   `json:"縣市"`
		Town    string   `json:"鄉鎮區"`
		Address string   `json:"地址"`
		Open    flag     `json:"營業中"`
		Freeway flag     `json:"國道高速公路"`
		U98     flag     `json:"無鉛98"`
		Diesel  flag     `json:"超柴"`
		Self    flag     `json:"刷卡自助"`
		SelfDsl flag     `json:"自助柴油站"`
		Lon     *float64 `json:"經度"`
		Lat     *float64 `json:"緯度"`
		Hours   string   `json:"營業時間"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode CPC stations: %w", err)
	}
	var out []Station
	for _, r := range rows {
		s := Station{ID: strings.TrimSpace(r.ID), Name: strings.TrimSpace(r.Name), Kind: strings.TrimSpace(r.Kind),
			County: strings.TrimSpace(r.County), Town: strings.TrimSpace(r.Town), Hours: strings.TrimSpace(r.Hours),
			Operating: bool(r.Open), SelfService: bool(r.Self) || bool(r.SelfDsl), Freeway: bool(r.Freeway),
			Has98: bool(r.U98), Diesel: bool(r.Diesel)}
		if s.ID == "" || s.Name == "" || len(s.ID) > 20 {
			continue
		}
		s.Address = strings.TrimSpace(r.Address)
		if s.Address != "" && !strings.HasPrefix(s.Address, s.County) {
			s.Address = s.County + s.Town + s.Address
		}
		if r.Lon != nil && r.Lat != nil && *r.Lat > 21 && *r.Lat < 27 && *r.Lon > 118 && *r.Lon < 123 {
			s.Lon, s.Lat = r.Lon, r.Lat
		}
		out = append(out, s)
	}
	return out, nil
}

type StationPipeline struct {
	URL string
}

func (p *StationPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "fuel_station",
		Name:         "台灣中油加油站",
		SourceAgency: "台灣中油",
		SourceURL:    "https://data.gov.tw/dataset/6065",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   7 * 24 * time.Hour,
	}
}

func (p *StationPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseStations(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("加油站是空的")
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
	unknown := 0
	for _, s := range list {
		var county, lon, lat any
		if c, ok := counties.Code(s.County); ok {
			county = c
		} else {
			unknown++
		}
		if s.Lon != nil {
			lon, lat = *s.Lon, *s.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.fuel_station (station_id, name, kind, county_code, town, address, open_hours, operating,
			                              self_service, freeway, has_98, has_diesel, geom)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			        CASE WHEN $13::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($13, $14), 4326) END)
			ON CONFLICT (station_id) DO UPDATE SET
				name = EXCLUDED.name, kind = EXCLUDED.kind, county_code = EXCLUDED.county_code, town = EXCLUDED.town,
				address = EXCLUDED.address, open_hours = EXCLUDED.open_hours, operating = EXCLUDED.operating,
				self_service = EXCLUDED.self_service, freeway = EXCLUDED.freeway, has_98 = EXCLUDED.has_98,
				has_diesel = EXCLUDED.has_diesel, geom = EXCLUDED.geom, last_seen_at = now()`,
			s.ID, s.Name, pipeline.NullIfEmpty(s.Kind), county, pipeline.NullIfEmpty(s.Town), pipeline.NullIfEmpty(s.Address),
			pipeline.NullIfEmpty(s.Hours), s.Operating, s.SelfService, s.Freeway, s.Has98, s.Diesel, lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert fuel station %s: %w", s.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if unknown > 0 {
		res.Warnf("%d 個加油站對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
