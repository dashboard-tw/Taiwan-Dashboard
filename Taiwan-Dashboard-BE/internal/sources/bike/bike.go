package bike

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/tdx"
)

type City struct {
	Name, County string
}

var Cities = []City{
	{"Taipei", "63000"}, {"NewTaipei", "65000"}, {"Taoyuan", "68000"}, {"Hsinchu", "10018"},
	{"HsinchuCounty", "10004"}, {"MiaoliCounty", "10005"}, {"Taichung", "66000"}, {"ChanghuaCounty", "10007"},
	{"YunlinCounty", "10009"}, {"Chiayi", "10020"}, {"ChiayiCounty", "10010"}, {"Tainan", "67000"},
	{"Kaohsiung", "64000"}, {"PingtungCounty", "10013"}, {"TaitungCounty", "10014"},
}

type Station struct {
	UID, Name, Address string
	Capacity           *int
	ServiceType        int // 1 YouBike 1.0, 2 YouBike 2.0
	Lon, Lat           *float64
}

func ParseStations(body []byte) ([]Station, error) {
	var rows []struct {
		StationUID  string `json:"StationUID"`
		StationName struct {
			ZhTw string `json:"Zh_tw"`
		} `json:"StationName"`
		StationAddress struct {
			ZhTw string `json:"Zh_tw"`
		} `json:"StationAddress"`
		StationPosition struct {
			PositionLon float64 `json:"PositionLon"`
			PositionLat float64 `json:"PositionLat"`
		} `json:"StationPosition"`
		BikesCapacity *int `json:"BikesCapacity"`
		ServiceType   int  `json:"ServiceType"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode bike stations: %w", err)
	}
	var out []Station
	for _, r := range rows {
		name := strings.TrimSpace(r.StationName.ZhTw)
		if i := strings.Index(name, "_"); i >= 0 && strings.HasPrefix(name, "YouBike") {
			name = name[i+1:]
		}
		s := Station{UID: strings.TrimSpace(r.StationUID), Name: name, Address: strings.TrimSpace(r.StationAddress.ZhTw),
			ServiceType: r.ServiceType}
		if s.UID == "" || s.Name == "" || len(s.UID) > 30 {
			continue
		}
		if r.BikesCapacity != nil && *r.BikesCapacity >= 0 && *r.BikesCapacity < 1000 {
			s.Capacity = r.BikesCapacity
		}
		lon, lat := r.StationPosition.PositionLon, r.StationPosition.PositionLat
		if lat > 21 && lat < 27 && lon > 118 && lon < 123 {
			s.Lon, s.Lat = &lon, &lat
		}
		out = append(out, s)
	}
	return out, nil
}

type Availability struct {
	UID                  string
	Status               int // 0 停止營運, 1 正常營運, 2 暫停營運
	Rent, Return, EBikes int
	UpdatedAt            *time.Time
}

func ParseAvailability(body []byte) ([]Availability, error) {
	var rows []struct {
		StationUID               string `json:"StationUID"`
		ServiceStatus            int    `json:"ServiceStatus"`
		AvailableRentBikes       int    `json:"AvailableRentBikes"`
		AvailableReturnBikes     int    `json:"AvailableReturnBikes"`
		AvailableRentBikesDetail struct {
			ElectricBikes int `json:"ElectricBikes"`
		} `json:"AvailableRentBikesDetail"`
		SrcUpdateTime string `json:"SrcUpdateTime"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode bike availability: %w", err)
	}
	var out []Availability
	for _, r := range rows {
		a := Availability{UID: strings.TrimSpace(r.StationUID), Status: r.ServiceStatus, Rent: r.AvailableRentBikes,
			Return: r.AvailableReturnBikes, EBikes: r.AvailableRentBikesDetail.ElectricBikes}
		if a.UID == "" || len(a.UID) > 30 || a.Rent < 0 || a.Return < 0 || a.Rent > 1000 || a.Return > 1000 {
			continue
		}
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.SrcUpdateTime)); err == nil {
			t = t.In(pipeline.Taipei)
			a.UpdatedAt = &t
		}
		out = append(out, a)
	}
	return out, nil
}

func cities(list []City) []City {
	if len(list) == 0 {
		return Cities
	}
	return list
}

func cityPath(kind, city string) string { return "v2/Bike/" + kind + "/City/" + city }

type StationPipeline struct {
	TDX    *tdx.Client
	Cities []City // empty uses Cities
}

func (p *StationPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "bike_station",
		Name:         "公共自行車站點（YouBike）",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   21 * 24 * time.Hour,
	}
}

