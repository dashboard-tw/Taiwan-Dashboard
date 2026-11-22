package outage

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultPowerURL = "https://service.taipower.com.tw/data/opendata/apply/file/d006012/001.xml"

type PowerOutage struct {
	Info            string
	District        string // 行政區, usually starting with the county name
	Area            string
	Households      *int
	ExpectedRestore string
	UpdatedAt       string // 資料更新時間, as written
}

var powerFields = map[string]bool{
	"停電資訊": true, "行政區": true, "停電區域": true, "影響戶數": true, "預計修復時間": true, "資料更新時間": true,
}

func ParsePowerKML(body []byte) ([]PowerOutage, []string, error) {
	dec := xml.NewDecoder(bytes.NewReader(pipeline.StripBOM(body)))
	var out []PowerOutage
	var warnings []string
	var fields map[string]string // non-nil inside a Placemark
	var dataName string          // inside <Data name="…">
	sawKML := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("decode outage KML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "kml":
				sawKML = true
			case "Placemark":
				fields = map[string]string{}
			case "SimpleData":
				if fields != nil {
					name := attr(t, "name")
					var text string
					if err := dec.DecodeElement(&text, &t); err != nil {
						return nil, nil, fmt.Errorf("decode SimpleData: %w", err)
					}
					if powerFields[name] {
						fields[name] = strings.TrimSpace(text)
					}
				}
			case "Data":
				if fields != nil {
					dataName = attr(t, "name")
				}
			case "value":
				if fields != nil && dataName != "" {
					var text string
					if err := dec.DecodeElement(&text, &t); err != nil {
						return nil, nil, fmt.Errorf("decode Data value: %w", err)
					}
					if powerFields[dataName] {
						fields[dataName] = strings.TrimSpace(text)
					}
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "Data":
				dataName = ""
			case "Placemark":
				o := PowerOutage{
					Info:            fields["停電資訊"],
					District:        fields["行政區"],
					Area:            fields["停電區域"],
					ExpectedRestore: fields["預計修復時間"],
					UpdatedAt:       fields["資料更新時間"],
				}
				if v, err := pipeline.ParseNumber(fields["影響戶數"]); err != nil {
					warnings = append(warnings, fmt.Sprintf("第 %d 筆的影響戶數：%v", len(out)+1, err))
				} else if v != nil && *v >= 0 {
					n := int(*v)
					o.Households = &n
				}
				if o.District == "" && o.Area == "" && o.Households == nil {
					warnings = append(warnings, fmt.Sprintf("第 %d 筆停電資料沒有可用的欄位", len(out)+1))
				} else {
					out = append(out, o)
				}
				fields = nil
			}
		}
	}
	if !sawKML {
		return nil, nil, errors.New("停電資料不是 KML 檔案")
	}
	return out, warnings, nil
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

var updatedLayouts = []string{
	"2006/01/02 15:04:05", "2006/01/02 15:04", "2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02T15:04:05", "2006/1/2 15:04:05", "2006/1/2 15:04",
}

type PowerPipeline struct {
	URL string
}

func (p *PowerPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_outage",
		Name:         "台電停電資訊（災害期間）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/44799",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

func (p *PowerPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	outages, warnings, err := ParsePowerKML(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(outages)

	observed := env.Now()
	for _, o := range outages {
		if o.UpdatedAt == "" {
			continue
		}
		if t, err := pipeline.ParseTaipeiTime(o.UpdatedAt, updatedLayouts...); err == nil {
			if t.Before(observed) || t.Equal(observed) {
				observed = t
			}
		} else {
			res.Warnf("資料更新時間「%s」無法解析，改用抓取時間", o.UpdatedAt)
		}
		break
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM data.power_outage_current`); err != nil {
		return res, err
	}
	for i, o := range outages {
		county, _ := counties.CodeFromAddress(o.District)
		if county == "" {
			county, _ = counties.CodeFromAddress(o.Area)
		}
		args := []any{observed, i + 1, pipeline.NullIfEmpty(county), pipeline.NullIfEmpty(o.District),
			pipeline.NullIfEmpty(o.Area), pipeline.NullIfEmpty(o.Info), o.Households, pipeline.NullIfEmpty(o.ExpectedRestore)}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_outage_history
				(observed_at, seq, county_code, district, area, info, households, expected_restore)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (observed_at, seq) DO NOTHING`, args...)
		if err != nil {
			return res, fmt.Errorf("insert outage %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_outage_current
				(observed_at, seq, county_code, district, area, info, households, expected_restore)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, args...); err != nil {
			return res, fmt.Errorf("insert current outage %d: %w", i+1, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
