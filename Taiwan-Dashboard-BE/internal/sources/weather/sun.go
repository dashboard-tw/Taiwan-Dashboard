package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const (
	DefaultSunURL  = cwa.Datastore + "A-B0062-001"
	DefaultMoonURL = cwa.Datastore + "A-B0063-001"
)

// SunMoonDays is how many days ahead are fetched.
const SunMoonDays = 30

// SunMoonDay holds one county's times for one day. Keys are column names of data.sun_moon.
type SunMoonDay struct {
	County string
	Date   time.Time
	Values map[string]any
}

// columns of data.sun_moon and the source field of each
var sunFields = [][2]string{
	{"civil_dawn", "BeginCivilTwilightTime"}, {"sunrise", "SunRiseTime"}, {"sunrise_az", "SunRiseAZ"},
	{"sun_transit", "SunTransitTime"}, {"sun_transit_alt", "SunTransitAlt"}, {"sunset", "SunSetTime"},
	{"sunset_az", "SunSetAZ"}, {"civil_dusk", "EndCivilTwilightTime"},
}

var moonFields = [][2]string{
	{"moonrise", "MoonRiseTime"}, {"moonrise_az", "MoonRiseAZ"}, {"moon_transit", "MoonTransitTime"},
	{"moon_transit_alt", "MoonTransitAlt"}, {"moonset", "MoonSetTime"}, {"moonset_az", "MoonSetAZ"},
}

func sunValue(col, raw string) any {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "" || raw == "-" || raw == "--":
		return nil
	case strings.HasSuffix(col, "_az"):
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 && v <= 360 {
			return strconv.Itoa(v)
		}
		return nil
	case strings.HasSuffix(col, "_alt"):
		return raw
	default:
		if _, err := time.Parse("15:04", raw); err == nil {
			return raw
		}
		return nil
	}
}

// ParseSunMoon reads A-B0062-001 or A-B0063-001.
func ParseSunMoon(body []byte, moon bool) ([]SunMoonDay, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Locations struct {
				Location []struct {
					County string              `json:"CountyName"`
					Time   []map[string]string `json:"time"`
				} `json:"location"`
			} `json:"locations"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, fmt.Errorf("decode sun/moon times: %w", err)
	}
	if file.Success != "true" {
		return nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	fields := sunFields
	if moon {
		fields = moonFields
	}
	var out []SunMoonDay
	for _, loc := range file.Records.Locations.Location {
		for _, t := range loc.Time {
			date, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(t["Date"]), pipeline.Taipei)
			if err != nil {
				continue
			}
			d := SunMoonDay{County: strings.TrimSpace(loc.County), Date: date, Values: map[string]any{}}
			for _, f := range fields {
				d.Values[f[0]] = sunValue(f[0], t[f[1]])
			}
			out = append(out, d)
		}
	}
	return out, nil
}

type SunMoonPipeline struct {
	SunURL  string
	MoonURL string
	APIKey  string
}

func (p *SunMoonPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "sun_moon",
		Name:         "日出日沒及月出月沒時刻",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/astronomy/A-B0062-001",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   3 * 24 * time.Hour,
	}
}

func (p *SunMoonPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	today := pipeline.TaipeiDate(env.Now())
	q := url.Values{
		"timeFrom": {today.Format(time.DateOnly)},
		"timeTo":   {today.AddDate(0, 0, SunMoonDays).Format(time.DateOnly)},
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

	for i, src := range []struct {
		url    string
		fields [][2]string
	}{{p.SunURL, sunFields}, {p.MoonURL, moonFields}} {
		body, err := cwa.GetWith(ctx, env, src.url, p.APIKey, q)
		if err != nil {
			return res, err
		}
		days, err := ParseSunMoon(body, i == 1)
		if err != nil {
			return res, err
		}
		if len(days) == 0 {
			return res, errors.New("日出日沒或月出月沒資料是空的")
		}
		res.RowsFetched += len(days)
		cols := []string{"county_code", "date"}
		for _, f := range src.fields {
			cols = append(cols, f[0])
		}
		casts := make([]string, len(cols))
		for j, c := range cols {
			switch {
			case c == "county_code" || strings.HasSuffix(c, "_alt"):
				casts[j] = "v." + c
			case c == "date":
				casts[j] = "v.date::date"
			case strings.HasSuffix(c, "_az"):
				casts[j] = "v." + c + "::smallint"
			default:
				casts[j] = "v." + c + "::time"
			}
		}
		var rows [][]any
		unknown := map[string]bool{}
		for _, d := range days {
			code, ok := counties.Code(d.County)
			if !ok {
				if !unknown[d.County] {
					unknown[d.County] = true
					res.Warnf("縣市「%s」對不到縣市代碼", d.County)
				}
				continue
			}
			row := []any{code, d.Date.Format(time.DateOnly)}
			for _, f := range src.fields {
				row = append(row, d.Values[f[0]])
			}
			rows = append(rows, row)
		}
		data := cols[2:]
		old := make([]string, len(data))
		ex := make([]string, len(data))
		for j, c := range data {
			old[j], ex[j] = "data.sun_moon."+c, "EXCLUDED."+c
		}
		n, err := pipeline.InsertRows(ctx, tx,
			"INSERT INTO data.sun_moon ("+strings.Join(cols, ", ")+") SELECT "+strings.Join(casts, ", ")+" FROM (VALUES",
			rows, ") AS v("+strings.Join(cols, ", ")+") ON CONFLICT (county_code, date) DO UPDATE SET "+
				pipeline.SetExcluded(data)+" WHERE ("+strings.Join(old, ", ")+") IS DISTINCT FROM ("+strings.Join(ex, ", ")+")")
		if err != nil {
			return res, fmt.Errorf("upsert sun/moon: %w", err)
		}
		res.RowsInserted += n
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
