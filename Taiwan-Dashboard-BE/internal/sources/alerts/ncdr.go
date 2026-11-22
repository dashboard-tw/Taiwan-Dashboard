package alerts

import (
	"cmp"
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

const DefaultFeedURL = "https://alerts.ncdr.nat.gov.tw/RssAtomFeed.ashx"

const capHost = "https://alerts.ncdr.nat.gov.tw/"

var Kinds = map[string]string{
	"土石流及大規模崩塌": "debris_flow",
	"淹水":        "flood",
	"淹水感測":      "flood_sensor",
	"道路封閉":      "road_closure",
	"水庫放流":      "reservoir_release",
}

const maxNewPerRun = 300

type FeedEntry struct {
	ID       string
	Category string
	Link     string
}

func ParseFeed(body []byte) (time.Time, []FeedEntry, error) {
	var feed struct {
		Updated string `xml:"updated"`
		Entries []struct {
			ID       string `xml:"id"`
			Category struct {
				Term string `xml:"term,attr"`
			} `xml:"category"`
			Links []struct {
				Rel  string `xml:"rel,attr"`
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &feed); err != nil {
		return time.Time{}, nil, fmt.Errorf("decode NCDR feed: %w", err)
	}
	updated, _ := time.Parse(time.RFC3339, strings.TrimSpace(feed.Updated))
	var out []FeedEntry
	for _, e := range feed.Entries {
		f := FeedEntry{ID: strings.TrimSpace(e.ID), Category: strings.TrimSpace(e.Category.Term)}
		for _, l := range e.Links {
			if l.Rel == "alternate" || f.Link == "" {
				f.Link = strings.TrimSpace(l.Href)
			}
		}
		if f.ID != "" && len(f.ID) <= 200 {
			out = append(out, f)
		}
	}
	return updated, out, nil
}

type Area struct {
	Desc    string `json:"desc"`
	Geocode string `json:"geocode,omitempty"`
}

type Alert struct {
	ID          string
	Kind        string
	MsgType     string
	Sender      string
	Sent        time.Time
	Effective   *time.Time
	Expires     *time.Time
	Headline    string
	Description string
	Level       string
	Color       string
	URL         string
	Areas       []Area
	Lat, Lon    *float64
	Supersedes  []string
}

var colors = map[string]bool{"紅色": true, "橙色": true, "黃色": true, "綠色": true}

func ParseCAP(body []byte, kind string) (Alert, error) {
	var doc struct {
		Identifier string `xml:"identifier"`
		Sent       string `xml:"sent"`
		MsgType    string `xml:"msgType"`
		References string `xml:"references"`
		Info       []struct {
			Effective   string `xml:"effective"`
			Expires     string `xml:"expires"`
			SenderName  string `xml:"senderName"`
			Headline    string `xml:"headline"`
			Description string `xml:"description"`
			Web         string `xml:"web"`
			Parameters  []struct {
				Name  string `xml:"valueName"`
				Value string `xml:"value"`
			} `xml:"parameter"`
			Areas []struct {
				Desc     string   `xml:"areaDesc"`
				Circles  []string `xml:"circle"`
				Geocodes []struct {
					Name  string `xml:"valueName"`
					Value string `xml:"value"`
				} `xml:"geocode"`
			} `xml:"area"`
		} `xml:"info"`
	}
	if err := xml.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return Alert{}, fmt.Errorf("decode CAP: %w", err)
	}
	a := Alert{ID: strings.TrimSpace(doc.Identifier), Kind: kind, MsgType: strings.TrimSpace(doc.MsgType)}
	if a.ID == "" || len(doc.Info) == 0 {
		return a, errors.New("CAP 缺少 identifier 或 info")
	}
	switch a.MsgType {
	case "Alert", "Update", "Cancel":
	default:
		return a, fmt.Errorf("不支援的 msgType %q", a.MsgType)
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(doc.Sent))
	if err != nil {
		return a, fmt.Errorf("sent %q: %w", doc.Sent, err)
	}
	a.Sent = t
	for _, ref := range strings.Fields(doc.References) {
		if parts := strings.Split(ref, ","); len(parts) >= 2 && parts[1] != "" {
			a.Supersedes = append(a.Supersedes, parts[1])
		}
	}
	info := doc.Info[0]
	parseTime := func(s string) *time.Time {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
			return &t
		}
		return nil
	}
	a.Effective, a.Expires = parseTime(info.Effective), parseTime(info.Expires)
	a.Sender = strings.TrimSpace(info.SenderName)
	a.Headline = strings.TrimSpace(info.Headline)
	a.Description = strings.TrimSpace(strings.ReplaceAll(info.Description, "​", ""))
	a.URL = strings.TrimSpace(info.Web)
	for _, p := range info.Parameters {
		v := strings.TrimSpace(p.Value)
		switch {
		case p.Name == "severity_level":
			a.Level = v
		case p.Name == "alert_color":
			a.Color = v
		case a.Color == "" && colors[v]:
			a.Color = v
		}
	}
	for _, ar := range info.Areas {
		area := Area{Desc: strings.TrimSpace(ar.Desc)}
		for _, g := range ar.Geocodes {
			if strings.TrimSpace(g.Value) != "" {
				area.Geocode = strings.TrimSpace(g.Value)
				break
			}
		}
		a.Areas = append(a.Areas, area)
		if a.Lat == nil {
			for _, c := range ar.Circles {
				if lat, lon, ok := parseCircle(c); ok {
					a.Lat, a.Lon = &lat, &lon
					break
				}
			}
		}
	}
	if a.Headline == "" {
		a.Headline = a.Sender + " " + a.Kind
	}
	return a, nil
}

func parseCircle(s string) (float64, float64, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, 0, false
	}
	ll := strings.Split(fields[0], ",")
	if len(ll) != 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(ll[0], 64)
	lon, err2 := strconv.ParseFloat(ll[1], 64)
	if err1 != nil || err2 != nil || lat < 10 || lat > 27 || lon < 114 || lon > 123 {
		return 0, 0, false
	}
	return lat, lon, true
}

