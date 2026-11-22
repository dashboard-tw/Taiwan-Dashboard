package aviation

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

const (
	DeparturePath = "v2/Air/FIDS/Airport/Departure"
	ArrivalPath   = "v2/Air/FIDS/Airport/Arrival"
	AirportPath   = "v2/Air/Airport"
	AirlinePath   = "v2/Air/Airline"
)

type name struct {
	ZhTw string `json:"Zh_tw"`
	En   string `json:"En"`
}

func (n name) String() string {
	if s := strings.TrimSpace(n.ZhTw); s != "" {
		return s
	}
	return strings.TrimSpace(n.En)
}

type Airport struct {
	ID, Name, City, Country, Address string
	Lon, Lat                         *float64
}

func ParseAirports(body []byte) ([]Airport, error) {
	var rows []struct {
		ID       string `json:"AirportID"`
		Name     name   `json:"AirportName"`
		City     name   `json:"AirportCityName"`
		Country  string `json:"AirportNationality"`
		Address  string `json:"AirportAddress"`
		Position struct {
			Lon *float64 `json:"PositionLon"`
			Lat *float64 `json:"PositionLat"`
		} `json:"AirportPosition"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode airports: %w", err)
	}
	var out []Airport
	for _, r := range rows {
		a := Airport{ID: strings.TrimSpace(r.ID), Name: r.Name.String(), City: r.City.String(),
			Country: strings.TrimSpace(r.Country), Address: strings.TrimSpace(r.Address)}
		if a.ID == "" || a.Name == "" || len(a.ID) > 4 {
			continue
		}
		if p := r.Position; p.Lon != nil && p.Lat != nil && *p.Lat >= -90 && *p.Lat <= 90 && *p.Lon >= -180 && *p.Lon <= 180 {
			a.Lon, a.Lat = p.Lon, p.Lat
		}
		out = append(out, a)
	}
	return out, nil
}

type Airline struct {
	ID, Name string
}

func ParseAirlines(body []byte) ([]Airline, error) {
	var rows []struct {
		ID    string `json:"AirlineID"`
		Name  name   `json:"AirlineName"`
		Alias name   `json:"AirlineNameAlias"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode airlines: %w", err)
	}
	var out []Airline
	for _, r := range rows {
		a := Airline{ID: strings.TrimSpace(r.ID), Name: strings.TrimSpace(r.Alias.ZhTw)}
		if a.Name == "" {
			a.Name = r.Name.String()
		}
		if a.ID == "" || a.Name == "" || len(a.ID) > 4 {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

type RefPipeline struct {
	TDX *tdx.Client
}

func (p *RefPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "airport",
		Name:         "機場與航空公司",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   21 * 24 * time.Hour,
	}
}

func (p *RefPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := p.TDX.Get(ctx, env, AirportPath, nil)
	if err != nil {
		return res, err
	}
	airports, err := ParseAirports(body)
	if err != nil {
		return res, err
	}
	body, err = p.TDX.Get(ctx, env, AirlinePath, nil)
	if err != nil {
		return res, err
	}
	airlines, err := ParseAirlines(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(airports) + len(airlines)
	if len(airports) == 0 || len(airlines) == 0 {
		return res, errors.New("機場或航空公司是空的")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	var rows [][]any
	seen := map[string]bool{}
	for _, a := range airports {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		var county, point any
		if a.Country == "TW" {
			if c, ok := counties.CodeFromAddress(a.Address); ok {
				county = c
			}
		}
		if a.Lon != nil {
			point = fmt.Sprintf("POINT(%f %f)", *a.Lon, *a.Lat)
		}
		rows = append(rows, []any{a.ID, a.Name, pipeline.NullIfEmpty(a.City), pipeline.NullIfEmpty(a.Country), county, point})
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	n, err := pipeline.InsertRows(ctx, tx, `INSERT INTO ref.airport (airport_id, name, city, country, county_code, geom)
		SELECT v.id, v.name, v.city, v.country, v.county, ST_GeomFromText(v.point, 4326)
		FROM (VALUES`, rows, `) AS v (id, name, city, country, county, point)
		ON CONFLICT (airport_id) DO UPDATE SET name = EXCLUDED.name, city = EXCLUDED.city, country = EXCLUDED.country,
			county_code = EXCLUDED.county_code, geom = EXCLUDED.geom, last_seen_at = now()`)
	if err != nil {
		return res, fmt.Errorf("upsert airports: %w", err)
	}
	res.RowsInserted += n
	rows = rows[:0]
	seen = map[string]bool{}
	for _, a := range airlines {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		rows = append(rows, []any{a.ID, a.Name})
	}
	n, err = pipeline.InsertRows(ctx, tx, `INSERT INTO ref.airline (airline_id, name) VALUES`, rows,
		`ON CONFLICT (airline_id) DO UPDATE SET name = EXCLUDED.name, last_seen_at = now()`)
	if err != nil {
		return res, fmt.Errorf("upsert airlines: %w", err)
	}
	res.RowsInserted += n
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type Flight struct {
	Direction         string // dep, arr
	Airport, Other    string
	Airline, Number   string
	Date              time.Time
	Scheduled         time.Time
	Estimated, Actual *time.Time
	Status, Remark    string
	Cargo             bool
	Updated           *time.Time
}

func Status(remark string) string {
	r := strings.ToUpper(remark)
	switch {
	case strings.Contains(r, "取消") || strings.Contains(r, "CANCEL"):
		return "cancelled"
	case strings.Contains(r, "延誤") || strings.Contains(r, "延遲") || strings.Contains(r, "DELAY"):
		return "delayed"
	case strings.Contains(r, "時間更改") || strings.Contains(r, "SCHEDULE CHANGE"):
		return "changed"
	case strings.Contains(r, "出發") || strings.Contains(r, "已到") || strings.Contains(r, "DEPARTED") || strings.Contains(r, "ARRIVED"):
		return "done"
	case strings.Contains(r, "準時") || strings.Contains(r, "ON TIME"):
		return "on_time"
	case strings.Contains(r, "加班"):
		return "extra"
	}
	return "other"
}

func localTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := pipeline.ParseTaipeiTime(s, "2006-01-02T15:04", "2006-01-02T15:04:05")
	if err != nil {
		return nil
	}
	return &t
}

func ParseFlights(body []byte, dir string) ([]Flight, error) {
	var rows []struct {
		Date      string `json:"FlightDate"`
		Number    string `json:"FlightNumber"`
		Airline   string `json:"AirlineID"`
		From      string `json:"DepartureAirportID"`
		To        string `json:"ArrivalAirportID"`
		SchedDep  string `json:"ScheduleDepartureTime"`
		EstDep    string `json:"EstimatedDepartureTime"`
		ActDep    string `json:"ActualDepartureTime"`
		SchedArr  string `json:"ScheduleArrivalTime"`
		EstArr    string `json:"EstimatedArrivalTime"`
		ActArr    string `json:"ActualArrivalTime"`
		RemarkDep string `json:"DepartureRemark"`
		RemarkArr string `json:"ArrivalRemark"`
		Cargo     bool   `json:"IsCargo"`
		Update    string `json:"UpdateTime"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode FIDS: %w", err)
	}
	var out []Flight
	for _, r := range rows {
		f := Flight{Direction: dir, Airline: strings.TrimSpace(r.Airline), Number: strings.TrimSpace(r.Number), Cargo: r.Cargo}
		var sched *time.Time
		if dir == "dep" {
			f.Airport, f.Other = strings.TrimSpace(r.From), strings.TrimSpace(r.To)
			sched, f.Estimated, f.Actual, f.Remark = localTime(r.SchedDep), localTime(r.EstDep), localTime(r.ActDep), r.RemarkDep
		} else {
			f.Airport, f.Other = strings.TrimSpace(r.To), strings.TrimSpace(r.From)
			sched, f.Estimated, f.Actual, f.Remark = localTime(r.SchedArr), localTime(r.EstArr), localTime(r.ActArr), r.RemarkArr
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(r.Date))
		if sched == nil || err != nil || f.Airport == "" || f.Airline == "" || f.Number == "" || len(f.Airport) > 4 {
			continue
		}
		f.Date, f.Scheduled = d, *sched
		f.Remark = strings.TrimSpace(f.Remark)
		f.Status = Status(f.Remark)
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.Update)); err == nil {
			f.Updated = &t
		}
		out = append(out, f)
	}
	return out, nil
}

type FlightPipeline struct {
	TDX      *tdx.Client
	KeepDays int // zero is 30
}

func (p *FlightPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "flight",
		Name:         "機場航班動態（全國各機場）",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   3 * time.Hour,
		StaleAfter:   7 * time.Hour,
	}
}

func (p *FlightPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var flights []Flight
	for _, d := range []struct{ dir, path string }{{"dep", DeparturePath}, {"arr", ArrivalPath}} {
		body, err := p.TDX.Get(ctx, env, d.path, nil)
		if err != nil {
			return res, fmt.Errorf("%s: %w", d.dir, err)
		}
		list, err := ParseFlights(body, d.dir)
		if err != nil {
			return res, err
		}
		flights = append(flights, list...)
	}
	res.RowsFetched = len(flights)
	if len(flights) == 0 {
		return res, errors.New("航班是空的")
	}
	type key struct {
		dir, airport, airline, number string
		sched                         time.Time
	}
	seen := map[key]bool{}
	var rows [][]any
	var updated time.Time
	for _, f := range flights {
		k := key{f.Direction, f.Airport, f.Airline, f.Number, f.Scheduled}
		if seen[k] {
			continue
		}
		seen[k] = true
		if f.Updated != nil && f.Updated.After(updated) {
			updated = *f.Updated
		}
		rows = append(rows, []any{f.Direction, f.Airport, f.Airline, f.Number, f.Scheduled, f.Date,
			pipeline.NullIfEmpty(f.Other), f.Estimated, f.Actual, f.Status, pipeline.NullIfEmpty(f.Remark), f.Cargo})
	}
	keep := p.KeepDays
	if keep == 0 {
		keep = 30
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.flight (direction, airport_id, airline_id,
		flight_no, scheduled_at, flight_date, other_airport, estimated_at, actual_at, status, remark, cargo) VALUES`, rows,
		`ON CONFLICT (direction, airport_id, airline_id, flight_no, scheduled_at) DO UPDATE SET
			flight_date = EXCLUDED.flight_date, other_airport = EXCLUDED.other_airport, estimated_at = EXCLUDED.estimated_at,
			actual_at = EXCLUDED.actual_at, status = EXCLUDED.status, remark = EXCLUDED.remark, cargo = EXCLUDED.cargo,
			last_seen_at = now()`); err != nil {
		return res, fmt.Errorf("upsert flights: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.flight WHERE flight_date < $1`,
		pipeline.TaipeiDate(env.Now()).AddDate(0, 0, -keep)); err != nil {
		return res, err
	}
	if updated.IsZero() {
		updated = env.Now()
	}
	res.ObserveTime(updated)
	return res, tx.Commit()
}
