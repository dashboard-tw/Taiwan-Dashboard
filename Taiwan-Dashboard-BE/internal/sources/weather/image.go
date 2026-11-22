package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // the radar image is a PNG
	"net/url"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

type ImageProduct struct {
	ID     string // radar or satellite, the directory and data.weather_image.product
	DataID string // the CWA data id
	Name   string
	Source string // dataset page
}

var ImageProducts = map[string]ImageProduct{
	"radar": {ID: "radar", DataID: "O-A0058-003", Name: "雷達合成回波圖（臺灣）",
		Source: "https://opendata.cwa.gov.tw/dataset/observation/O-A0058-003"},
	"satellite": {ID: "satellite", DataID: "O-B0032-002", Name: "衛星雲圖（東亞紅外線彩色）",
		Source: "https://opendata.cwa.gov.tw/dataset/observation/O-B0032-002"},
}

const (
	ImageRetention = 7 * 24 * time.Hour
	imageMaxSide   = 1200
)

var imageHost = "cwaopendata.s3.ap-northeast-1.amazonaws.com"

func ImageFile(t time.Time) string { return FrameFile(t, ".jpg") }

type ImageMeta struct {
	URL        string
	ObservedAt time.Time
}

func ParseImageMeta(body []byte) (ImageMeta, error) {
	var file struct {
		Open struct {
			Dataset struct {
				DateTime string `json:"DateTime"`
				ObsTime  struct {
					Datetime string `json:"Datetime"`
				} `json:"ObsTime"`
				Resource struct {
					ProductURL string `json:"ProductURL"`
				} `json:"resource"`
				Resource2 struct {
					ProductURL string `json:"ProductURL"`
				} `json:"Resource"`
			} `json:"dataset"`
		} `json:"cwaopendata"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return ImageMeta{}, fmt.Errorf("decode image description: %w", err)
	}
	d := file.Open.Dataset
	m := ImageMeta{URL: strings.TrimSpace(d.Resource.ProductURL)}
	if m.URL == "" {
		m.URL = strings.TrimSpace(d.Resource2.ProductURL)
	}
	ts := strings.TrimSpace(d.DateTime)
	if ts == "" {
		ts = strings.TrimSpace(d.ObsTime.Datetime)
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("影像時間「%s」無法解析", ts)
	}
	m.ObservedAt = t
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host != imageHost {
		return ImageMeta{}, fmt.Errorf("影像網址「%s」不是氣象署的檔案位置", m.URL)
	}
	return m, nil
}

func Shrink(src []byte, max int) ([]byte, int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > max || h > max {
		if w >= h {
			w, h = max, h*max/w
		} else {
			w, h = w*max/h, max
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		img = dst
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), w, h, nil
}

type ImagePipeline struct {
	Product ImageProduct
	URL     string // the file API description, cwa.FileAPI + Product.DataID
	APIKey  string
	Dir     string // IMAGE_DIR
}

func (p *ImagePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           p.Product.ID + "_image",
		Name:         p.Product.Name,
		SourceAgency: "交通部中央氣象署",
		SourceURL:    p.Product.Source,
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *ImagePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.Dir == "" {
		return res, errors.New("IMAGE_DIR 沒有設定")
	}
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	meta, err := ParseImageMeta(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = 1
	res.ObserveTime(meta.ObservedAt)

	exists, err := frameExists(ctx, env, p.Product.ID, meta.ObservedAt)
	if err != nil {
		return res, err
	}
	if !exists {
		raw, err := env.Fetch.Get(ctx, meta.URL, meta.URL)
		if err != nil {
			return res, err
		}
		jpg, w, h, err := Shrink(raw, imageMaxSide)
		if err != nil {
			return res, err
		}
		if err := saveFrame(ctx, env, p.Dir, Frame{Product: p.Product.ID, ObservedAt: meta.ObservedAt,
			Ext: ".jpg", Data: jpg, Width: w, Height: h}); err != nil {
			return res, err
		}
		res.RowsInserted = 1
	}
	if err := pruneFrames(ctx, env, p.Dir, p.Product.ID); err != nil {
		res.Warnf("清除舊影像失敗：%v", err)
	}
	return res, nil
}
