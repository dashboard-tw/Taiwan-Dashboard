package weather

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultCloudTileURL  = "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/Himawari_AHI_Band13_Clean_Infrared/default/%s/GoogleMapsCompatible_Level6/%d/%d/%d.png"
	DefaultCloudColorURL = "https://gibs.earthdata.nasa.gov/colormaps/v1.3/Clean_Longwave_Infrared_Window_Band.xml"
)

var CloudArea = struct {
	West, South, East, North float64
	Zoom                     int
}{110, 12, 135, 32, 6}

const (
	warmC         = 10.0
	coldC         = -60.0
	cloudLevels   = 48
	cloudBackfill = 6
	cloudLookback = 3 * time.Hour
)

type ColorTemps map[[3]uint8]float64

func ParseColorMap(body []byte) (ColorTemps, error) {
	var doc struct {
		Maps []struct {
			Entries []struct {
				RGB         string `xml:"rgb,attr"`
				Transparent string `xml:"transparent,attr"`
				Value       string `xml:"value,attr"`
			} `xml:"Entries>ColorMapEntry"`
		} `xml:"ColorMap"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode colour map: %w", err)
	}
	out := ColorTemps{}
	for _, m := range doc.Maps {
		for _, e := range m.Entries {
			if e.Transparent == "true" {
				continue
			}
			parts := strings.Split(e.RGB, ",")
			if len(parts) != 3 {
				continue
			}
			var rgb [3]uint8
			ok := true
			for i, p := range parts {
				v, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil || v < 0 || v > 255 {
					ok = false
					break
				}
				rgb[i] = uint8(v)
			}
			lo, hi, okv := interval(e.Value)
			if !ok || !okv {
				continue
			}
			out[rgb] = (lo + hi) / 2
		}
	}
	if len(out) < 10 {
		return nil, fmt.Errorf("色表只有 %d 個顏色", len(out))
	}
	return out, nil
}

func interval(s string) (float64, float64, bool) {
	s = strings.Trim(strings.TrimSpace(s), "()[]")
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		v, err := strconv.ParseFloat(s, 64)
		return v, v, err == nil
	}
	lo, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	hi, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	switch {
	case err1 == nil && err2 == nil:
		return lo, hi, true
	case err1 == nil:
		return lo, lo, true
	case err2 == nil:
		return hi, hi, true
	}
	return 0, 0, false
}

func (c ColorTemps) temp(rgb [3]uint8, cache map[[3]uint8]float64) float64 {
	if t, ok := c[rgb]; ok {
		return t
	}
	if t, ok := cache[rgb]; ok {
		return t
	}
	best, bestD := math.NaN(), math.MaxFloat64
	for k, t := range c {
		dr, dg, db := float64(k[0])-float64(rgb[0]), float64(k[1])-float64(rgb[1]), float64(k[2])-float64(rgb[2])
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = t, d
		}
	}
	cache[rgb] = best
	return best
}

func cloudPalette() color.Palette {
	p := color.Palette{color.NRGBA{}}
	for i := 1; i <= cloudLevels; i++ {
		f := float64(i) / cloudLevels
		g := uint8(190 + 65*f) // thin cloud a little grey, thick cloud white
		p = append(p, color.NRGBA{g, g, g, uint8(math.Round(255 * math.Pow(f, 0.8)))})
	}
	return p
}

func cloudLevel(t float64) uint8 {
	if math.IsNaN(t) || t >= warmC {
		return 0
	}
	f := (warmC - t) / (warmC - coldC)
	if f > 1 {
		f = 1
	}
	return uint8(math.Max(1, math.Round(f*cloudLevels)))
}

func tileRange() (x0, x1, y0, y1 int) {
	n := math.Exp2(float64(CloudArea.Zoom))
	col := func(lon float64) int { return int(math.Floor((lon + 180) / 360 * n)) }
	row := func(lat float64) int {
		r := lat * math.Pi / 180
		return int(math.Floor((1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2 * n))
	}
	return col(CloudArea.West), col(CloudArea.East), row(CloudArea.North), row(CloudArea.South)
}

func tileBounds(x0, x1, y0, y1 int) [4]float64 {
	n := math.Exp2(float64(CloudArea.Zoom))
	lon := func(x int) float64 { return float64(x)/n*360 - 180 }
	lat := func(y int) float64 { return math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi }
	return [4]float64{lon(x0), lat(y1 + 1), lon(x1 + 1), lat(y0)}
}

func RenderClouds(src image.Image, temps ColorTemps) ([]byte, error) {
	b := src.Bounds()
	out := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), cloudPalette())
	cache := map[[3]uint8]float64{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if c.A < 128 {
				continue // outside the satellite's view
			}
			out.SetColorIndex(x-b.Min.X, y-b.Min.Y, cloudLevel(temps.temp([3]uint8{c.R, c.G, c.B}, cache)))
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type CloudPipeline struct {
	TileURL  string // fmt pattern: time, zoom, row, column
	ColorURL string
	Dir      string // IMAGE_DIR
}

func (p *CloudPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "cloud_layer",
		Name:         "動態雲圖（向日葵衛星紅外線）",
		SourceAgency: "NASA GIBS（日本氣象廳向日葵衛星）",
		SourceURL:    "https://nasa-gibs.github.io/gibs-api-docs/",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   3 * time.Hour,
		License:      "NASA 開放資料（無使用限制，需標示 NASA GIBS 與日本氣象廳）",
	}
}

func (p *CloudPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.Dir == "" {
		return res, errors.New("IMAGE_DIR 沒有設定")
	}
	body, err := env.Fetch.Get(ctx, p.ColorURL, p.ColorURL)
	if err != nil {
		return res, err
	}
	temps, err := ParseColorMap(body)
	if err != nil {
		return res, err
	}

	now := env.Now().UTC().Truncate(10 * time.Minute)
	var missing []time.Time
	for t := now; !t.Before(now.Add(-cloudLookback)); t = t.Add(-10 * time.Minute) {
		exists, err := frameExists(ctx, env, "cloud_layer", t)
		if err != nil {
			return res, err
		}
		if !exists {
			missing = append(missing, t)
		}
	}
	stored := 0
	for _, t := range missing {
		if stored >= cloudBackfill {
			break
		}
		ok, err := p.frame(ctx, env, t, temps)
		if err != nil {
			return res, err
		}
		if ok {
			stored++
			res.ObserveTime(t)
		}
	}
	res.RowsFetched = stored
	res.RowsInserted = stored
	var latest *time.Time
	if err := env.DB.QueryRowContext(ctx,
		`SELECT max(observed_at) FROM data.weather_image WHERE product = 'cloud_layer'`).Scan(&latest); err == nil && latest != nil {
		res.ObserveTime(*latest)
	}
	if err := pruneFrames(ctx, env, p.Dir, "cloud_layer"); err != nil {
		res.Warnf("清除舊影像失敗：%v", err)
	}
	return res, nil
}

func (p *CloudPipeline) frame(ctx context.Context, env pipeline.Env, t time.Time, temps ColorTemps) (bool, error) {
	x0, x1, y0, y1 := tileRange()
	ts := t.Format("2006-01-02T15:04:05Z")
	canvas := image.NewNRGBA(image.Rect(0, 0, (x1-x0+1)*256, (y1-y0+1)*256))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			u := fmt.Sprintf(p.TileURL, ts, CloudArea.Zoom, y, x)
			b, err := env.Fetch.Get(ctx, u, u)
			var he *pipeline.HTTPError
			if errors.As(err, &he) && (he.Status == 404 || he.Status == 400) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			tile, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				return false, fmt.Errorf("decode cloud tile %s: %w", u, err)
			}
			draw.Draw(canvas, image.Rect((x-x0)*256, (y-y0)*256, (x-x0+1)*256, (y-y0+1)*256), tile, tile.Bounds().Min, draw.Src)
		}
	}
	data, err := RenderClouds(canvas, temps)
	if err != nil {
		return false, err
	}
	bounds := tileBounds(x0, x1, y0, y1)
	err = saveFrame(ctx, env, p.Dir, Frame{Product: "cloud_layer", ObservedAt: t, Ext: ".png", Data: data,
		Width: canvas.Bounds().Dx(), Height: canvas.Bounds().Dy(), Bounds: &bounds})
	return err == nil, err
}
