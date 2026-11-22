package rail

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
	TRAAlertPath   = "v3/Rail/TRA/Alert"
	THSRAlertPath  = "v2/Rail/THSR/AlertInfo"
	TRALivePath    = "v3/Rail/TRA/TrainLiveBoard"
	TRAStationPath = "v3/Rail/TRA/Station"
)

func tdxTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0001-") {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	t = t.In(pipeline.Taipei)
	return &t
}

type Alert struct {
	Operator, ID, Title, Description, Reason string
	Status, Level                            int
	Lines, Stations                          []string
	StartsAt, EndsAt, PublishedAt, UpdatedAt *time.Time
}

func ParseTRAAlerts(body []byte) ([]Alert, error) {
	var doc struct {
		Alerts []struct {
			AlertID     string `json:"AlertID"`
			Title       string `json:"Title"`
			Description string `json:"Description"`
			Status      int    `json:"Status"`
			Level       int    `json:"Level"`
			Reason      string `json:"Reason"`
			Scope       struct {
				Stations []struct {
					StationName string `json:"StationName"`
				} `json:"Stations"`
				Lines []struct {
					LineName string `json:"LineName"`
				} `json:"Lines"`
			} `json:"Scope"`
			StartTime, EndTime, PublishTime, UpdateTime string
		} `json:"Alerts"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode TRA alerts: %w", err)
	}
	var out []Alert
	for _, a := range doc.Alerts {
		x := Alert{Operator: "TRA", ID: strings.TrimSpace(a.AlertID), Title: strings.TrimSpace(a.Title),
			Description: pipeline.Truncate(strings.TrimSpace(a.Description), 2000), Reason: strings.TrimSpace(a.Reason),
			Status: a.Status, Level: a.Level, StartsAt: tdxTime(a.StartTime), EndsAt: tdxTime(a.EndTime),
			PublishedAt: tdxTime(a.PublishTime), UpdatedAt: tdxTime(a.UpdateTime)}
		if x.ID == "" || x.Title == "" || len(x.ID) > 60 {
			continue
		}
		for _, l := range a.Scope.Lines {
			if n := strings.TrimSpace(l.LineName); n != "" {
				x.Lines = append(x.Lines, n)
			}
		}
		for _, s := range a.Scope.Stations {
			if n := strings.TrimSpace(s.StationName); n != "" {
				x.Stations = append(x.Stations, n)
			}
		}
		out = append(out, x)
	}
	return out, nil
}

func ParseTHSRAlerts(body []byte) ([]Alert, error) {
	var rows []struct {
		AlertID                                     string `json:"AlertID"`
		Title                                       string `json:"Title"`
		Description                                 string `json:"Description"`
		Reason                                      string `json:"Reason"`
		StartTime, EndTime, PublishTime, UpdateTime string
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode THSR alerts: %w", err)
	}
	var out []Alert
	for _, a := range rows {
		id := strings.TrimSpace(a.AlertID)
		if id == "" || strings.Trim(id, "0-") == "" || strings.Contains(a.Title, "營運正常") || len(id) > 60 {
			continue
		}
		out = append(out, Alert{Operator: "THSR", ID: id, Title: strings.TrimSpace(a.Title),
			Description: pipeline.Truncate(strings.TrimSpace(a.Description), 2000), Reason: strings.TrimSpace(a.Reason),
			StartsAt: tdxTime(a.StartTime), EndsAt: tdxTime(a.EndTime), PublishedAt: tdxTime(a.PublishTime),
			UpdatedAt: tdxTime(a.UpdateTime)})
	}
	return out, nil
}

type AlertPipeline struct {
	TDX *tdx.Client
}

func (p *AlertPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "rail_alert",
		Name:         "臺鐵與高鐵營運通阻",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   2 * time.Hour,
		StaleAfter:   5 * time.Hour,
	}
}

func (p *AlertPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := p.TDX.Get(ctx, env, TRAAlertPath, nil)
	if err != nil {
		return res, err
	}
	tra, err := ParseTRAAlerts(body)
	if err != nil {
		return res, err
	}
	body, err = p.TDX.Get(ctx, env, THSRAlertPath, nil)
	if err != nil {
		return res, err
	}
	thsr, err := ParseTHSRAlerts(body)
	if err != nil {
		return res, err
	}
	all := append(tra, thsr...)
	res.RowsFetched = len(all)
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, a := range all {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.rail_alert (operator, alert_id, title, description, reason, status, level, lines, stations,
			                             starts_at, ends_at, published_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (operator, alert_id) DO UPDATE SET
				title = EXCLUDED.title, description = EXCLUDED.description, reason = EXCLUDED.reason,
				status = EXCLUDED.status, level = EXCLUDED.level, lines = EXCLUDED.lines, stations = EXCLUDED.stations,
				starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at, published_at = EXCLUDED.published_at,
				updated_at = EXCLUDED.updated_at, last_seen_at = now()`,
			a.Operator, a.ID, a.Title, pipeline.NullIfEmpty(a.Description), pipeline.NullIfEmpty(a.Reason), a.Status,
			a.Level, textArray(a.Lines), textArray(a.Stations), a.StartsAt, a.EndsAt, a.PublishedAt, a.UpdatedAt)
		if err != nil {
			return res, fmt.Errorf("upsert rail alert %s: %w", a.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

func textArray(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(x) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type Train struct {
	No, Type, StationID, StationName string
	StationStatus, DelayMin          int
	UpdatedAt                        *time.Time
}

func ParseLive(body []byte) ([]Train, *time.Time, error) {
	var doc struct {
		UpdateTime string `json:"UpdateTime"`
		Boards     []struct {
			TrainNo       string `json:"TrainNo"`
			TrainTypeName struct {
				ZhTw string `json:"Zh_tw"`
			} `json:"TrainTypeName"`
			StationID   string `json:"StationID"`
			StationName struct {
				ZhTw string `json:"Zh_tw"`
			} `json:"StationName"`
			TrainStationStatus int    `json:"TrainStationStatus"`
			DelayTime          int    `json:"DelayTime"`
			UpdateTime         string `json:"UpdateTime"`
		} `json:"TrainLiveBoards"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, nil, fmt.Errorf("decode TRA live board: %w", err)
	}
	var out []Train
	for _, b := range doc.Boards {
		t := Train{No: strings.TrimSpace(b.TrainNo), Type: strings.TrimSpace(b.TrainTypeName.ZhTw),
			StationID: strings.TrimSpace(b.StationID), StationName: strings.TrimSpace(b.StationName.ZhTw),
			StationStatus: b.TrainStationStatus, DelayMin: b.DelayTime, UpdatedAt: tdxTime(b.UpdateTime)}
		if t.No == "" || len(t.No) > 10 || t.DelayMin < 0 || t.DelayMin > 1440 {
			continue
		}
		out = append(out, t)
	}
	return out, tdxTime(doc.UpdateTime), nil
}

type LivePipeline struct {
	TDX *tdx.Client
}

func (p *LivePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "tra_live",
		Name:         "臺鐵列車即時位置與誤點",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

const DelayedMin = 5

func (p *LivePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := p.TDX.Get(ctx, env, TRALivePath, nil)
	if err != nil {
		return res, err
	}
	trains, updated, err := ParseLive(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(trains)
	at := env.Now()
	if updated != nil {
		at = *updated
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.tra_train_live`); err != nil {
		return res, err
	}
	delayed, sum, max := 0, 0, 0
	for _, t := range trains {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.tra_train_live (train_no, train_type, station_id, station_name, station_status, delay_min, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (train_no) DO NOTHING`,
			t.No, pipeline.NullIfEmpty(t.Type), pipeline.NullIfEmpty(t.StationID), pipeline.NullIfEmpty(t.StationName),
			t.StationStatus, t.DelayMin, t.UpdatedAt); err != nil {
			return res, fmt.Errorf("insert train %s: %w", t.No, err)
		}
		res.RowsInserted++
		if t.DelayMin >= DelayedMin {
			delayed++
		}
		sum += t.DelayMin
		if t.DelayMin > max {
			max = t.DelayMin
		}
	}
	var avg any
	if len(trains) > 0 {
		avg = float64(sum) / float64(len(trains))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data.tra_delay_history (observed_at, running, delayed, avg_delay_min, max_delay_min)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (observed_at) DO UPDATE SET running = EXCLUDED.running, delayed = EXCLUDED.delayed,
			avg_delay_min = EXCLUDED.avg_delay_min, max_delay_min = EXCLUDED.max_delay_min`,
		at.Truncate(time.Minute), len(trains), delayed, avg, max); err != nil {
		return res, fmt.Errorf("insert delay summary: %w", err)
	}
	res.ObserveTime(at)
	return res, tx.Commit()
}

type Station struct {
	ID, Name, City, Address string
	Lon, Lat                *float64
}

func ParseStations(body []byte) ([]Station, error) {
	var doc struct {
		Stations []struct {
			StationID   string `json:"StationID"`
			StationName struct {
				ZhTw string `json:"Zh_tw"`
			} `json:"StationName"`
			StationAddress  string `json:"StationAddress"`
			LocationCity    string `json:"LocationCity"`
			StationPosition struct {
				PositionLon float64 `json:"PositionLon"`
				PositionLat float64 `json:"PositionLat"`
			} `json:"StationPosition"`
		} `json:"Stations"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode TRA stations: %w", err)
	}
	var out []Station
	for _, s := range doc.Stations {
		x := Station{ID: strings.TrimSpace(s.StationID), Name: strings.TrimSpace(s.StationName.ZhTw),
			City:    strings.TrimSpace(s.LocationCity),
			Address: strings.TrimLeft(strings.TrimSpace(s.StationAddress), "0123456789 ")}
		if x.ID == "" || x.Name == "" || len(x.ID) > 10 {
			continue
		}
		lon, lat := s.StationPosition.PositionLon, s.StationPosition.PositionLat
		if lat > 21 && lat < 27 && lon > 118 && lon < 123 {
			x.Lon, x.Lat = &lon, &lat
		}
		out = append(out, x)
	}
	return out, nil
}

type StationPipeline struct {
	TDX *tdx.Client
}

func (p *StationPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "tra_station",
		Name:         "臺鐵車站",
		SourceAgency: "交通部（TDX 運輸資料流通服務）",
		SourceURL:    "https://tdx.transportdata.tw/",
		UpdateFreq:   7 * 24 * time.Hour,
		StaleAfter:   21 * 24 * time.Hour,
	}
}

func (p *StationPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := p.TDX.Get(ctx, env, TRAStationPath, nil)
	if err != nil {
		return res, err
	}
	list, err := ParseStations(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("臺鐵車站是空的")
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
	for _, s := range list {
		var county, lon, lat any
		if c, ok := counties.Code(s.City); ok {
			county = c
		} else if c, ok := counties.CodeFromAddress(s.Address); ok {
			county = c
		}
		if s.Lon != nil {
			lon, lat = *s.Lon, *s.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.tra_station (station_id, name, county_code, address, geom)
			VALUES ($1, $2, $3, $4, CASE WHEN $5::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($5, $6), 4326) END)
			ON CONFLICT (station_id) DO UPDATE SET
				name = EXCLUDED.name, county_code = EXCLUDED.county_code, address = EXCLUDED.address,
				geom = EXCLUDED.geom, last_seen_at = now()`,
			s.ID, s.Name, county, pipeline.NullIfEmpty(s.Address), lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert TRA station %s: %w", s.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ref.tra_station s SET county_code = c.code
		FROM ref.county c
		WHERE s.county_code IS NULL AND s.geom IS NOT NULL AND ST_Contains(c.boundary, s.geom)`); err != nil {
		return res, err
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
