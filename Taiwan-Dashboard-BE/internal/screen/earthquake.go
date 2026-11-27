package screen

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

type Quake struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"` // significant 顯著有感、local 小區域有感
	No               *int      `json:"no"`
	OriginTime       time.Time `json:"origin_time"`
	Location         *string   `json:"location"`
	Magnitude        *float64  `json:"magnitude"`
	DepthKm          *float64  `json:"depth_km"`
	MaxIntensity     *string   `json:"max_intensity"`
	MaxIntensityRank *int      `json:"max_intensity_rank"`
	Color            *string   `json:"color"`
}

type QuakeIntensity struct {
	CountyCode string `json:"county_code"`
	County     string `json:"county"`
	Intensity  string `json:"intensity"`
	Rank       int    `json:"rank"`
}

type QuakeDetail struct {
	Quake
	IssuedAt    *time.Time       `json:"issued_at"`
	Content     *string          `json:"content"`
	ReportURL   *string          `json:"report_url"`
	ImageURL    *string          `json:"image_url"`
	Lon         *float64         `json:"lon"`
	Lat         *float64         `json:"lat"`
	ShakemapURL *string          `json:"shakemap_url"`
	ReportType  *string          `json:"report_type"`
	Remark      *string          `json:"remark"`
	Intensities []QuakeIntensity `json:"intensities"`
	Stations    []QuakeStation   `json:"stations"`
}

// QuakeStation is one seismograph's reading.
type QuakeStation struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	CountyCode  *string  `json:"county_code"`
	County      *string  `json:"county"`
	Lon         *float64 `json:"lon"`
	Lat         *float64 `json:"lat"`
	Intensity   *string  `json:"intensity"`
	Rank        *int     `json:"rank"`
	PGA         *float64 `json:"pga_gal"`
	PGAEW       *float64 `json:"pga_ew_gal"`
	PGANS       *float64 `json:"pga_ns_gal"`
	PGAV        *float64 `json:"pga_v_gal"`
	PGV         *float64 `json:"pgv_kine"`
	PGVEW       *float64 `json:"pgv_ew_kine"`
	PGVNS       *float64 `json:"pgv_ns_kine"`
	PGVV        *float64 `json:"pgv_v_kine"`
	DistanceKm  *float64 `json:"distance_km"`
	BackAzimuth *float64 `json:"back_azimuth"`
	WaveURL     *string  `json:"wave_url"`
}

var quakeDatasets = []string{"earthquake", "earthquake_local"}

const quakeColumns = `report_id, kind, earthquake_no, origin_time, location, magnitude::float8, depth_km::float8,
	max_intensity, max_intensity_rank, report_color`

func scanQuake(row interface{ Scan(...any) error }, q *Quake, extra ...any) error {
	return row.Scan(append([]any{&q.ID, &q.Kind, &q.No, &q.OriginTime, &q.Location, &q.Magnitude, &q.DepthKm,
		&q.MaxIntensity, &q.MaxIntensityRank, &q.Color}, extra...)...)
}

func (s *Service) Quakes(ctx context.Context) ([]Quake, error) {
	list, _, err := s.quakes.Get(ctx, "quakes", quakeDatasets, 5*time.Minute,
		func(ctx context.Context) ([]Quake, error) {
			rows, err := s.App.QueryContext(ctx, `SELECT `+quakeColumns+`
				FROM data.earthquake ORDER BY origin_time DESC LIMIT 30`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []Quake{}
			for rows.Next() {
				var q Quake
				if err := scanQuake(rows, &q); err != nil {
					return nil, err
				}
				out = append(out, q)
			}
			return out, rows.Err()
		})
	return list, err
}

func (s *Service) Quake(ctx context.Context, id string) (*QuakeDetail, error) {
	d, _, err := s.quake.Get(ctx, id, quakeDatasets, 10*time.Minute,
		func(ctx context.Context) (*QuakeDetail, error) {
			var d QuakeDetail
			err := scanQuake(s.App.QueryRowContext(ctx, `SELECT `+quakeColumns+`,
				issued_at, content, report_url, image_url, ST_X(geom), ST_Y(geom), shakemap_url, report_type, report_remark
				FROM data.earthquake WHERE report_id = $1`, id),
				&d.Quake, &d.IssuedAt, &d.Content, &d.ReportURL, &d.ImageURL, &d.Lon, &d.Lat, &d.ShakemapURL,
				&d.ReportType, &d.Remark)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, &Error{Status: http.StatusNotFound, Code: "not_found", Message: "找不到這份地震報告"}
			}
			if err != nil {
				return nil, err
			}
			rows, err := s.App.QueryContext(ctx, `
				SELECT i.county_code, c.name, i.intensity, i.intensity_rank
				FROM data.earthquake_intensity i JOIN ref.county c ON c.code = i.county_code
				WHERE i.report_id = $1
				ORDER BY i.intensity_rank DESC, i.county_code`, id)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			d.Intensities = []QuakeIntensity{}
			for rows.Next() {
				var i QuakeIntensity
				if err := rows.Scan(&i.CountyCode, &i.County, &i.Intensity, &i.Rank); err != nil {
					return nil, err
				}
				d.Intensities = append(d.Intensities, i)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			rows.Close()
			stations, err := s.App.QueryContext(ctx, `
				SELECT s.station_id, s.name, s.county_code, c.name, ST_X(s.geom), ST_Y(s.geom), s.intensity, s.intensity_rank,
				       s.pga_gal::float8, s.pga_ew_gal::float8, s.pga_ns_gal::float8, s.pga_v_gal::float8,
				       s.pgv_kine::float8, s.pgv_ew_kine::float8, s.pgv_ns_kine::float8, s.pgv_v_kine::float8,
				       s.distance_km::float8, s.back_azimuth::float8, s.wave_url
				FROM data.earthquake_station s LEFT JOIN ref.county c ON c.code = s.county_code
				WHERE s.report_id = $1
				ORDER BY s.intensity_rank DESC NULLS LAST, s.pga_gal DESC NULLS LAST, s.station_id`, id)
			if err != nil {
				return nil, err
			}
			defer stations.Close()
			d.Stations = []QuakeStation{}
			for stations.Next() {
				var st QuakeStation
				if err := stations.Scan(&st.ID, &st.Name, &st.CountyCode, &st.County, &st.Lon, &st.Lat, &st.Intensity,
					&st.Rank, &st.PGA, &st.PGAEW, &st.PGANS, &st.PGAV, &st.PGV, &st.PGVEW, &st.PGVNS, &st.PGVV,
					&st.DistanceKm, &st.BackAzimuth, &st.WaveURL); err != nil {
					return nil, err
				}
				d.Stations = append(d.Stations, st)
			}
			return &d, stations.Err()
		})
	return d, err
}
