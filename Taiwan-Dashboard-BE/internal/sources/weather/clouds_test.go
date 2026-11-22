package weather

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func TestParseColorMap(t *testing.T) {
	temps, err := ParseColorMap(read(t, "testdata/colormap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 20 {
		t.Errorf("colours = %d", len(temps))
	}
	if v := temps[[3]uint8{255, 255, 255}]; math.Abs(v+91.6) > 1e-9 {
		t.Errorf("white = %v", v)
	}
	if v := temps[[3]uint8{90, 90, 90}]; math.Abs(v-22.65) > 1e-9 {
		t.Errorf("grey = %v", v)
	}
	if v := temps.temp([3]uint8{91, 89, 90}, map[[3]uint8]float64{}); math.Abs(v-22.65) > 1e-9 {
		t.Errorf("nearest = %v", v)
	}
}

func TestCloudLevel(t *testing.T) {
	cases := map[float64]uint8{30: 0, 10: 0, 9.99: 1, -25: cloudLevels / 2, -60: cloudLevels, -90: cloudLevels}
	for temp, want := range cases {
		if got := cloudLevel(temp); got != want {
			t.Errorf("cloudLevel(%v) = %d, want %d", temp, got, want)
		}
	}
	if cloudLevel(math.NaN()) != 0 {
		t.Error("unknown colours are clear")
	}
}

func TestTileRange(t *testing.T) {
	x0, x1, y0, y1 := tileRange()
	if x0 != 51 || x1 != 56 || y0 != 25 || y1 != 29 {
		t.Errorf("tiles x %d–%d y %d–%d", x0, x1, y0, y1)
	}
	b := tileBounds(x0, x1, y0, y1)
	if b[0] > CloudArea.West || b[1] > CloudArea.South || b[2] < CloudArea.East || b[3] < CloudArea.North {
		t.Errorf("bounds %v do not cover the area", b)
	}
}

func tilePNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCloudRun(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	cold := filepath.Join(dir, "cold.png")
	if err := os.WriteFile(cold, tilePNG(t, color.NRGBA{255, 255, 255, 255}), 0o600); err != nil {
		t.Fatal(err)
	}
	routes := map[string]string{"/colormap.xml": "testdata/colormap.xml"}
	x0, x1, y0, y1 := tileRange()
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			routes[fmt.Sprintf("/2026-10-22T09:40:00Z/%d/%d/%d.png", CloudArea.Zoom, y, x)] = cold
		}
	}
	srv := pipelinetest.NewServer(t, routes)
	images := filepath.Join(dir, "images")
	p := &CloudPipeline{TileURL: srv.URL + "/%s/%d/%d/%d.png", ColorURL: srv.URL + "/colormap.xml", Dir: images}
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 18, 5, 0, 0, pipeline.Taipei)

	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil || res.RowsInserted != 1 {
		t.Fatalf("run: %+v, %v", res, err)
	}
	f, err := os.Open(filepath.Join(images, "cloud_layer", "20261022T0940Z.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if w := img.Bounds().Dx(); w != (x1-x0+1)*256 {
		t.Errorf("width %d", w)
	}
	if _, _, _, a := img.At(10, 10).RGBA(); a>>8 != 255 {
		t.Errorf("a -91 °C cloud top should be opaque, alpha %d", a>>8)
	}
	var west, north float64
	if err := db.QueryRow(`SELECT west, north FROM data.weather_image WHERE product = 'cloud_layer'`).Scan(&west, &north); err != nil || west != tileBounds(x0, x1, y0, y1)[0] {
		t.Errorf("bounds %v %v (%v)", west, north, err)
	}
	if res, err := p.Run(ctx, srv.Env(db, now.Add(5*time.Minute))); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
}
