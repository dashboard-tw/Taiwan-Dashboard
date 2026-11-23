package weather

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultWindURL = "https://cwaopendata.s3.ap-northeast-1.amazonaws.com/Model/M-A0061-%03d.grb2"

var WindArea = struct {
	West, South, East, North, Step float64
}{100, 5, 150, 40, 0.25}

const WindMax = 50.0

var windLeads = []int{0, 6, 12}

const windHeader = 512

func RenderWind(u, v *gribField) ([]byte, int, int, error) {
	if u.Grid != v.Grid || len(u.Values) != len(v.Values) {
		return nil, 0, 0, errors.New("U、V 的格點不一致")
	}
	pr := newLambertProj(&u.Grid)
	a := WindArea
	w := int(math.Round((a.East-a.West)/a.Step)) + 1
	h := int(math.Round((a.North-a.South)/a.Step)) + 1
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	enc := func(x float64) uint8 {
		x = math.Max(-WindMax, math.Min(WindMax, x))
		return uint8(math.Round((x + WindMax) / (2 * WindMax) * 254))
	}
	valid := 0
	for y := 0; y < h; y++ {
		lat := a.North - float64(y)*a.Step
		for x := 0; x < w; x++ {
			lon := a.West + float64(x)*a.Step
			gu, gv := pr.sample(u.Values, lat, lon), pr.sample(v.Values, lat, lon)
			if math.IsNaN(gu) || math.IsNaN(gv) || math.Abs(gu) > 150 || math.Abs(gv) > 150 {
				continue // transparent: no data
			}
			eu, ev := pr.earthWind(gu, gv, lon)
			img.SetNRGBA(x, y, color.NRGBA{enc(eu), enc(ev), 0, 255})
			valid++
		}
	}
	if valid < w*h/2 {
		return nil, 0, 0, fmt.Errorf("只有 %d/%d 個格點有風的資料", valid, w*h)
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}

type WindPipeline struct {
	URL string // fmt pattern taking the forecast hour
	Dir string // IMAGE_DIR

	offsets map[int][2]int64
}

func (p *WindPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "wind_layer",
		Name:         "風場（數值預報 10 公尺風）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/mathematics/M-A0061-000",
		UpdateFreq:   6 * time.Hour,
		StaleAfter:   18 * time.Hour,
	}
}

var windFile = regexp.MustCompile(`-f(\d{2})\.png$`)

func windFrameName(valid time.Time, lead int) string {
	return valid.UTC().Format("20060102T1504Z") + fmt.Sprintf("-f%02d.png", lead)
}

func (p *WindPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.Dir == "" {
		return res, errors.New("IMAGE_DIR 沒有設定")
	}
	if p.offsets == nil {
		p.offsets = map[int][2]int64{}
	}
	for _, lead := range windLeads {
		url := fmt.Sprintf(p.URL, lead)
		stored, ref, err := p.lead(ctx, env, url, lead)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && (he.Status == 404 || he.Status == 403) {
			res.Warnf("第 %d 小時的預報檔不存在", lead)
			continue
		}
		if err != nil {
			return res, fmt.Errorf("預報第 %d 小時：%w", lead, err)
		}
		res.RowsFetched++
		if stored {
			res.RowsInserted++
		}
		res.ObserveTime(ref)
	}
	if err := pruneFrames(ctx, env, p.Dir, "wind_layer"); err != nil {
		res.Warnf("清除舊影像失敗：%v", err)
	}
	return res, nil
}

