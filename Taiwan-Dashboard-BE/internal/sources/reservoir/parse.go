package reservoir

import (
	"fmt"
	"math"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type Observation struct {
	ReservoirID  string
	ObservedAt   time.Time
	WaterLevel   *float64 // m
	Storage      *float64 // effective storage, 萬立方公尺
	Inflow       *float64 // m³/s
	TotalOutflow *float64 // m³/s
	Rainfall     *float64 // mm
	StatusType   *int16
}

func (o Observation) HasData() bool {
	return o.WaterLevel != nil || o.Storage != nil || o.Inflow != nil || o.TotalOutflow != nil
}

type DailyRecord struct {
	ReservoirID string
	Name        string
	Capacity    *float64 // effective capacity, 萬立方公尺
	Date        time.Time
}

type BasicRecord struct {
	ReservoirID string
	Name        string
	Township    string   // 鄉鎮市區名稱, e.g. 基隆巿安樂區
	Capacity    *float64 // 目前有效容量, 萬立方公尺
}

var timeLayouts = []string{"2006-01-02T15:04:05", "20060102T150405", "2006-01-02T15:04", "2006-01-02"}

func parseTime(s string) (time.Time, error) {
	return pipeline.ParseTaipeiTime(strings.ToUpper(strings.TrimSpace(s)), timeLayouts...)
}

func ParseRealtime(body []byte) ([]Observation, []string, error) {
	recs, err := pipeline.DecodeWRAList(body)
	if err != nil {
		return nil, nil, err
	}
	var out []Observation
	var warnings []string
	for i, r := range recs {
		o, err := parseObservation(r)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（水庫 %s）：%v", i+1, r["reservoiridentifier"], err))
			continue
		}
		out = append(out, o)
	}
	return out, warnings, nil
}

func parseObservation(r map[string]string) (Observation, error) {
	o := Observation{ReservoirID: strings.TrimSpace(r["reservoiridentifier"])}
	if o.ReservoirID == "" {
		return o, fmt.Errorf("缺少水庫代碼")
	}
	var err error
	if o.ObservedAt, err = parseTime(r["observationtime"]); err != nil {
		return o, fmt.Errorf("觀測時間：%w", err)
	}
	fields := []struct {
		name string
		dst  **float64
	}{
		{"waterlevel", &o.WaterLevel},
		{"effectivewaterstoragecapacity", &o.Storage},
		{"inflowdischarge", &o.Inflow},
		{"totaloutflow", &o.TotalOutflow},
		{"accumulaterainfallincatchment", &o.Rainfall},
	}
	for _, f := range fields {
		if *f.dst, err = pipeline.ParseNumber(r[f.name]); err != nil {
			return o, fmt.Errorf("%s：%w", f.name, err)
		}
	}
	st, err := pipeline.ParseNumber(r["statustype"])
	if err != nil {
		return o, fmt.Errorf("statustype：%w", err)
	}
	if st != nil {
		v := int16(math.Round(*st))
		o.StatusType = &v
	}
	return o, nil
}

func ParseDaily(body []byte) ([]DailyRecord, []string, error) {
	recs, err := pipeline.DecodeWRAList(body)
	if err != nil {
		return nil, nil, err
	}
	var out []DailyRecord
	var warnings []string
	for _, r := range recs {
		d := DailyRecord{
			ReservoirID: strings.TrimSpace(r["reservoiridentifier"]),
			Name:        strings.TrimSpace(r["reservoirname"]),
		}
		if d.ReservoirID == "" {
			continue
		}
		if d.Capacity, err = pipeline.ParseNumber(r["capacity"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("每日營運（水庫 %s）有效容量：%v", d.ReservoirID, err))
		}
		if d.Date, err = parseTime(r["datetime"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("每日營運（水庫 %s）統計時間：%v", d.ReservoirID, err))
		}
		out = append(out, d)
	}
	return out, warnings, nil
}

func ParseBasic(body []byte) ([]BasicRecord, []string, error) {
	recs, err := pipeline.DecodeWRAList(body)
	if err != nil {
		return nil, nil, err
	}
	var out []BasicRecord
	var warnings []string
	for _, r := range recs {
		b := BasicRecord{
			ReservoirID: strings.TrimSpace(r["水庫代碼"]),
			Name:        strings.TrimSpace(r["水庫名稱"]),
			Township:    r["鄉鎮市區名稱"],
		}
		if b.ReservoirID == "" {
			continue
		}
		if b.Capacity, err = pipeline.ParseNumber(r["目前有效容量"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("基本資料（水庫 %s）目前有效容量：%v", b.ReservoirID, err))
		}
		out = append(out, b)
	}
	return out, warnings, nil
}
