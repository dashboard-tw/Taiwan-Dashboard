package resources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultShelterURL       = "https://portal2.emic.gov.tw/Pub/EEA2/OpenData/Shelter.xml"
	DefaultShelterReportURL = "https://portal2.emic.gov.tw/Pub/ERA2/OpenData/ERA2_D3.json"
)

type Shelter struct {
	ID, Code, Name, County, Town, Address, DisasterTypes string
	Indoor, Outdoor, ForWeaker                           *bool
	Open                                                 bool
	Lon, Lat                                             *float64
}

func yesNo(s string) *bool {
	switch strings.TrimSpace(s) {
	case "是":
		v := true
		return &v
	case "否":
		v := false
		return &v
	}
	return nil
}

func ParseShelters(body []byte) ([]Shelter, error) {
	var doc struct {
		Items []struct {
			ID         string `xml:"shelterId"`
			Code       string `xml:"shelterCode"`
			Name       string `xml:"name"`
			County     string `xml:"county"`
			Town       string `xml:"town"`
			Address    string `xml:"address"`
			Types      string `xml:"disastertype"`
			Indoor     string `xml:"isIndoor"`
			Outdoor    string `xml:"isOutdoor"`
			Weaker     string `xml:"adaptForWeaker"`
			OpenStatus string `xml:"openstatus"`
			Lat        string `xml:"lat"`
			Lon        string `xml:"lon"`
		} `xml:"ShletersInfo>ShletersInfo"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode shelters: %w", err)
	}
	byID := map[string]*Shelter{}
	var order []string
	for _, it := range doc.Items {
		id := strings.TrimSpace(it.ID)
		if id == "" || len(id) > 20 || strings.TrimSpace(it.Name) == "" {
			continue
		}
		s, ok := byID[id]
		if !ok {
			s = &Shelter{ID: id, Code: strings.TrimSpace(it.Code), Name: strings.TrimSpace(it.Name),
				County: strings.TrimSpace(it.County), Town: strings.TrimSpace(it.Town), Address: strings.TrimSpace(it.Address),
				DisasterTypes: strings.TrimSpace(it.Types), Indoor: yesNo(it.Indoor), Outdoor: yesNo(it.Outdoor),
				ForWeaker: yesNo(it.Weaker)}
			lat, err1 := pipeline.ParseNumber(it.Lat)
			lon, err2 := pipeline.ParseNumber(it.Lon)
			if err1 == nil && err2 == nil && lat != nil && lon != nil && *lat > 10 && *lat < 27 && *lon > 114 && *lon < 123 {
				s.Lat, s.Lon = lat, lon
			}
			byID[id] = s
			order = append(order, id)
		}
		if strings.TrimSpace(it.OpenStatus) == "開啟" {
			s.Open = true
		}
	}
	out := make([]Shelter, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

type ShelterPipeline struct {
	URL string
}

func (p *ShelterPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "shelter",
		Name:         "避難收容處所與開設狀態",
		SourceAgency: "內政部消防署",
		SourceURL:    "https://data.gov.tw/dataset/12849",
		UpdateFreq:   30 * time.Minute,
		StaleAfter:   6 * time.Hour,
	}
}

func (p *ShelterPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseShelters(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("避難收容處所是空的")
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
		if c, ok := counties.Code(s.County); ok {
			county = c
		}
		if s.Lon != nil {
			lon, lat = *s.Lon, *s.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.shelter (shelter_id, code, name, county_code, town, address, disaster_types, indoor, outdoor,
			                         for_weaker, is_open, geom)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			        CASE WHEN $12::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($12, $13), 4326) END)
			ON CONFLICT (shelter_id) DO UPDATE SET
				code = EXCLUDED.code, name = EXCLUDED.name, county_code = EXCLUDED.county_code, town = EXCLUDED.town,
				address = EXCLUDED.address, disaster_types = EXCLUDED.disaster_types, indoor = EXCLUDED.indoor,
				outdoor = EXCLUDED.outdoor, for_weaker = EXCLUDED.for_weaker, is_open = EXCLUDED.is_open,
				geom = EXCLUDED.geom, last_seen_at = now()`,
			s.ID, pipeline.NullIfEmpty(s.Code), s.Name, county, pipeline.NullIfEmpty(s.Town), pipeline.NullIfEmpty(s.Address),
			pipeline.NullIfEmpty(s.DisasterTypes), s.Indoor, s.Outdoor, s.ForWeaker, s.Open, lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert shelter %s: %w", s.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type ShelterReport struct {
	ShelterID, County, Town, Name, Project string
	OpenedAt                               time.Time
	ClosedAt, ReportedAt                   *time.Time
	Refugees, RefugeesTotal, Capacity      *int
}

func parseEMICTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.NewReplacer("上午", "AM", "下午", "PM").Replace(s)
	for _, layout := range []string{"2006/1/2 PM 03:04:05", "2006/1/2 15:04:05", "2006/1/2 PM 3:04:05"} {
		if t, err := time.ParseInLocation(layout, s, pipeline.Taipei); err == nil {
			return &t
		}
	}
	return nil
}

