package animal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultAdoptURL = "https://data.moa.gov.tw/Service/OpenData/TransService.aspx?UnitId=QcbUEzN6E6DL"
	DefaultStatsURL = "https://data.moa.gov.tw/Service/OpenData/TransService.aspx?UnitId=DyplMIk3U1hf"
)

type Animal struct {
	ID                                    int64
	Kind, Variety, Sex, Age, Body, Colour string
	Sterilized                            *bool
	Shelter, Address                      string
	Opened                                *time.Time
}

func ParseAnimals(body []byte) ([]Animal, error) {
	var rows []struct {
		ID      int64  `json:"animal_id"`
		Kind    string `json:"animal_kind"`
		Variety string `json:"animal_Variety"`
		Sex     string `json:"animal_sex"`
		Age     string `json:"animal_age"`
		Body    string `json:"animal_bodytype"`
		Colour  string `json:"animal_colour"`
		Steril  string `json:"animal_sterilization"`
		Status  string `json:"animal_status"`
		Opened  string `json:"animal_opendate"`
		Shelter string `json:"shelter_name"`
		Address string `json:"shelter_address"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode adoption list: %w", err)
	}
	var out []Animal
	for _, r := range rows {
		a := Animal{ID: r.ID, Kind: strings.TrimSpace(r.Kind), Variety: strings.TrimSpace(r.Variety),
			Sex: strings.TrimSpace(r.Sex), Age: strings.TrimSpace(r.Age), Body: strings.TrimSpace(r.Body),
			Colour: strings.TrimSpace(r.Colour), Shelter: strings.TrimSpace(r.Shelter), Address: strings.TrimSpace(r.Address)}
		if a.ID <= 0 || a.Kind == "" || a.Shelter == "" || strings.TrimSpace(r.Status) != "OPEN" {
			continue
		}
		switch strings.TrimSpace(r.Steril) {
		case "T":
			v := true
			a.Sterilized = &v
		case "F":
			v := false
			a.Sterilized = &v
		}
		if t, err := time.Parse("2006-01-02", strings.TrimSpace(r.Opened)); err == nil {
			a.Opened = &t
		}
		out = append(out, a)
	}
	return out, nil
}

type AdoptPipeline struct {
	URL string
}

func (p *AdoptPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "shelter_animal",
		Name:         "公立動物收容所開放認養動物",
		SourceAgency: "農業部",
		SourceURL:    "https://data.gov.tw/dataset/85903",
		UpdateFreq:   6 * time.Hour,
		StaleAfter:   2 * 24 * time.Hour,
	}
}

func (p *AdoptPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseAnimals(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("認養動物是空的")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	latest := env.Now().AddDate(0, 0, 1)
	seen := map[int64]bool{}
	var rows [][]any
	unknown := 0
	for _, a := range list {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		var county any
		if c, ok := counties.CodeFromAddress(a.Address); ok {
			county = c
		} else if c, ok := counties.CodeFromAddress(a.Shelter); ok {
			county = c
		} else {
			unknown++
		}
		var opened any
		if a.Opened != nil && !a.Opened.After(latest) {
			opened = *a.Opened
		}
		rows = append(rows, []any{a.ID, a.Kind, pipeline.NullIfEmpty(a.Variety), pipeline.NullIfEmpty(a.Sex),
			pipeline.NullIfEmpty(a.Age), pipeline.NullIfEmpty(a.Body), pipeline.NullIfEmpty(a.Colour), a.Sterilized,
			a.Shelter, county, opened})
	}
	if unknown > 0 {
		res.Warnf("%d 隻動物的收容所對不到縣市", unknown)
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.shelter_animal`); err != nil {
		return res, err
	}
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.shelter_animal (animal_id, kind, variety, sex,
		age, body_type, colour, sterilized, shelter, county_code, open_date) VALUES`, rows, ""); err != nil {
		return res, fmt.Errorf("insert shelter animals: %w", err)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type Month struct {
	County                              string
	Month                               time.Time
	Accepted, Adopted, Euthanized, Died int
}

func ParseStats(body []byte) ([]Month, error) {
	var rows []struct {
		Year   json.Number `json:"rpt_year"`
		Month  json.Number `json:"rpt_month"`
		County string      `json:"rpt_country"`
		Accept json.Number `json:"accept_num"`
		Adopt  json.Number `json:"adopt_num"`
		End    json.Number `json:"end_num"`
		Dead   json.Number `json:"dead_num"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &rows); err != nil {
		return nil, fmt.Errorf("decode shelter statistics: %w", err)
	}
	num := func(n json.Number) (int, bool) {
		v, err := strconv.Atoi(strings.TrimSpace(string(n)))
		return v, err == nil && v >= 0
	}
	var out []Month
	for _, r := range rows {
		y, ok1 := num(r.Year)
		m, ok2 := num(r.Month)
		a, ok3 := num(r.Accept)
		ad, ok4 := num(r.Adopt)
		e, ok5 := num(r.End)
		d, ok6 := num(r.Dead)
		if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) || y < 90 || m < 1 || m > 12 {
			continue
		}
		county := strings.TrimSpace(r.County)
		if county == "桃園縣" {
			county = "桃園市"
		}
		out = append(out, Month{County: county, Month: time.Date(y+1911, time.Month(m), 1, 0, 0, 0, 0, time.UTC),
			Accepted: a, Adopted: ad, Euthanized: e, Died: d})
	}
	return out, nil
}

type StatsPipeline struct {
	URL string
}

func (p *StatsPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "shelter_stats",
		Name:         "全國公立動物收容所收容處理情形統計",
		SourceAgency: "農業部",
		SourceURL:    "https://data.gov.tw/dataset/41236",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

func (p *StatsPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseStats(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("收容統計是空的")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	type key struct {
		county string
		month  time.Time
	}
	seen := map[key]bool{}
	var rows [][]any
	var latest time.Time
	unknown := 0
	for _, m := range list {
		code, ok := counties.Code(m.County)
		if !ok {
			unknown++
			continue
		}
		k := key{code, m.Month}
		if seen[k] {
			continue
		}
		seen[k] = true
		if m.Month.After(latest) {
			latest = m.Month
		}
		rows = append(rows, []any{code, m.Month, m.Accepted, m.Adopted, m.Euthanized, m.Died})
	}
	if unknown > 0 {
		res.Warnf("%d 列對不到縣市", unknown)
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.shelter_month (county_code, month, accepted,
		adopted, euthanized, died) VALUES`, rows, `ON CONFLICT (county_code, month) DO UPDATE SET accepted = EXCLUDED.accepted,
		adopted = EXCLUDED.adopted, euthanized = EXCLUDED.euthanized, died = EXCLUDED.died`); err != nil {
		return res, fmt.Errorf("upsert shelter statistics: %w", err)
	}
	if !latest.IsZero() {
		res.ObserveTime(time.Date(latest.Year(), latest.Month(), 1, 0, 0, 0, 0, pipeline.Taipei).AddDate(0, 1, -1))
	}
	return res, tx.Commit()
}
