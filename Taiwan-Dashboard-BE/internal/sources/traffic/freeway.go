package traffic

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	base                   = "https://tisvcloud.freeway.gov.tw/history/motc20/"
	DefaultSectionURL      = base + "Section.xml"
	DefaultSectionShapeURL = base + "SectionShape.xml"
	DefaultLiveURL         = base + "LiveTraffic.xml"
)

type Section struct {
	ID         string
	Name       string
	Road       string
	Direction  string
	Start, End string
	LengthKm   *float64
	SpeedLimit *int
	WKT        string // LINESTRING(lon lat, …)
}

func ParseSections(body []byte) ([]Section, error) {
	var doc struct {
		Sections []struct {
			ID         string `xml:"SectionID"`
			Name       string `xml:"SectionName"`
			RoadName   string `xml:"RoadName"`
			Direction  string `xml:"RoadDirection"`
			Start      string `xml:"RoadSection>Start"`
			End        string `xml:"RoadSection>End"`
			Length     string `xml:"SectionLength"`
			SpeedLimit string `xml:"SpeedLimit"`
		} `xml:"Sections>Section"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode sections: %w", err)
	}
	var out []Section
	for _, s := range doc.Sections {
		x := Section{ID: strings.TrimSpace(s.ID), Name: strings.TrimSpace(s.Name), Road: strings.TrimSpace(s.RoadName),
			Direction: strings.TrimSpace(s.Direction), Start: strings.TrimSpace(s.Start), End: strings.TrimSpace(s.End)}
		if x.ID == "" || x.Name == "" || len(x.ID) > 20 {
			continue
		}
		if v, err := pipeline.ParseNumber(s.Length); err == nil {
			x.LengthKm = v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(s.SpeedLimit)); err == nil && v > 0 && v <= 150 {
			x.SpeedLimit = &v
		}
		out = append(out, x)
	}
	return out, nil
}

func ParseShapes(body []byte) (map[string]string, error) {
	var doc struct {
		Shapes []struct {
			ID       string `xml:"SectionID"`
			Geometry string `xml:"Geometry"`
		} `xml:"SectionShapes>SectionShape"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, fmt.Errorf("decode section shapes: %w", err)
	}
	out := map[string]string{}
	for _, s := range doc.Shapes {
		g := strings.TrimSpace(s.Geometry)
		if strings.HasPrefix(g, "LINESTRING(") && strings.HasSuffix(g, ")") {
			out[strings.TrimSpace(s.ID)] = g
		}
	}
	return out, nil
}

type SectionPipeline struct {
	URL      string
	ShapeURL string
}

func (p *SectionPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "freeway_section",
		Name:         "高速公路路段與線形",
		SourceAgency: "交通部高速公路局",
		SourceURL:    "https://tisvcloud.freeway.gov.tw/",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   72 * time.Hour,
	}
}

