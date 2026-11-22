package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const DefaultRadarGridURL = cwa.FileAPI + "O-A0059-001"

type RadarGrid struct {
	ObservedAt time.Time
	Lon0, Lat0 float64 // the south-west grid point
	Step       float64 // degrees between grid points
	NX, NY     int
	DBZ        []float32 // NaN for no echo or no data
}

func ParseRadarGrid(body []byte) (RadarGrid, error) {
	var file struct {
		Open struct {
			Dataset struct {
				Info struct {
					Params struct {
						Lon  string `json:"StartPointLongitude"`
						Lat  string `json:"StartPointLatitude"`
						Step string `json:"GridResolution"`
						Time string `json:"DateTime"`
						NX   string `json:"GridDimensionX"`
						NY   string `json:"GridDimensionY"`
					} `json:"parameterSet"`
				} `json:"datasetInfo"`
				Contents struct {
					Content string `json:"content"`
				} `json:"contents"`
			} `json:"dataset"`
		} `json:"cwaopendata"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return RadarGrid{}, fmt.Errorf("decode radar grid: %w", err)
	}
	p := file.Open.Dataset.Info.Params
	var g RadarGrid
	var err error
	if g.ObservedAt, err = time.Parse(time.RFC3339, strings.TrimSpace(p.Time)); err != nil {
		return g, fmt.Errorf("雷達資料時間「%s」無法解析", p.Time)
	}
	nums := map[string]*float64{"StartPointLongitude": &g.Lon0, "StartPointLatitude": &g.Lat0, "GridResolution": &g.Step}
	for k, s := range map[string]string{"StartPointLongitude": p.Lon, "StartPointLatitude": p.Lat, "GridResolution": p.Step} {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return g, fmt.Errorf("雷達資料的 %s「%s」無法解析", k, s)
		}
		*nums[k] = v
	}
	if g.NX, err = strconv.Atoi(strings.TrimSpace(p.NX)); err != nil {
		return g, fmt.Errorf("雷達資料的 GridDimensionX「%s」無法解析", p.NX)
	}
	if g.NY, err = strconv.Atoi(strings.TrimSpace(p.NY)); err != nil {
		return g, fmt.Errorf("雷達資料的 GridDimensionY「%s」無法解析", p.NY)
	}
	if g.NX <= 0 || g.NY <= 0 || g.NX*g.NY > 4_000_000 || g.Step <= 0 {
		return g, fmt.Errorf("雷達資料的格點大小 %d×%d 不合理", g.NX, g.NY)
	}
	g.DBZ = make([]float32, 0, g.NX*g.NY)
	for _, s := range strings.Split(file.Open.Dataset.Contents.Content, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		v, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return g, fmt.Errorf("雷達回波值「%s」無法解析", s)
		}
		if v <= -99 || v > 100 {
			v = math.NaN()
		}
		g.DBZ = append(g.DBZ, float32(v))
	}
	if len(g.DBZ) != g.NX*g.NY {
		return g, fmt.Errorf("雷達資料有 %d 個值，應為 %d×%d", len(g.DBZ), g.NX, g.NY)
	}
	return g, nil
}

func (g RadarGrid) Bounds() [4]float64 {
	h := g.Step / 2
	return [4]float64{g.Lon0 - h, g.Lat0 - h, g.Lon0 + float64(g.NX-1)*g.Step + h, g.Lat0 + float64(g.NY-1)*g.Step + h}
}

var RadarColors = []color.RGBA{
	{0x9b, 0xf5, 0xff, 0xff}, // 0–5
	{0x00, 0xd5, 0xff, 0xff}, // 5–10
	{0x00, 0x78, 0xff, 0xff}, // 10–15
	{0x00, 0xff, 0x00, 0xff}, // 15–20
	{0x00, 0xc8, 0x00, 0xff}, // 20–25
	{0x00, 0x96, 0x00, 0xff}, // 25–30
	{0xff, 0xff, 0x00, 0xff}, // 30–35
	{0xff, 0xc8, 0x00, 0xff}, // 35–40
	{0xff, 0x8c, 0x00, 0xff}, // 40–45
	{0xff, 0x00, 0x00, 0xff}, // 45–50
	{0xc8, 0x00, 0x00, 0xff}, // 50–55
	{0x96, 0x00, 0x00, 0xff}, // 55–60
	{0xff, 0x00, 0xff, 0xff}, // 60–65
	{0x96, 0x00, 0xff, 0xff}, // 65 以上
}

const minDBZ = 5

func RenderRadar(g RadarGrid) ([]byte, int, int, error) {
	b := g.Bounds()
	mercY := func(lat float64) float64 { return math.Log(math.Tan(math.Pi/4 + lat*math.Pi/360)) }
	latOf := func(y float64) float64 { return (2*math.Atan(math.Exp(y)) - math.Pi/2) * 180 / math.Pi }
	yN, yS := mercY(b[3]), mercY(b[1])
	w := g.NX
	midLat := (b[1] + b[3]) / 2
	h := int(math.Round(float64(g.NY) * (yN - yS) / ((b[3] - b[1]) * math.Pi / 180) * math.Cos(midLat*math.Pi/180)))
	if h < 1 {
		h = 1
	}

	palette := color.Palette{color.RGBA{}}
	for _, c := range RadarColors {
		palette = append(palette, c)
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	for row := 0; row < h; row++ {
		lat := latOf(yN - (float64(row)+0.5)/float64(h)*(yN-yS))
		j := int(math.Round((lat - g.Lat0) / g.Step))
		if j < 0 || j >= g.NY {
			continue
		}
		for i := 0; i < w; i++ {
			v := g.DBZ[j*g.NX+i]
			if math.IsNaN(float64(v)) || v < minDBZ {
				continue
			}
			k := int(v / 5)
			if k >= len(RadarColors) {
				k = len(RadarColors) - 1
			}
			img.SetColorIndex(i, row, uint8(k+1))
		}
	}
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, img); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), w, h, nil
}

type RadarLayerPipeline struct {
	URL    string
	APIKey string
	Dir    string // IMAGE_DIR
}

func (p *RadarLayerPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "radar_layer",
		Name:         "雷達回波地圖圖層",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/observation/O-A0059-001",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *RadarLayerPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.Dir == "" {
		return res, errors.New("IMAGE_DIR 沒有設定")
	}
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	g, err := ParseRadarGrid(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = 1
	res.ObserveTime(g.ObservedAt)
	exists, err := frameExists(ctx, env, "radar_layer", g.ObservedAt)
	if err != nil {
		return res, err
	}
	if !exists {
		data, w, h, err := RenderRadar(g)
		if err != nil {
			return res, err
		}
		bounds := g.Bounds()
		if err := saveFrame(ctx, env, p.Dir, Frame{Product: "radar_layer", ObservedAt: g.ObservedAt,
			Ext: ".png", Data: data, Width: w, Height: h, Bounds: &bounds}); err != nil {
			return res, err
		}
		res.RowsInserted = 1
	}
	if err := pruneFrames(ctx, env, p.Dir, "radar_layer"); err != nil {
		res.Warnf("清除舊影像失敗：%v", err)
	}
	return res, nil
}
