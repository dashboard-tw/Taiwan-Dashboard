package weather

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func radarJSON(nx, ny int, values []string) string {
	return fmt.Sprintf(`{"cwaopendata": {"dataset": {
		"datasetInfo": {"parameterSet": {"StartPointLongitude": "120.0", "StartPointLatitude": "22.0",
			"GridResolution": "0.5", "DateTime": "2026-10-22T17:40:00+08:00",
			"GridDimensionX": "%d", "GridDimensionY": "%d", "Reflectivity": "dBZ"}},
		"contents": {"contentDescription": "", "content": "%s"}}}}`, nx, ny, strings.Join(values, ","))
}

func TestParseRadarGrid(t *testing.T) {
	g, err := ParseRadarGrid([]byte(radarJSON(3, 2, []string{"-9.990E+02", "-9.900E+01", "1.200E+01", "4.600E+01", "7.000E+01", "0.0"})))
	if err != nil {
		t.Fatal(err)
	}
	if g.NX != 3 || g.NY != 2 || g.Lon0 != 120 || g.Lat0 != 22 || g.Step != 0.5 || !g.ObservedAt.Equal(at(17, 40)) {
		t.Errorf("grid = %+v", g)
	}
	if !math.IsNaN(float64(g.DBZ[0])) || !math.IsNaN(float64(g.DBZ[1])) || g.DBZ[2] != 12 || g.DBZ[3] != 46 {
		t.Errorf("values = %v", g.DBZ)
	}
	if b := g.Bounds(); b != [4]float64{119.75, 21.75, 121.25, 22.75} {
		t.Errorf("bounds = %v", b)
	}
	if _, err := ParseRadarGrid([]byte(radarJSON(3, 2, []string{"1", "2"}))); err == nil {
		t.Error("a short grid should fail")
	}
}

func TestRenderRadar(t *testing.T) {
	vals := make([]string, 4*4)
	for i := range vals {
		vals[i] = "-9.900E+01"
	}
	vals[3*4+0] = "4.600E+01" // row 3 = northernmost
	g, err := ParseRadarGrid([]byte(radarJSON(4, 4, vals)))
	if err != nil {
		t.Fatal(err)
	}
	data, w, h, err := RenderRadar(g)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, w, h) || w != 4 {
		t.Fatalf("size %v (%d×%d)", img.Bounds(), w, h)
	}
	if r, g2, b, a := img.At(0, 0).RGBA(); a == 0 || r>>8 != 0xff || g2 != 0 || b != 0 {
		t.Errorf("top-left = %v", img.At(0, 0))
	}
	if _, _, _, a := img.At(w-1, h-1).RGBA(); a != 0 {
		t.Errorf("bottom-right should be transparent, got %v", img.At(w-1, h-1))
	}
}

func TestRadarLayerRun(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	vals := make([]string, 4*4)
	for i := range vals {
		vals[i] = "3.000E+01"
	}
	grid := filepath.Join(dir, "grid.json")
	if err := os.WriteFile(grid, []byte(radarJSON(4, 4, vals)), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := pipelinetest.NewServer(t, map[string]string{"/grid": grid})
	images := filepath.Join(dir, "images")
	p := &RadarLayerPipeline{URL: srv.URL + "/grid", APIKey: "k", Dir: images}
	ctx := context.Background()

	if res, err := p.Run(ctx, srv.Env(db, at(17, 45))); err != nil || res.RowsInserted != 1 {
		t.Fatalf("first run: %+v, %v", res, err)
	}
	if res, err := p.Run(ctx, srv.Env(db, at(17, 50))); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run: %+v, %v", res, err)
	}
	st, err := os.Stat(filepath.Join(images, "radar_layer", "20261022T0940Z.png"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o044 != 0o044 && os.PathSeparator == '/' {
		t.Errorf("mode = %v", st.Mode())
	}
	var west, north float64
	if err := db.QueryRow(`SELECT west, north FROM data.weather_image WHERE product = 'radar_layer'`).Scan(&west, &north); err != nil || west != 119.75 || north != 23.75 {
		t.Errorf("bounds %v %v (%v)", west, north, err)
	}
}
