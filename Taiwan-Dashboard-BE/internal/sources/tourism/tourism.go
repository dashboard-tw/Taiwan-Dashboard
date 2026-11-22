package tourism

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	base                 = "https://media.taiwan.net.tw/XMLReleaseAll_public/v2.0/Zh_tw/"
	DefaultAttractionURL = base + "Attraction-json.zip"
	DefaultHotelURL      = base + "Hotel-json.zip"
)

const maxJSON = 200 << 20

func unzip(body []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".json") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, maxJSON+1))
		if err != nil {
			return nil, fmt.Errorf("unzip %s: %w", f.Name, err)
		}
		if len(b) > maxJSON {
			return nil, fmt.Errorf("%s 解壓縮後超過 %d bytes", f.Name, maxJSON)
		}
		return pipeline.StripBOM(b), nil
	}
	return nil, errors.New("zip 裡沒有 JSON")
}

type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" {
		s = ""
	}
	*f = flex(strings.TrimSpace(s))
	return nil
}

func (f flex) float() *float64 {
	v, err := strconv.ParseFloat(string(f), 64)
	if err != nil {
		return nil
	}
	return &v
}

func (f flex) int(max int) *int {
	v, err := strconv.Atoi(string(f))
	if err != nil || v < 0 || v > max {
		return nil
	}
	return &v
}

type postal struct {
	City     string `json:"City"`
	CityCode string `json:"CityCode"`
	Town     string `json:"Town"`
	Street   string `json:"StreetAddress"`
}

func (p postal) address() string {
	if p.Street == "" {
		return ""
	}
	if strings.HasPrefix(p.Street, p.City) {
		return p.Street
	}
	return p.City + p.Town + p.Street
}

func point(lat, lon flex) (*float64, *float64) {
	la, lo := lat.float(), lon.float()
	if la == nil || lo == nil || *la < 21 || *la > 27 || *lo < 118 || *lo > 123 {
		return nil, nil
	}
	return lo, la
}

type Attraction struct {
	ID, Name, County, Town, Address, Description string
	Classes                                      []int
	Free                                         *bool
	Status                                       *int
	Lon, Lat                                     *float64
}

func ParseAttractions(body []byte) ([]Attraction, error) {
	js, err := unzip(body)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Attractions []struct {
			ID          string `json:"AttractionID"`
			Name        string `json:"AttractionName"`
			Description string `json:"Description"`
			Lat         flex   `json:"PositionLat"`
			Lon         flex   `json:"PositionLon"`
			Classes     []int  `json:"AttractionClasses"`
			Postal      postal `json:"PostalAddress"`
			Free        flex   `json:"IsAccessibleForFree"`
			Status      flex   `json:"ServiceStatus"`
		} `json:"Attractions"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("decode attractions: %w", err)
	}
	var out []Attraction
	for _, a := range doc.Attractions {
		x := Attraction{ID: strings.TrimSpace(a.ID), Name: strings.TrimSpace(a.Name), County: a.Postal.CityCode,
			Town: a.Postal.Town, Address: a.Postal.address(), Description: pipeline.Truncate(strings.TrimSpace(a.Description), 200),
			Classes: a.Classes, Status: a.Status.int(99)}
		if x.ID == "" || x.Name == "" || len(x.ID) > 60 {
			continue
		}
		if f := a.Free.int(1); f != nil {
			b := *f == 1
			x.Free = &b
		}
		x.Lon, x.Lat = point(a.Lat, a.Lon)
		out = append(out, x)
	}
	return out, nil
}

type Hotel struct {
	ID, Name, County, Town, Address string
	Class, Stars                    *int
	Rooms, Capacity                 *int
	LowestPrice, HighestPrice       *int
	Lon, Lat                        *float64
}

func ParseHotels(body []byte) ([]Hotel, error) {
	js, err := unzip(body)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Hotels []struct {
			ID       string `json:"HotelID"`
			Name     string `json:"HotelName"`
			Lat      flex   `json:"PositionLat"`
			Lon      flex   `json:"PositionLon"`
			Classes  []int  `json:"HotelClasses"`
			Stars    flex   `json:"HotelStars"`
			Rooms    flex   `json:"TotalRooms"`
			Capacity flex   `json:"TotalCapacity"`
			Lowest   flex   `json:"LowestPrice"`
			Highest  flex   `json:"CeilingPrice"`
			Postal   postal `json:"PostalAddress"`
		} `json:"Hotels"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("decode hotels: %w", err)
	}
	var out []Hotel
	for _, h := range doc.Hotels {
		x := Hotel{ID: strings.TrimSpace(h.ID), Name: strings.TrimSpace(h.Name), County: h.Postal.CityCode,
			Town: h.Postal.Town, Address: h.Postal.address(), Stars: h.Stars.int(5), Rooms: h.Rooms.int(10000),
			Capacity: h.Capacity.int(50000), LowestPrice: h.Lowest.int(10000000), HighestPrice: h.Highest.int(10000000)}
		if x.ID == "" || x.Name == "" || len(x.ID) > 60 {
			continue
		}
		if len(h.Classes) > 0 && h.Classes[0] >= 1 && h.Classes[0] <= 4 {
			c := h.Classes[0]
			x.Class = &c
		}
		if x.Stars != nil && *x.Stars == 0 {
			x.Stars = nil
		}
		x.Lon, x.Lat = point(h.Lat, h.Lon)
		out = append(out, x)
	}
	return out, nil
}