func (p *StationPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	type cityStations struct {
		city City
		list []Station
	}
	var all []cityStations
	for _, c := range cities(p.Cities) {
		body, err := p.TDX.Get(ctx, env, cityPath("Station", c.Name), nil)
		if err != nil {
			return res, fmt.Errorf("%s: %w", c.Name, err)
		}
		list, err := ParseStations(body)
		if err != nil {
			return res, fmt.Errorf("%s: %w", c.Name, err)
		}
		if len(list) == 0 {
			res.Warnf("%s 沒有公共自行車站點", c.Name)
		}
		res.RowsFetched += len(list)
		all = append(all, cityStations{c, list})
	}
	if res.RowsFetched == 0 {
		return res, errors.New("公共自行車站點是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, cs := range all {
		for _, s := range cs.list {
			var lon, lat any
			if s.Lon != nil {
				lon, lat = *s.Lon, *s.Lat
			}
			r, err := tx.ExecContext(ctx, `
				INSERT INTO ref.bike_station (station_uid, city, county_code, name, address, capacity, service_type, geom)
				VALUES ($1, $2, $3, $4, $5, $6, $7,
				        CASE WHEN $8::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($8, $9), 4326) END)
				ON CONFLICT (station_uid) DO UPDATE SET
					city = EXCLUDED.city, county_code = EXCLUDED.county_code, name = EXCLUDED.name,
					address = EXCLUDED.address, capacity = EXCLUDED.capacity, service_type = EXCLUDED.service_type,
					geom = EXCLUDED.geom, last_seen_at = now()`,
				s.UID, cs.city.Name, cs.city.County, s.Name, pipeline.NullIfEmpty(s.Address), s.Capacity, s.ServiceType,
				lon, lat)
			if err != nil {
				return res, fmt.Errorf("upsert bike station %s: %w", s.UID, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type AvailabilityPipeline struct {
	TDX    *tdx.Client
	Cities []City // empty uses Cities
}

func (p *AvailabilityPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "bike_availability",
		Name:         "公共自行車即時車位（YouBike）",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   12 * time.Hour,
		StaleAfter:   26 * time.Hour,
	}
}

func (p *AvailabilityPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	type cityAvail struct {
		city City
		list []Availability
	}
	var all []cityAvail
	for _, c := range cities(p.Cities) {
		body, err := p.TDX.Get(ctx, env, cityPath("Availability", c.Name), nil)
		if err != nil {
			res.Warnf("%s：%v", c.Name, err)
			continue
		}
		list, err := ParseAvailability(body)
		if err != nil {
			res.Warnf("%s：%v", c.Name, err)
			continue
		}
		res.RowsFetched += len(list)
		all = append(all, cityAvail{c, list})
	}
	if len(all) == 0 {
		return res, errors.New("所有縣市的公共自行車車位都沒有取得")
	}
	observed := env.Now().Truncate(time.Minute)
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, ca := range all {
		for _, a := range ca.list {
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.bike_availability_current (station_uid, status, bikes, docks, ebikes, updated_at, fetched_at)
				SELECT $1, $2, $3, $4, $5, $6, $7
				WHERE EXISTS (SELECT 1 FROM ref.bike_station WHERE station_uid = $1)
				ON CONFLICT (station_uid) DO UPDATE SET
					status = EXCLUDED.status, bikes = EXCLUDED.bikes, docks = EXCLUDED.docks, ebikes = EXCLUDED.ebikes,
					updated_at = EXCLUDED.updated_at, fetched_at = EXCLUDED.fetched_at`,
				a.UID, a.Status, a.Rent, a.Return, a.EBikes, a.UpdatedAt, observed)
			if err != nil {
				return res, fmt.Errorf("upsert bike availability %s: %w", a.UID, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
			if a.UpdatedAt != nil {
				res.ObserveTime(*a.UpdatedAt)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.bike_county_history (county_code, observed_at, stations, bikes, docks, empty_stations, full_stations)
			SELECT s.county_code, $2, count(*) FILTER (WHERE a.status = 1),
			       coalesce(sum(a.bikes) FILTER (WHERE a.status = 1), 0), coalesce(sum(a.docks) FILTER (WHERE a.status = 1), 0),
			       count(*) FILTER (WHERE a.status = 1 AND a.bikes = 0), count(*) FILTER (WHERE a.status = 1 AND a.docks = 0)
			FROM data.bike_availability_current a
			JOIN ref.bike_station s USING (station_uid)
			WHERE s.county_code = $1 AND a.fetched_at = $2
			GROUP BY s.county_code
			ON CONFLICT (county_code, observed_at) DO NOTHING`,
			ca.city.County, observed); err != nil {
			return res, fmt.Errorf("bike summary %s: %w", ca.city.Name, err)
		}
	}
	return res, tx.Commit()
}
