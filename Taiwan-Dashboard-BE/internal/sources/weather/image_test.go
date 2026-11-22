package weather

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseImageMeta(t *testing.T) {
	radar, err := ParseImageMeta(read(t, "testdata/radar_meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(radar.URL, "/O-A0058-003.png") || !radar.ObservedAt.Equal(at(11, 30)) {
		t.Errorf("radar = %+v", radar)
	}
	sat, err := ParseImageMeta(read(t, "testdata/satellite_meta.json"))
	if err != nil || !strings.HasSuffix(sat.URL, "/O-B0032-002.jpg") || !sat.ObservedAt.Equal(at(11, 30)) {
		t.Errorf("satellite = %+v, %v", sat, err)
	}
	bad := strings.Replace(string(read(t, "testdata/radar_meta.json")), imageHost, "evil.example.com", 1)
	if _, err := ParseImageMeta([]byte(bad)); err == nil {
		t.Error("a foreign image host should be refused")
	}
}

func TestImageFile(t *testing.T) {
	if got := ImageFile(at(17, 30)); got != "20261022T0930Z.jpg" {
		t.Errorf("ImageFile = %s", got)
	}
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{R: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestShrink(t *testing.T) {
	out, w, h, err := Shrink(pngOf(t, 3600, 1800), 1200)
	if err != nil || w != 1200 || h != 600 {
		t.Fatalf("w=%d h=%d err=%v", w, h, err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 1200 || cfg.Height != 600 {
		t.Errorf("output %+v, %v", cfg, err)
	}
	if _, w, h, err := Shrink(pngOf(t, 400, 300), 1200); err != nil || w != 400 || h != 300 {
		t.Errorf("small: %d×%d, %v", w, h, err)
	}
}

func TestImageRun(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "radar.png")
	if err := os.WriteFile(img, pngOf(t, 3600, 3600), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := pipelinetest.NewServer(t, map[string]string{"/radar.png": img})
	u, _ := url.Parse(srv.URL)
	old := imageHost
	imageHost = u.Host
	t.Cleanup(func() { imageHost = old })

	meta := strings.Replace(string(read(t, "testdata/radar_meta.json")),
		"https://"+old+"/Observation/O-A0058-003.png", srv.URL+"/radar.png", 1)
	metaFile := filepath.Join(dir, "meta.json")
	if err := os.WriteFile(metaFile, []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.Set("/meta", metaFile)

	images := filepath.Join(dir, "images")
	p := &ImagePipeline{Product: ImageProducts["radar"], URL: srv.URL + "/meta", APIKey: "k", Dir: images}
	ctx := context.Background()
	if res, err := p.Run(ctx, srv.Env(db, at(11, 40))); err != nil || res.RowsInserted != 1 {
		t.Fatalf("first run: %+v, %v", res, err)
	}
	file := filepath.Join(images, "radar", "20261022T0330Z.jpg")
	if cfg, err := readJPEG(file); err != nil || cfg.Width != 1200 {
		t.Errorf("stored image: %+v, %v", cfg, err)
	}
	if res, err := p.Run(ctx, srv.Env(db, at(11, 50))); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
	if _, err := p.Run(ctx, srv.Env(db, at(11, 40).Add(ImageRetention+time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("old file still there: %v", err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.weather_image WHERE observed_at = '2026-10-22 11:30+08'`); n != 0 {
		t.Errorf("old row still there")
	}
}

func readJPEG(path string) (image.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Config{}, err
	}
	defer f.Close()
	return jpeg.DecodeConfig(f)
}