func fetchCounties(ctx context.Context, env pipeline.Env) (map[string]bool, error) {
	rows, err := env.DB.QueryContext(ctx, `SELECT code FROM ref.county`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out[c] = true
	}
	return out, rows.Err()
}

func dataset(id, name, page string) pipeline.Dataset {
	return pipeline.Dataset{
		ID:           id,
		Name:         name,
		SourceAgency: "交通部觀光署",
		SourceURL:    page,
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   7 * 24 * time.Hour,
	}
}

func pointArgs(lon, lat *float64) (any, any) {
	if lon == nil {
		return nil, nil
	}
	return *lon, *lat
}

type AttractionPipeline struct {
	URL string
}

func (p *AttractionPipeline) Dataset() pipeline.Dataset {
	return dataset("tourism_attraction", "觀光景點", "https://data.gov.tw/dataset/7777")
}

func (p *AttractionPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseAttractions(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("景點是空的")
	}
	known, err := fetchCounties(ctx, env)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	unknown := 0
	for _, a := range list {
		var county any
		if known[a.County] {
			county = a.County
		} else {
			unknown++
		}
		classes := a.Classes
		if classes == nil {
			classes = []int{}
		}
		lon, lat := pointArgs(a.Lon, a.Lat)
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.tourism_attraction (attraction_id, name, classes, county_code, town, address, description,
			                                    free, status, geom)
			VALUES ($1, $2, $3::int[], $4, $5, $6, $7, $8, $9,
			        CASE WHEN $10::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($10, $11), 4326) END)
			ON CONFLICT (attraction_id) DO UPDATE SET
				name = EXCLUDED.name, classes = EXCLUDED.classes, county_code = EXCLUDED.county_code, town = EXCLUDED.town,
				address = EXCLUDED.address, description = EXCLUDED.description, free = EXCLUDED.free,
				status = EXCLUDED.status, geom = EXCLUDED.geom, last_seen_at = now()`,
			a.ID, a.Name, intArray(classes), county, pipeline.NullIfEmpty(a.Town), pipeline.NullIfEmpty(a.Address),
			pipeline.NullIfEmpty(a.Description), a.Free, a.Status, lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert attraction %s: %w", a.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if unknown > 0 {
		res.Warnf("%d 個景點對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

func intArray(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type HotelPipeline struct {
	URL string
}

func (p *HotelPipeline) Dataset() pipeline.Dataset {
	return dataset("tourism_hotel", "旅館民宿", "https://data.gov.tw/dataset/7780")
}

func (p *HotelPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseHotels(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("旅宿是空的")
	}
	known, err := fetchCounties(ctx, env)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	unknown := 0
	for _, h := range list {
		var county any
		if known[h.County] {
			county = h.County
		} else {
			unknown++
		}
		lon, lat := pointArgs(h.Lon, h.Lat)
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.tourism_hotel (hotel_id, name, class, stars, rooms, capacity, lowest_price, highest_price,
			                               county_code, town, address, geom)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			        CASE WHEN $12::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($12, $13), 4326) END)
			ON CONFLICT (hotel_id) DO UPDATE SET
				name = EXCLUDED.name, class = EXCLUDED.class, stars = EXCLUDED.stars, rooms = EXCLUDED.rooms,
				capacity = EXCLUDED.capacity, lowest_price = EXCLUDED.lowest_price, highest_price = EXCLUDED.highest_price,
				county_code = EXCLUDED.county_code, town = EXCLUDED.town, address = EXCLUDED.address, geom = EXCLUDED.geom,
				last_seen_at = now()`,
			h.ID, h.Name, h.Class, h.Stars, h.Rooms, h.Capacity, h.LowestPrice, h.HighestPrice, county,
			pipeline.NullIfEmpty(h.Town), pipeline.NullIfEmpty(h.Address), lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert hotel %s: %w", h.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if unknown > 0 {
		res.Warnf("%d 間旅宿對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