func countyCodes(areas []Area, known map[string]bool) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, a := range areas {
		if len(a.Geocode) < 5 {
			continue
		}
		c := a.Geocode[:5]
		if known[c] && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

type NCDRPipeline struct {
	URL     string
	CAPHost string
}

func (p *NCDRPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "disaster_alert",
		Name:         "防災示警（土石流、淹水、道路封閉、水庫放流）",
		SourceAgency: "國家災害防救科技中心（彙整農村發展及水土保持署、水利署、公路局等）",
		SourceURL:    "https://alerts.ncdr.nat.gov.tw/",
		UpdateFreq:   5 * time.Minute,
		StaleAfter:   time.Hour,
		License:      "公眾領域（民生示警公開資料平台）",
	}
}

func (p *NCDRPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	_, entries, err := ParseFeed(body)
	if err != nil {
		return res, err
	}
	known := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT code FROM ref.county`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return res, err
		}
		known[c] = true
	}
	rows.Close()

	var wanted []FeedEntry
	var ids []string
	for _, e := range entries {
		if Kinds[e.Category] != "" {
			wanted = append(wanted, e)
			ids = append(ids, e.ID)
		}
	}
	res.RowsFetched = len(wanted)
	have := map[string]bool{}
	rows, err = env.DB.QueryContext(ctx, `
		UPDATE data.disaster_alert SET last_seen_at = now() WHERE alert_id = ANY ($1) RETURNING alert_id`, ids)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, err
		}
		have[id] = true
	}
	rows.Close()

	fetched := 0
	for _, e := range wanted {
		if have[e.ID] {
			continue
		}
		if fetched >= maxNewPerRun {
			res.Warnf("新的示警超過 %d 則，其餘下次再抓", maxNewPerRun)
			break
		}
		if !strings.HasPrefix(e.Link, cmp.Or(p.CAPHost, capHost)) {
			res.Warnf("示警 %s 的連結不在民生示警平台：%s", e.ID, e.Link)
			continue
		}
		fetched++
		capBody, err := env.Fetch.Get(ctx, e.Link, e.Link)
		if err != nil {
			res.Warnf("下載示警 %s 失敗：%v", e.ID, err)
			continue
		}
		a, err := ParseCAP(capBody, Kinds[e.Category])
		if err != nil {
			res.Warnf("示警 %s：%v", e.ID, err)
			continue
		}
		areas, _ := json.Marshal(a.Areas)
		if a.Areas == nil {
			areas = []byte("[]")
		}
		supersedes := a.Supersedes
		if supersedes == nil {
			supersedes = []string{}
		}
		var lon, lat any
		if a.Lat != nil {
			lon, lat = *a.Lon, *a.Lat
		}
		r, err := env.DB.ExecContext(ctx, `
			INSERT INTO data.disaster_alert (alert_id, kind, msg_type, sender, sent_at, effective_at, expires_at, headline,
			    description, level, color, areas, county_codes, geom, url, supersedes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			        CASE WHEN $14::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($14, $15), 4326) END, $16, $17)
			ON CONFLICT (alert_id) DO UPDATE SET last_seen_at = now()`,
			a.ID, a.Kind, a.MsgType, a.Sender, a.Sent, a.Effective, a.Expires, a.Headline,
			pipeline.NullIfEmpty(a.Description), pipeline.NullIfEmpty(a.Level), pipeline.NullIfEmpty(a.Color),
			string(areas), countyCodes(a.Areas, known), lon, lat, pipeline.NullIfEmpty(a.URL), supersedes)
		if err != nil {
			return res, fmt.Errorf("insert alert %s: %w", a.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, nil
}
