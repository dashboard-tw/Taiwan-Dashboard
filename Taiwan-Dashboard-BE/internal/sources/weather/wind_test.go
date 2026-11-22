package weather

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

var testGrid = lambertGrid{Nx: 70, Ny: 55, La1: -5, Lo1: 90, LaD: 10, LoV: 120, Dx: 100000, Dy: 100000,
	Latin1: 10, Latin2: 40, ScanMode: 0x40, EarthRelUV: true, EarthRadiusM: 6371229}

func gribMessage(ref time.Time, lead int, category, number uint8, level uint32, value func(i, j int) float64) []byte {
	g := testGrid
	sec := func(n byte, body []byte) []byte {
		b := make([]byte, 5, 5+len(body))
		binary.BigEndian.PutUint32(b, uint32(5+len(body)))
		b[4] = n
		return append(b, body...)
	}
	u16 := func(v int) []byte { return binary.BigEndian.AppendUint16(nil, uint16(v)) }
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	sm32 := func(v float64) []byte {
		x := uint32(math.Round(math.Abs(v) * 1e6))
		if v < 0 {
			x |= 0x80000000
		}
		return u32(x)
	}
	var s1 []byte
	s1 = append(s1, u16(0)...)          // centre
	s1 = append(s1, u16(0)...)          // subcentre
	s1 = append(s1, 2, 1, 1)            // tables, significance of the reference time
	s1 = append(s1, u16(ref.Year())...) // reference time
	s1 = append(s1, byte(ref.Month()), byte(ref.Day()), byte(ref.Hour()), byte(ref.Minute()), 0)
	s1 = append(s1, 0, 1) // status, type
	var s3 []byte
	s3 = append(s3, 0)                                              // source of grid definition
	s3 = append(s3, u32(uint32(g.Nx*g.Ny))...)                      // number of points
	s3 = append(s3, 0, 0)                                           // no optional list
	s3 = append(s3, u16(30)...)                                     // Lambert conformal
	s3 = append(s3, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // spherical earth, 6,371,229 m
	s3 = append(s3, u32(uint32(g.Nx))...)
	s3 = append(s3, u32(uint32(g.Ny))...)
	s3 = append(s3, sm32(g.La1)...)
	s3 = append(s3, u32(uint32(g.Lo1*1e6))...)
	s3 = append(s3, 0) // u and v towards east and north
	s3 = append(s3, sm32(g.LaD)...)
	s3 = append(s3, u32(uint32(g.LoV*1e6))...)
	s3 = append(s3, u32(uint32(g.Dx*1e3))...)
	s3 = append(s3, u32(uint32(g.Dy*1e3))...)
	s3 = append(s3, 0, g.ScanMode)
	s3 = append(s3, sm32(g.Latin1)...)
	s3 = append(s3, sm32(g.Latin2)...)
	s3 = append(s3, sm32(0)...)
	s3 = append(s3, sm32(0)...)
	var s4 []byte
	s4 = append(s4, u16(0)...) // no coordinate values
	s4 = append(s4, u16(0)...) // template 4.0
	s4 = append(s4, category, number, 2, 0, 0)
	s4 = append(s4, u16(0)...)
	s4 = append(s4, 0, 1) // hours
	s4 = append(s4, u32(uint32(lead))...)
	s4 = append(s4, 103, 0) // height above ground, scale 0
	s4 = append(s4, u32(level)...)
	s4 = append(s4, 255, 0, 0, 0, 0, 0)
	var s5 []byte
	s5 = append(s5, u32(uint32(g.Nx*g.Ny))...)
	s5 = append(s5, u16(0)...) // simple packing
	s5 = append(s5, u32(math.Float32bits(-500))...)
	s5 = append(s5, 0, 0, 0, 1) // E = 0, D = 1
	s5 = append(s5, 16, 0)
	var s7 []byte
	for j := 0; j < g.Ny; j++ {
		for i := 0; i < g.Nx; i++ {
			s7 = append(s7, u16(int(math.Round(value(i, j)*10+500)))...)
		}
	}
	body := append(sec(1, s1), sec(3, s3)...)
	body = append(body, sec(4, s4)...)
	body = append(body, sec(5, s5)...)
	body = append(body, sec(6, []byte{255})...)
	body = append(body, sec(7, s7)...)
	body = append(body, "7777"...)
	head := []byte{'G', 'R', 'I', 'B', 0, 0, 0, 2}
	head = binary.BigEndian.AppendUint64(head, uint64(16+len(body)))
	return append(head, body...)
}

func windFileFor(t *testing.T, dir string, ref time.Time, lead int, u, v float64) string {
	t.Helper()
	var b []byte
	b = append(b, gribMessage(ref, lead, 0, 0, 2, func(i, j int) float64 { return 25 })...)
	b = append(b, gribMessage(ref, lead, 2, 2, 10, func(i, j int) float64 { return u })...)
	b = append(b, gribMessage(ref, lead, 2, 3, 10, func(i, j int) float64 { return v })...)
	path := filepath.Join(dir, ref.Format("0215")+"-"+string(rune('a'+lead))+".grb2")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseGrib(t *testing.T) {
	ref := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	msg := gribMessage(ref, 6, 2, 2, 10, func(i, j int) float64 { return float64(i) - float64(j)/10 })
	f, err := parseGrib(msg, false)
	if err != nil {
		t.Fatal(err)
	}
	if !isWind(f, 2) || f.Lead != 6 || !f.RefTime.Equal(ref) || f.Grid != testGrid {
		t.Errorf("header %+v", f)
	}
	if v := f.Values[3*testGrid.Nx+7]; math.Abs(v-6.7) > 1e-9 {
		t.Errorf("value (7, 3) = %v", v)
	}
	h, err := parseGrib(msg[:windHeader], true)
	if err != nil || !isWind(h, 2) || h.Values != nil {
		t.Errorf("header only: %+v, %v", h, err)
	}
	if _, err := parseGrib(msg[:windHeader], false); err == nil {
		t.Error("a cut message must not decode")
	}
}

func TestLambertRoundTrip(t *testing.T) {
	pr := newLambertProj(&testGrid)
	i, j := pr.index(25.03, 121.56)
	lat, lon := pr.latLon(i, j)
	if math.Abs(lat-25.03) > 1e-6 || math.Abs(lon-121.56) > 1e-6 {
		t.Errorf("round trip %v %v", lat, lon)
	}
	if lat, lon := pr.latLon(0, 0); math.Abs(lat-testGrid.La1) > 1e-6 || math.Abs(lon-testGrid.Lo1) > 1e-6 {
		t.Errorf("first point %v %v", lat, lon)
	}
}

func TestEarthWind(t *testing.T) {
	g := testGrid
	g.EarthRelUV = false
	pr := newLambertProj(&g)
	if u, v := pr.earthWind(3, 4, 120); math.Abs(u-3) > 1e-9 || math.Abs(v-4) > 1e-9 {
		t.Errorf("central meridian %v %v", u, v)
	}
	u, v := pr.earthWind(0, 10, 150)
	if u <= 0 || v <= 0 || math.Abs(math.Hypot(u, v)-10) > 1e-9 {
		t.Errorf("east of the centre %v %v", u, v)
	}
}

func TestRenderWind(t *testing.T) {
	ref := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	u, _ := parseGrib(gribMessage(ref, 0, 2, 2, 10, func(i, j int) float64 { return 5 }), false)
	v, _ := parseGrib(gribMessage(ref, 0, 2, 3, 10, func(i, j int) float64 { return -3 }), false)
	data, w, h, err := RenderWind(u, v)
	if err != nil {
		t.Fatal(err)
	}
	if w != 201 || h != 141 {
		t.Errorf("size %d×%d", w, h)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, _, a := img.At(86, 60).RGBA() // 121.5°E, 25°N
	if r>>8 != 140 || g>>8 != 119 || a>>8 != 255 {
		t.Errorf("pixel %d %d %d", r>>8, g>>8, a>>8)
	}
}

func TestWindRun(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	run1 := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/M-000.grb2": windFileFor(t, dir, run1, 0, 5, -3),
		"/M-006.grb2": windFileFor(t, dir, run1, 6, 6, -3),
	})
	images := filepath.Join(dir, "images")
	p := &WindPipeline{URL: srv.URL + "/M-%03d.grb2", Dir: images}
	ctx := context.Background()
	now := run1.Add(5 * time.Hour)
	res, err := p.Run(ctx, srv.Env(db, now))
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsInserted != 2 || len(res.Warnings) != 1 || !res.LastObservedAt.Equal(run1) {
		t.Errorf("result %+v", res)
	}
	for _, name := range []string{"20261022T1800Z-f00.png", "20261023T0000Z-f06.png"} {
		if _, err := os.Stat(filepath.Join(images, "wind_layer", name)); err != nil {
			t.Error(err)
		}
	}
	if res, err := p.Run(ctx, srv.Env(db, now)); err != nil || res.RowsInserted != 0 {
		t.Errorf("second run %+v, %v", res, err)
	}

	run2 := run1.Add(6 * time.Hour)
	srv.Set("/M-000.grb2", windFileFor(t, dir, run2, 0, 7, -3))
	srv.Fail("/M-006.grb2", 404)
	res, err = p.Run(ctx, srv.Env(db, run2.Add(5*time.Hour)))
	if err != nil || res.RowsInserted != 1 {
		t.Fatalf("third run %+v, %v", res, err)
	}
	var file string
	if err := db.QueryRow(`SELECT file FROM data.weather_image WHERE product = 'wind_layer' AND observed_at = $1`, run2).Scan(&file); err != nil || file != "wind_layer/20261023T0000Z-f00.png" {
		t.Errorf("file %q, %v", file, err)
	}
	if _, err := os.Stat(filepath.Join(images, "wind_layer", "20261023T0000Z-f06.png")); !os.IsNotExist(err) {
		t.Error("the replaced frame is removed")
	}
}
