package traffic

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultAccidentURL = "https://opdadm.moi.gov.tw/api/v1/no-auth/resource/api/dataset/02D40248-7CAA-4354-82EA-E27AB8DCAB39/resource/67214307-AD41-4FE7-A04F-D8051A0301DB/download"

type Accident struct {
	ID                                       string
	OccurredAt                               time.Time
	Police, Place, Weather, Light, RoadClass string
	SpeedLimit                               *int
	Type, Cause, Vehicle                     string
	Deaths, Injuries                         int
	Lon, Lat                                 *float64
}

var casualtyRE = regexp.MustCompile(`死亡(\d+);受傷(\d+)`)

func ParseAccidents(body []byte) ([]Accident, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read A1 header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"發生日期", "發生時間", "發生地點", "死亡受傷人數"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("A1 少了欄位 %s", need)
		}
	}
	byID := map[string]*Accident{}
	var order []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read A1: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		date, clock, place := get("發生日期"), get("發生時間"), get("發生地點")
		if len(date) != 8 || place == "" {
			continue
		}
		if len(clock) < 6 {
			clock = strings.Repeat("0", 6-len(clock)) + clock
		}
		at, err := time.ParseInLocation("20060102150405", date+clock, pipeline.Taipei)
		if err != nil {
			continue
		}
		sum := sha1.Sum([]byte(date + clock + "|" + place))
		id := hex.EncodeToString(sum[:10])
		a, ok := byID[id]
		if !ok {
			m := casualtyRE.FindStringSubmatch(get("死亡受傷人數"))
			if m == nil {
				continue
			}
			a = &Accident{ID: id, OccurredAt: at, Police: get("處理單位名稱警局層"), Place: pipeline.Truncate(place, 200),
				Weather: get("天候名稱"), Light: get("光線名稱"), RoadClass: get("道路類別-第1當事者-名稱"),
				Type: get("事故類型及型態大類別名稱"), Cause: get("肇因研判子類別名稱-主要")}
			a.Deaths, _ = strconv.Atoi(m[1])
			a.Injuries, _ = strconv.Atoi(m[2])
			if v, err := strconv.Atoi(get("速限-第1當事者")); err == nil && v > 0 && v <= 130 {
				a.SpeedLimit = &v
			}
			lon, err1 := pipeline.ParseNumber(get("經度"))
			lat, err2 := pipeline.ParseNumber(get("緯度"))
			if err1 == nil && err2 == nil && lon != nil && lat != nil && *lat > 21 && *lat < 27 && *lon > 118 && *lon < 123 {
				a.Lon, a.Lat = lon, lat
			}
			byID[id] = a
			order = append(order, id)
		}
		if get("當事者順位") == "1" && a.Vehicle == "" {
			a.Vehicle = get("當事者區分-類別-大類別名稱-車種")
		}
	}
	out := make([]Accident, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

type AccidentPipeline struct {
	URL string
}

func (p *AccidentPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "traffic_accident",
		Name:         "A1 類道路交通事故",
		SourceAgency: "內政部警政署",
		SourceURL:    "https://data.gov.tw/dataset/12818",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

func (p *AccidentPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseAccidents(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("A1 事故是空的")
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
	years := map[int]bool{}
	for _, a := range list {
		years[a.OccurredAt.Year()] = true
	}
	for y := range years {
		if _, err := tx.ExecContext(ctx, `DELETE FROM data.traffic_accident
			WHERE occurred_at >= make_timestamptz($1, 1, 1, 0, 0, 0, 'Asia/Taipei')
			  AND occurred_at < make_timestamptz($1 + 1, 1, 1, 0, 0, 0, 'Asia/Taipei')`, y); err != nil {
			return res, fmt.Errorf("clear accidents %d: %w", y, err)
		}
	}
	var newest time.Time
	for _, a := range list {
		var county, lon, lat any
		if c, ok := counties.CodeFromAddress(a.Place); ok {
			county = c
		} else if c, ok := counties.CodeFromAddress(a.Police); ok {
			county = c
		}
		if a.Lon != nil {
			lon, lat = *a.Lon, *a.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.traffic_accident (accident_id, occurred_at, county_code, police, place, weather, light,
			                                   road_class, speed_limit, accident_type, cause, vehicle, deaths, injuries, geom)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			        CASE WHEN $15::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($15, $16), 4326) END)
			ON CONFLICT (accident_id) DO NOTHING`,
			a.ID, a.OccurredAt, county, pipeline.NullIfEmpty(a.Police), a.Place, pipeline.NullIfEmpty(a.Weather),
			pipeline.NullIfEmpty(a.Light), pipeline.NullIfEmpty(a.RoadClass), a.SpeedLimit, pipeline.NullIfEmpty(a.Type),
			pipeline.NullIfEmpty(a.Cause), pipeline.NullIfEmpty(a.Vehicle), a.Deaths, a.Injuries, lon, lat)
		if err != nil {
			return res, fmt.Errorf("insert accident %s: %w", a.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		if a.OccurredAt.After(newest) {
			newest = a.OccurredAt
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE data.traffic_accident a SET county_code = c.code
		FROM ref.county c
		WHERE a.county_code IS NULL AND a.geom IS NOT NULL AND ST_Contains(c.boundary, a.geom)`); err != nil {
		return res, fmt.Errorf("locate accidents: %w", err)
	}
	var unknown int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM data.traffic_accident WHERE county_code IS NULL`).Scan(&unknown); err != nil {
		return res, err
	}
	if unknown > 0 {
		res.Warnf("%d 件事故對不到縣市", unknown)
	}
	res.ObserveTime(newest)
	return res, tx.Commit()
}