func (p *SectionPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	sections, err := ParseSections(body)
	if err != nil {
		return res, err
	}
	shapeBody, err := env.Fetch.Get(ctx, p.ShapeURL, p.ShapeURL)
	if err != nil {
		return res, err
	}
	shapes, err := ParseShapes(shapeBody)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(sections)
	if len(sections) == 0 {
		return res, errors.New("路段資料是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	missing := 0
	for _, s := range sections {
		wkt := shapes[s.ID]
		if wkt == "" {
			missing++
		}
		r, err := tx.ExecContext(ctx, `
			WITH g AS (SELECT CASE WHEN $9 = '' THEN NULL ELSE ST_GeomFromText($9, 4326) END AS geom)
			INSERT INTO ref.freeway_section (section_id, name, road_name, direction, start_name, end_name, length_km,
			                                 speed_limit, county_code, geom)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8,
			       (SELECT c.code FROM ref.county c WHERE g.geom IS NOT NULL
			           AND ST_Contains(c.boundary, ST_LineInterpolatePoint(g.geom, 0.5)) LIMIT 1),
			       g.geom
			  FROM g
			ON CONFLICT (section_id) DO UPDATE SET
				name = EXCLUDED.name, road_name = EXCLUDED.road_name, direction = EXCLUDED.direction,
				start_name = EXCLUDED.start_name, end_name = EXCLUDED.end_name, length_km = EXCLUDED.length_km,
				speed_limit = EXCLUDED.speed_limit, county_code = EXCLUDED.county_code,
				geom = coalesce(EXCLUDED.geom, ref.freeway_section.geom), updated_at = now()
			WHERE (ref.freeway_section.name, ref.freeway_section.length_km, ref.freeway_section.speed_limit,
			       ref.freeway_section.geom) IS DISTINCT FROM
			      (EXCLUDED.name, EXCLUDED.length_km, EXCLUDED.speed_limit, coalesce(EXCLUDED.geom, ref.freeway_section.geom))`,
			s.ID, s.Name, s.Road, pipeline.NullIfEmpty(s.Direction), pipeline.NullIfEmpty(s.Start),
			pipeline.NullIfEmpty(s.End), s.LengthKm, s.SpeedLimit, wkt)
		if err != nil {
			return res, fmt.Errorf("upsert section %s: %w", s.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if missing > 0 {
		res.Warnf("%d 個路段沒有線形", missing)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type Live struct {
	SectionID  string
	SpeedKmh   *int
	TravelTime *int
	Level      *int
	ObservedAt time.Time
}

func ParseLive(body []byte) (time.Time, []Live, error) {
	var doc struct {
		Updated string `xml:"UpdateTime"`
		Items   []struct {
			ID         string `xml:"SectionID"`
			TravelTime string `xml:"TravelTime"`
			Speed      string `xml:"TravelSpeed"`
			Level      string `xml:"CongestionLevel"`
			Collected  string `xml:"DataCollectTime"`
		} `xml:"LiveTraffics>LiveTraffic"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return time.Time{}, nil, fmt.Errorf("decode live traffic: %w", err)
	}
	updated, err := time.Parse(time.RFC3339, strings.TrimSpace(doc.Updated))
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("live traffic update time %q: %w", doc.Updated, err)
	}
	num := func(s string, lo, hi int) *int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v < lo || v > hi {
			return nil
		}
		return &v
	}
	var out []Live
	for _, it := range doc.Items {
		l := Live{SectionID: strings.TrimSpace(it.ID), ObservedAt: updated}
		if l.SectionID == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(it.Collected)); err == nil {
			l.ObservedAt = t
		}
		l.SpeedKmh = num(it.Speed, 0, 200)
		l.TravelTime = num(it.TravelTime, 0, 86400)
		if lv := num(it.Level, -1, 5); lv != nil && *lv != 0 {
			l.Level = lv
		}
		out = append(out, l)
	}
	return updated, out, nil
}

type LivePipeline struct {
	URL string
}

func (p *LivePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "freeway_traffic",
		Name:         "高速公路即時路況（路段旅行速率）",
		SourceAgency: "交通部高速公路局",
		SourceURL:    "https://tisvcloud.freeway.gov.tw/",
		UpdateFreq:   5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	}
}

func (p *LivePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	updated, list, err := ParseLive(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	known := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT section_id FROM ref.freeway_section`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, err
		}
		known[id] = true
	}
	rows.Close()
	if len(known) == 0 {
		return res, errors.New("還沒有路段資料，請先執行 freeway_section")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	slot := updated.Truncate(10 * time.Minute)
	for _, l := range list {
		if !known[l.SectionID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.freeway_traffic_current (section_id, observed_at, speed_kmh, travel_time_s, level)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (section_id) DO UPDATE SET observed_at = EXCLUDED.observed_at, speed_kmh = EXCLUDED.speed_kmh,
				travel_time_s = EXCLUDED.travel_time_s, level = EXCLUDED.level
			WHERE data.freeway_traffic_current.observed_at < EXCLUDED.observed_at`,
			l.SectionID, l.ObservedAt, l.SpeedKmh, l.TravelTime, l.Level); err != nil {
			return res, fmt.Errorf("upsert current traffic %s: %w", l.SectionID, err)
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.freeway_traffic_history (section_id, observed_at, speed_kmh, travel_time_s, level)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (section_id, observed_at) DO NOTHING`,
			l.SectionID, slot, l.SpeedKmh, l.TravelTime, l.Level)
		if err != nil {
			return res, fmt.Errorf("insert traffic history %s: %w", l.SectionID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(updated)
	return res, tx.Commit()
}
