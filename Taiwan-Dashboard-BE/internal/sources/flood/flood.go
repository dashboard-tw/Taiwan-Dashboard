package flood

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

var DefaultURL = "https://sta.colife.org.tw/STA_WaterResource_v2/v1.0/Datastreams?" + url.Values{
	"$filter": {"substringof('FL',Thing/properties/stationCode)"},
	"$select": {"name,observedArea"},
	"$expand": {"Thing($select=properties),Observations($top=1;$orderby=phenomenonTime desc;$select=phenomenonTime,result)"},
	"$top":    {"1000"},
}.Encode()

const FloodedCM = 10

type Reading struct {
	Code, Name, Authority string
	Lon, Lat              *float64
	Depth                 *float64
	Observed              time.Time
}

func ParsePage(body []byte) ([]Reading, string, error) {
	var page struct {
		Next  string `json:"@iot.nextLink"`
		Value []struct {
			Name string `json:"name"`
			Area *struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"observedArea"`
			Thing struct {
				Properties struct {
					Code      string `json:"stationCode"`
					Name      string `json:"stationName"`
					Authority string `json:"authority"`
				} `json:"properties"`
			} `json:"Thing"`
			Observations []struct {
				Time   string          `json:"phenomenonTime"`
				Result json.RawMessage `json:"result"`
			} `json:"Observations"`
		} `json:"value"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &page); err != nil {
		return nil, "", fmt.Errorf("decode flood sensors: %w", err)
	}
	var out []Reading
	for _, v := range page.Value {
		pr := v.Thing.Properties
		r := Reading{Code: strings.TrimSpace(pr.Code), Name: strings.TrimSpace(pr.Name), Authority: strings.TrimSpace(pr.Authority)}
		if strings.TrimSpace(v.Name) != "淹水深度" || r.Code == "" || r.Name == "" || len(r.Code) > 40 {
			continue
		}
		if a := v.Area; a != nil && len(a.Coordinates) == 2 {
			lon, lat := a.Coordinates[0], a.Coordinates[1]
			if lat > 21 && lat < 27 && lon > 118 && lon < 123 {
				r.Lon, r.Lat = &lon, &lat
			}
		}
		if len(v.Observations) > 0 {
			o := v.Observations[0]
			var d float64
			if err := json.Unmarshal(o.Result, &d); err == nil {
				if t, err := time.Parse(time.RFC3339, strings.TrimSpace(o.Time)); err == nil {
					r.Depth, r.Observed = &d, t.In(pipeline.Taipei)
				}
			}
		}
		out = append(out, r)
	}
	return out, page.Next, nil
}

type Pipeline struct {
	URL      string
	MaxPages int // zero is 10
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "flood_sensor",
		Name:         "淹水感測器即時淹水深度",
		SourceAgency: "經濟部水利署（民生公共物聯網）",
		SourceURL:    "https://colife.org.tw/",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	maxPages := p.MaxPages
	if maxPages == 0 {
		maxPages = 10
	}
	var all []Reading
	next := p.URL
	for i := 0; next != "" && i < maxPages; i++ {
		body, err := env.Fetch.Get(ctx, next, next)
		if err != nil {
			return res, err
		}
		list, n, err := ParsePage(body)
		if err != nil {
			return res, err
		}
		all = append(all, list...)
		next = n
	}
	res.RowsFetched = len(all)
	if len(all) == 0 {
		return res, errors.New("淹水感測器是空的")
	}
	now := env.Now()
	seen := map[string]bool{}
	var sensors, current, flooded [][]any
	var latest time.Time
	odd := 0
	for _, r := range all {
		if seen[r.Code] {
			continue
		}
		seen[r.Code] = true
		var point any
		if r.Lon != nil {
			point = fmt.Sprintf("POINT(%f %f)", *r.Lon, *r.Lat)
		}
		sensors = append(sensors, []any{r.Code, r.Name, pipeline.NullIfEmpty(r.Authority), point})
		if r.Depth == nil {
			continue
		}
		if *r.Depth < 0 || *r.Depth > 300 || r.Observed.After(now.Add(10*time.Minute)) || r.Observed.Before(now.Add(-2*time.Hour)) {
			odd++
			continue
		}
		if r.Observed.After(latest) {
			latest = r.Observed
		}
		current = append(current, []any{r.Code, *r.Depth, r.Observed})
		if *r.Depth >= FloodedCM {
			flooded = append(flooded, []any{r.Code, r.Observed, *r.Depth})
		}
	}
	if len(current) == 0 {
		return res, errors.New("沒有兩小時內的淹水深度")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := pipeline.InsertRows(ctx, tx, `INSERT INTO ref.flood_sensor (station_code, name, authority, geom)
		SELECT v.code, v.name, v.authority, ST_GeomFromText(v.point, 4326) FROM (VALUES`, sensors,
		`) AS v (code, name, authority, point)
		ON CONFLICT (station_code) DO UPDATE SET name = EXCLUDED.name, authority = EXCLUDED.authority,
			geom = EXCLUDED.geom, last_seen_at = now()`); err != nil {
		return res, fmt.Errorf("upsert flood sensors: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ref.flood_sensor s SET county_code = c.code
		FROM ref.county c
		WHERE s.geom IS NOT NULL AND s.county_code IS DISTINCT FROM c.code AND ST_Contains(c.boundary, s.geom)`); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ref.flood_sensor s SET county_code = (
			SELECT c.code FROM ref.county c WHERE ST_DWithin(c.boundary, s.geom, 0.02)
			ORDER BY ST_Distance(c.boundary, s.geom) LIMIT 1)
		WHERE s.geom IS NOT NULL AND s.county_code IS NULL`); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.flood_current`); err != nil {
		return res, err
	}
	if res.RowsInserted, err = pipeline.InsertRows(ctx, tx,
		`INSERT INTO data.flood_current (station_code, depth_cm, observed_at) VALUES`, current, ""); err != nil {
		return res, fmt.Errorf("insert flood depths: %w", err)
	}
	if _, err := pipeline.InsertRows(ctx, tx, `INSERT INTO data.flood_obs (station_code, observed_at, depth_cm) VALUES`,
		flooded, `ON CONFLICT (station_code, observed_at) DO NOTHING`); err != nil {
		return res, fmt.Errorf("insert flooded readings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.flood_obs WHERE observed_at < $1`, now.AddDate(-1, 0, 0)); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data.flood_county_history (observed_at, county_code, sensors, flooded, max_depth_cm)
		SELECT $1, s.county_code, count(*), count(*) FILTER (WHERE f.depth_cm >= $2), max(f.depth_cm)
		FROM data.flood_current f JOIN ref.flood_sensor s USING (station_code)
		WHERE s.county_code IS NOT NULL
		GROUP BY s.county_code
		ON CONFLICT (observed_at, county_code) DO NOTHING`, now.Truncate(time.Minute), FloodedCM); err != nil {
		return res, err
	}
	if odd > len(seen)/2 {
		res.Warnf("%d 站的淹水深度異常或超過兩小時沒有回報", odd)
	}
	res.ObserveTime(latest)
	return res, tx.Commit()
}
