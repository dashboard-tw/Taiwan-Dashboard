package ev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/tdx"
)

type City struct {
	Name, County string
}

var Cities = []City{
	{"Taipei", "63000"}, {"NewTaipei", "65000"}, {"Taoyuan", "68000"}, {"Taichung", "66000"}, {"Tainan", "67000"},
	{"Kaohsiung", "64000"}, {"Keelung", "10017"}, {"Hsinchu", "10018"}, {"HsinchuCounty", "10004"},
	{"MiaoliCounty", "10005"}, {"ChanghuaCounty", "10007"}, {"NantouCounty", "10008"}, {"YunlinCounty", "10009"},
	{"Chiayi", "10020"}, {"ChiayiCounty", "10010"}, {"PingtungCounty", "10013"}, {"YilanCounty", "10002"},
	{"HualienCounty", "10015"}, {"TaitungCounty", "10014"}, {"PenghuCounty", "10016"}, {"KinmenCounty", "09020"},
}

func cities(list []City) []City {
	if len(list) == 0 {
		return Cities
	}
	return list
}

var connectorNames = map[int]string{1: "CCS1", 2: "CCS2", 3: "CHAdeMO", 4: "Tesla", 5: "J1772", 6: "Type 2"}

type Station struct {
	ID, Name                               string
	OperationType, Spaces, Points          *int
	AC, DC                                 int
	Types                                  []string
	ServiceTime, ChargingRate, ParkingRate string
	Lon, Lat                               *float64
}