func ParseShelterReports(body []byte) ([]ShelterReport, []string, error) {
	var doc struct {
		Main struct {
			Project string `json:"prj_no"`
			RptTime string `json:"rpt_time"`
		} `json:"main"`
		Detail []struct {
			City     string `json:"city_name"`
			Town     string `json:"town_name"`
			ID       string `json:"shelter_id"`
			Name     string `json:"shelter"`
			Open     string `json:"open_datetime"`
			Close    string `json:"close_datetime"`
			Refugee  string `json:"refugee"`
			Total    string `json:"refugee_total"`
			Capacity string `json:"people_no"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, nil, fmt.Errorf("decode shelter reports: %w", err)
	}
	var reported *time.Time
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(doc.Main.RptTime)); err == nil {
		reported = &t
	}
	num := func(s string) *int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v < 0 || v > 1_000_000 {
			return nil
		}
		return &v
	}
	var out []ShelterReport
	var warnings []string
	for _, d := range doc.Detail {
		opened := parseEMICTime(d.Open)
		if strings.TrimSpace(d.Name) == "" || opened == nil {
			warnings = append(warnings, fmt.Sprintf("收容所通報「%s」缺少名稱或開設時間", d.Name))
			continue
		}
		id := strings.TrimSpace(d.ID)
		if id == "" {
			id = "adhoc:" + strings.TrimSpace(d.City) + strings.TrimSpace(d.Town) + strings.TrimSpace(d.Name)
		}
		out = append(out, ShelterReport{ShelterID: id, County: strings.TrimSpace(d.City),
			Town: strings.TrimSpace(d.Town), Name: strings.TrimSpace(d.Name), Project: strings.TrimSpace(doc.Main.Project),
			OpenedAt: *opened, ClosedAt: parseEMICTime(d.Close), ReportedAt: reported,
			Refugees: num(d.Refugee), RefugeesTotal: num(d.Total), Capacity: num(d.Capacity)})
	}
	return out, warnings, nil
}

type ShelterReportPipeline struct {
	URL string
}

func (p *ShelterReportPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "shelter_report",
		Name:         "收容所開設通報（災害期間）",
		SourceAgency: "內政部消防署（各直轄市、縣市政府通報）",
		SourceURL:    "https://data.gov.tw/dataset/77133",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *ShelterReportPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	var he *pipeline.HTTPError
	if errors.As(err, &he) && he.Status == 404 {
		res.Warnings = []string{"來源目前沒有通報檔，通常表示沒有災害"}
		res.ObserveTime(env.Now())
		return res, nil
	}
	if err != nil {
		return res, err
	}
	list, warnings, err := ParseShelterReports(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(list)
	res.ObserveTime(env.Now())
	if len(list) == 0 {
		return res, nil // 沒有災害時是空的
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
		var county any
		if c, ok := counties.Code(s.County); ok {
			county = c
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.shelter_report (shelter_id, opened_at, closed_at, county_code, town, name, refugees,
			                                 refugees_total, capacity, project, reported_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (shelter_id, opened_at) DO UPDATE SET
				closed_at = EXCLUDED.closed_at, refugees = EXCLUDED.refugees, refugees_total = EXCLUDED.refugees_total,
				capacity = EXCLUDED.capacity, reported_at = EXCLUDED.reported_at, last_seen_at = now()`,
			s.ShelterID, s.OpenedAt, s.ClosedAt, county, pipeline.NullIfEmpty(s.Town), s.Name, s.Refugees,
			s.RefugeesTotal, s.Capacity, pipeline.NullIfEmpty(s.Project), s.ReportedAt)
		if err != nil {
			return res, fmt.Errorf("upsert shelter report %s: %w", s.ShelterID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	return res, tx.Commit()
}