func (p *WindPipeline) lead(ctx context.Context, env pipeline.Env, url string, lead int) (bool, time.Time, error) {
	head, err := env.Fetch.GetRange(ctx, url, 0, windHeader)
	if err != nil {
		return false, time.Time{}, err
	}
	first, err := parseGrib(head, true)
	if err != nil {
		return false, time.Time{}, err
	}
	ref := first.RefTime
	valid := ref.Add(time.Duration(lead) * time.Hour)

	var oldFile string
	err = env.DB.QueryRowContext(ctx,
		`SELECT file FROM data.weather_image WHERE product = 'wind_layer' AND observed_at = $1`, valid).Scan(&oldFile)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, ref, err
	}
	if oldFile != "" {
		if m := windFile.FindStringSubmatch(oldFile); m != nil {
			if old, _ := strconv.Atoi(m[1]); old <= lead {
				return false, ref, nil
			}
		}
	}

	u, v, err := p.fields(ctx, env, url, lead, ref)
	if err != nil {
		return false, ref, err
	}
	data, w, h, err := RenderWind(u, v)
	if err != nil {
		return false, ref, err
	}
	if oldFile != "" {
		if _, err := env.DB.ExecContext(ctx,
			`DELETE FROM data.weather_image WHERE product = 'wind_layer' AND observed_at = $1`, valid); err != nil {
			return false, ref, err
		}
	}
	bounds := [4]float64{WindArea.West, WindArea.South, WindArea.East, WindArea.North}
	if err := saveFrame(ctx, env, p.Dir, Frame{Product: "wind_layer", ObservedAt: valid, Name: windFrameName(valid, lead),
		Data: data, Width: w, Height: h, Bounds: &bounds}); err != nil {
		return false, ref, err
	}
	if oldFile != "" {
		if err := os.Remove(filepath.Join(p.Dir, filepath.FromSlash(oldFile))); err != nil && !errors.Is(err, os.ErrNotExist) {
			env.Logger.Warn("remove replaced wind frame", "file", oldFile, "err", err)
		}
	}
	return true, ref, nil
}

func isWind(f *gribField, param uint8) bool {
	return f.Discipline == 0 && f.Category == 2 && f.Number == param && f.LevelType == 103 && f.Level == 10
}

func (p *WindPipeline) fields(ctx context.Context, env pipeline.Env, url string, lead int, ref time.Time) (*gribField, *gribField, error) {
	want := func(f *gribField, param uint8) bool {
		return isWind(f, param) && f.RefTime.Equal(ref) && f.Lead == lead
	}
	if off, ok := p.offsets[lead]; ok {
		u, err1 := p.message(ctx, env, url, off[0], func(f *gribField) bool { return want(f, 2) })
		v, err2 := p.message(ctx, env, url, off[1], func(f *gribField) bool { return want(f, 3) })
		if err1 == nil && err2 == nil && u != nil && v != nil {
			return u, v, nil
		}
	}
	var u, v *gribField
	var offs [2]int64
	var off int64
	for n := 0; n < 500 && (u == nil || v == nil); n++ {
		head, err := env.Fetch.GetRange(ctx, url, off, windHeader)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 416 {
			break // end of file
		}
		if err != nil {
			return nil, nil, err
		}
		total, err := gribHeader(head)
		if err != nil {
			return nil, nil, fmt.Errorf("位置 %d：%w", off, err)
		}
		f, err := parseGrib(head, true)
		if err != nil {
			return nil, nil, fmt.Errorf("位置 %d：%w", off, err)
		}
		if !f.RefTime.Equal(ref) {
			return nil, nil, errors.New("讀取期間檔案換成了新的預報，下次再試")
		}
		for i, param := range []uint8{2, 3} {
			if !want(f, param) {
				continue
			}
			full, err := p.message(ctx, env, url, off, func(*gribField) bool { return true })
			if err != nil {
				return nil, nil, err
			}
			offs[i] = off
			if param == 2 {
				u = full
			} else {
				v = full
			}
		}
		off += total
	}
	if u == nil || v == nil {
		return nil, nil, errors.New("檔案裡找不到 10 公尺風")
	}
	p.offsets[lead] = offs
	return u, v, nil
}

func (p *WindPipeline) message(ctx context.Context, env pipeline.Env, url string, off int64, ok func(*gribField) bool) (*gribField, error) {
	head, err := env.Fetch.GetRange(ctx, url, off, windHeader)
	if err != nil {
		return nil, err
	}
	total, err := gribHeader(head)
	if err != nil {
		return nil, err
	}
	h, err := parseGrib(head, true)
	if err != nil {
		return nil, err
	}
	if !ok(h) {
		return nil, nil
	}
	body, err := env.Fetch.GetRange(ctx, url, off, total)
	if err != nil {
		return nil, err
	}
	return parseGrib(body, false)
}