func ParseStations(body []byte) ([]Station, error) {
	var doc struct {
		Stations []struct {
			ID   string `json:"StationID"`
			Name struct {
				ZhTw string `json:"Zh_tw"`
			} `json:"StationName"`
			OperationType *int     `json:"OperationType"`
			Lat           *float64 `json:"PositionLat"`
			Lon           *float64 `json:"PositionLon"`
			Spaces        *int     `json:"Spaces"`
			Points        *int     `json:"ChargingPoints"`
			Connectors    []struct {
				Type     int `json:"Type"`
				Power    int `json:"Power"`
				Quantity int `json:"Quantity"`
			} `json:"Connectors"`
			ServiceTime  string `json:"ServiceTime"`
			ChargingRate string `json:"ChargingRate"`
			ParkingRate  string `json:"ParkingRate"`
		} `json:"Stations"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode EV stations: %w", err)
	}
	var out []Station
	for _, r := range doc.Stations {
		s := Station{ID: strings.TrimSpace(r.ID), Name: strings.TrimSpace(r.Name.ZhTw), OperationType: r.OperationType,
			Spaces: r.Spaces, Points: r.Points, ServiceTime: pipeline.Truncate(strings.TrimSpace(r.ServiceTime), 200),
			ChargingRate: pipeline.Truncate(strings.TrimSpace(r.ChargingRate), 300),
			ParkingRate:  pipeline.Truncate(strings.TrimSpace(r.ParkingRate), 300)}
		if s.ID == "" || s.Name == "" || len(s.ID) > 40 {
			continue
		}
		types := map[string]bool{}
		for _, c := range r.Connectors {
			if c.Quantity <= 0 || c.Quantity > 500 {
				continue
			}
			if c.Power == 2 {
				s.DC += c.Quantity
			} else {
				s.AC += c.Quantity
			}
			if n, ok := connectorNames[c.Type]; ok {
				types[n] = true
			} else {
				types["其他"] = true
			}
		}
		for t := range types {
			s.Types = append(s.Types, t)
		}
		sort.Strings(s.Types)
		if r.Lat != nil && r.Lon != nil && *r.Lat > 21 && *r.Lat < 27 && *r.Lon > 118 && *r.Lon < 123 {
			s.Lon, s.Lat = r.Lon, r.Lat
		}
		out = append(out, s)
	}
	return out, nil
}

type StationPipeline struct {
	TDX    *tdx.Client
	Cities []City // empty uses Cities
}

func (p *StationPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "ev_station",
		Name:         "電動車充電站",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   21 * 24 * time.Hour,
	}
}

func (p *StationPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var rows [][]any
	seen := map[string]bool{}
	for _, c := range cities(p.Cities) {
		body, err := p.TDX.Get(ctx, env, "v1/EV/Station/City/"+c.Name, nil)
		if err != nil {
			res.Warnf("%s：%v", c.Name, err)
			continue
		}
		list, err := ParseStations(body)
		if err != nil {
			res.Warnf("%s：%v", c.Name, err)
			continue
		}
		res.RowsFetched += len(list)
		for _, s := range list {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			var point any
			if s.Lon != nil {
				point = fmt.Sprintf("POINT(%f %f)", *s.Lon, *s.Lat)
			}
			types := "{" + strings.Join(s.Types, ",") + "}"
			rows = append(rows, []any{s.ID, s.Name, c.County, intText(s.OperationType), intText(s.Spaces), intText(s.Points),
				fmt.Sprint(s.AC), fmt.Sprint(s.DC), types, pipeline.NullIfEmpty(s.ServiceTime),
				pipeline.NullIfEmpty(s.ChargingRate), pipeline.NullIfEmpty(s.ParkingRate), point})
		}
	}
	if len(rows) == 0 {
		return res, errors.New("充電站是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO ref.ev_station (station_id, name, county_code,
		operation_type, spaces, charging_points, ac_connectors, dc_connectors, connector_types, service_time, charging_rate,
		parking_rate, geom)
		SELECT v.id, v.name, v.county, v.op::smallint, v.spaces::int, v.points::int, v.ac::int, v.dc::int, v.types::text[],
		       v.service, v.charge, v.parking, ST_GeomFromText(v.point, 4326)
		FROM (VALUES`, rows, `) AS v (id, name, county, op, spaces, points, ac, dc, types, service, charge, parking, point)
		ON CONFLICT (station_id) DO UPDATE SET name = EXCLUDED.name, county_code = EXCLUDED.county_code,
			operation_type = EXCLUDED.operation_type, spaces = EXCLUDED.spaces, charging_points = EXCLUDED.charging_points,
			ac_connectors = EXCLUDED.ac_connectors, dc_connectors = EXCLUDED.dc_connectors,
			connector_types = EXCLUDED.connector_types, service_time = EXCLUDED.service_time,
			charging_rate = EXCLUDED.charging_rate, parking_rate = EXCLUDED.parking_rate, geom = EXCLUDED.geom,
			last_seen_at = now()`); err != nil {
		return res, fmt.Errorf("upsert EV stations: %w", err)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

func intText(p *int) any {
	if p == nil {
		return nil
	}
	return fmt.Sprint(*p)
}

type Live struct {
	Station             string
	Free, InUse, Broken int
	Updated             *time.Time
}

func ParseLive(body []byte) ([]Live, error) {
	var doc struct {
		Statuses []struct {
			Station string `json:"StationID"`
			Status  int    `json:"ConnectorStatus"`
			Updated string `json:"LastUpdateTime"`
		} `json:"LiveStatuses"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode EV live status: %w", err)
	}
	byStation := map[string]*Live{}
	var order []string
	for _, r := range doc.Statuses {
		id := strings.TrimSpace(r.Station)
		if id == "" || len(id) > 40 {
			continue
		}
		l := byStation[id]
		if l == nil {
			l = &Live{Station: id}
			byStation[id] = l
			order = append(order, id)
		}
		switch r.Status {
		case 1:
			l.Free++
		case 2:
			l.InUse++
		case 3:
			l.Broken++
		}
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.Updated)); err == nil && (l.Updated == nil || t.After(*l.Updated)) {
			l.Updated = &t
		}
	}
	out := make([]Live, 0, len(order))
	for _, id := range order {
		out = append(out, *byStation[id])
	}
	return out, nil
}

type LivePipeline struct {
	TDX    *tdx.Client
	Cities []City // empty uses Cities
}

func (p *LivePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "ev_live",
		Name:         "電動車充電站即時狀態",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   50 * time.Hour,
	}
}

func (p *LivePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var rows [][]any
	var fetched []string
	seen := map[string]bool{}
	for _, c := range cities(p.Cities) {
		body, err := p.TDX.Get(ctx, env, "v1/EV/ConnectorLiveStatus/City/"+c.Name, nil)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			continue // 這個縣市沒有即時狀態
		}
		var list []Live
		if err == nil {
			list, err = ParseLive(body)
		}
		if err != nil {
			res.Warnf("%s：%v", c.Name, err)
			continue
		}
		res.RowsFetched += len(list)
		fetched = append(fetched, c.County)
		for _, l := range list {
			if seen[l.Station] {
				continue
			}
			seen[l.Station] = true
			rows = append(rows, []any{l.Station, l.Free, l.InUse, l.Broken, l.Updated})
		}
	}
	if len(rows) == 0 {
		return res, errors.New("充電站即時狀態都沒有取得")
	}
	now := env.Now().Truncate(time.Minute)
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM data.ev_station_live l
		WHERE l.station_id IN (SELECT station_id FROM ref.ev_station WHERE county_code = ANY ($1))
		   OR l.station_id NOT IN (SELECT station_id FROM ref.ev_station)`, fetched); err != nil {
		return res, err
	}
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.ev_station_live (station_id, free, in_use,
		broken, updated_at) VALUES`, rows, `ON CONFLICT (station_id) DO UPDATE SET free = EXCLUDED.free,
		in_use = EXCLUDED.in_use, broken = EXCLUDED.broken, updated_at = EXCLUDED.updated_at, fetched_at = now()`); err != nil {
		return res, fmt.Errorf("insert EV live status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data.ev_county_history (observed_at, county_code, free, in_use, broken)
		SELECT $1, s.county_code, sum(l.free), sum(l.in_use), sum(l.broken)
		FROM data.ev_station_live l JOIN ref.ev_station s USING (station_id)
		WHERE s.county_code = ANY ($2)
		GROUP BY s.county_code
		ON CONFLICT (observed_at, county_code) DO NOTHING`, now, fetched); err != nil {
		return res, err
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
