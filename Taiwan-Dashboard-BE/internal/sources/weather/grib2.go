package weather

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

type gribField struct {
	Discipline uint8
	Category   uint8 // product template 4.0: parameter category
	Number     uint8 // parameter number
	LevelType  uint8 // type of first fixed surface; 103 is height above ground
	Level      float64
	RefTime    time.Time
	Lead       int
	Grid       lambertGrid
	Values     []float64
}

type lambertGrid struct {
	Nx, Ny       int
	La1, Lo1     float64 // first grid point, degrees
	LaD, LoV     float64 // latitude where Dx, Dy apply; orientation longitude
	Dx, Dy       float64 // metres
	Latin1       float64
	Latin2       float64
	ScanMode     uint8
	EarthRelUV   bool // false: u and v are along the grid's x and y axes
	EarthRadiusM float64
}

var errGribShort = errors.New("GRIB2 訊息不完整")

func gribHeader(b []byte) (int64, error) {
	if len(b) < 16 || string(b[:4]) != "GRIB" {
		return 0, errors.New("不是 GRIB 訊息")
	}
	if b[7] != 2 {
		return 0, fmt.Errorf("GRIB 版本 %d，只支援第 2 版", b[7])
	}
	return int64(binary.BigEndian.Uint64(b[8:16])), nil
}

func parseGrib(b []byte, header bool) (*gribField, error) {
	total, err := gribHeader(b)
	if err != nil {
		return nil, err
	}
	if !header && int64(len(b)) < total {
		return nil, errGribShort
	}
	f := &gribField{Discipline: b[6]}
	var (
		refR             float32
		binScale, decScl int
		nbits            int
		packed           = -1
		points           int
		gotGrid, gotProd bool
	)
	p := 16
	for p+5 <= len(b) && int64(p) < total-4 {
		ln := int(binary.BigEndian.Uint32(b[p : p+4]))
		sec := b[p+4]
		if ln < 5 {
			return nil, fmt.Errorf("GRIB2 第 %d 段長度 %d 不正確", sec, ln)
		}
		if p+ln > len(b) {
			if header && gotGrid && gotProd {
				break
			}
			return nil, errGribShort
		}
		s := b[p : p+ln]
		switch sec {
		case 1:
			if ln < 19 {
				return nil, errGribShort
			}
			f.RefTime = time.Date(int(binary.BigEndian.Uint16(s[12:14])), time.Month(s[14]), int(s[15]),
				int(s[16]), int(s[17]), int(s[18]), 0, time.UTC)
		case 3:
			if err := f.Grid.parse(s); err != nil {
				return nil, err
			}
			points = f.Grid.Nx * f.Grid.Ny
			gotGrid = true
		case 4:
			if ln < 28 {
				return nil, errGribShort
			}
			f.Category, f.Number = s[9], s[10]
			f.LevelType, f.Lead = 255, -1
			if t := binary.BigEndian.Uint16(s[7:9]); t < 16 && s[17] == 1 {
				f.Lead = int(binary.BigEndian.Uint32(s[18:22]))
				f.LevelType = s[22]
				f.Level = float64(binary.BigEndian.Uint32(s[24:28])) / math.Pow10(int(int8sm(s[23])))
			}
			gotProd = true
			if header {
				return f, nil
			}
		case 5:
			if ln < 21 {
				return nil, errGribShort
			}
			if t := binary.BigEndian.Uint16(s[9:11]); t != 0 {
				return nil, fmt.Errorf("不支援的壓縮方式 5.%d", t)
			}
			refR = math.Float32frombits(binary.BigEndian.Uint32(s[11:15]))
			binScale = int16sm(s[15:17])
			decScl = int16sm(s[17:19])
			nbits = int(s[19])
		case 6:
			if s[5] != 255 {
				return nil, errors.New("不支援有 bitmap 的欄位")
			}
		case 7:
			packed = p + 5
		}
		p += ln
	}
	if !gotGrid || !gotProd {
		return nil, errors.New("GRIB2 訊息缺少格點或產品定義")
	}
	if packed < 0 {
		return nil, errors.New("GRIB2 訊息沒有資料段")
	}
	if nbits > 32 {
		return nil, fmt.Errorf("每個值 %d 位元，不正確", nbits)
	}
	if need := packed + (points*nbits+7)/8; need > len(b) {
		return nil, errGribShort
	}
	f.Values = make([]float64, points)
	bs, ds := math.Exp2(float64(binScale)), math.Pow10(-decScl)
	data := b[packed:]
	for i := range f.Values {
		var x uint64
		if nbits > 0 {
			x = readBits(data, i*nbits, nbits)
		}
		f.Values[i] = (float64(refR) + float64(x)*bs) * ds
	}
	return f, nil
}

func readBits(b []byte, off, n int) uint64 {
	var v uint64
	for n > 0 {
		byteIdx, bit := off/8, off%8
		take := min(8-bit, n)
		cur := uint64(b[byteIdx]>>(8-bit-take)) & (1<<take - 1)
		v = v<<take | cur
		off += take
		n -= take
	}
	return v
}

func int16sm(b []byte) int {
	v := int(binary.BigEndian.Uint16(b))
	if v&0x8000 != 0 {
		return -(v & 0x7fff)
	}
	return v
}

func int32sm(b []byte) float64 {
	v := binary.BigEndian.Uint32(b)
	if v&0x80000000 != 0 {
		return -float64(v & 0x7fffffff)
	}
	return float64(v)
}

func int8sm(b byte) int {
	if b&0x80 != 0 {
		return -int(b & 0x7f)
	}
	return int(b)
}

func (g *lambertGrid) parse(s []byte) error {
	if len(s) < 73 {
		return errGribShort
	}
	if t := binary.BigEndian.Uint16(s[12:14]); t != 30 {
		return fmt.Errorf("不支援的格點樣板 3.%d（只支援蘭伯特投影 3.30）", t)
	}
	switch s[14] {
	case 0:
		g.EarthRadiusM = 6367470
	case 1:
		g.EarthRadiusM = float64(binary.BigEndian.Uint32(s[16:20])) / math.Pow10(int(s[15]))
	default:
		g.EarthRadiusM = 6371229
	}
	g.Nx = int(binary.BigEndian.Uint32(s[30:34]))
	g.Ny = int(binary.BigEndian.Uint32(s[34:38]))
	g.La1 = int32sm(s[38:42]) / 1e6
	g.Lo1 = float64(binary.BigEndian.Uint32(s[42:46])) / 1e6
	g.EarthRelUV = s[46]&0x08 == 0
	g.LaD = int32sm(s[47:51]) / 1e6
	g.LoV = float64(binary.BigEndian.Uint32(s[51:55])) / 1e6
	g.Dx = float64(binary.BigEndian.Uint32(s[55:59])) / 1e3
	g.Dy = float64(binary.BigEndian.Uint32(s[59:63])) / 1e3
	if s[63]&0x80 != 0 {
		return errors.New("只支援北半球的蘭伯特投影")
	}
	g.ScanMode = s[64]
	if g.ScanMode&0xa0 != 0 {
		return fmt.Errorf("不支援的掃描方式 0x%02x", g.ScanMode)
	}
	g.Latin1 = int32sm(s[65:69]) / 1e6
	g.Latin2 = int32sm(s[69:73]) / 1e6
	if g.Nx < 2 || g.Ny < 2 || g.Nx*g.Ny > 5_000_000 || g.Dx <= 0 || g.Dy <= 0 {
		return fmt.Errorf("格點大小 %d×%d 不正確", g.Nx, g.Ny)
	}
	return nil
}

type lambertProj struct {
	g          *lambertGrid
	n, rf      float64 // cone constant; earth radius × F
	x1, y1     float64 // first grid point
	northwards bool
}

func newLambertProj(g *lambertGrid) *lambertProj {
	rad := math.Pi / 180
	p1, p2 := g.Latin1*rad, g.Latin2*rad
	var n float64
	if math.Abs(p1-p2) < 1e-9 {
		n = math.Sin(p1)
	} else {
		n = math.Log(math.Cos(p1)/math.Cos(p2)) / math.Log(math.Tan(math.Pi/4+p2/2)/math.Tan(math.Pi/4+p1/2))
	}
	f := math.Cos(p1) * math.Pow(math.Tan(math.Pi/4+p1/2), n) / n
	pr := &lambertProj{g: g, n: n, rf: g.EarthRadiusM * f, northwards: g.ScanMode&0x40 != 0}
	pr.x1, pr.y1 = pr.xy(g.La1, g.Lo1)
	return pr
}

func (pr *lambertProj) theta(lon float64) float64 {
	d := math.Mod(lon-pr.g.LoV+540, 360) - 180
	return pr.n * d * math.Pi / 180
}

func (pr *lambertProj) xy(lat, lon float64) (float64, float64) {
	rho := pr.rf / math.Pow(math.Tan(math.Pi/4+lat*math.Pi/360), pr.n)
	t := pr.theta(lon)
	return rho * math.Sin(t), -rho * math.Cos(t)
}

func (pr *lambertProj) index(lat, lon float64) (float64, float64) {
	x, y := pr.xy(lat, lon)
	i := (x - pr.x1) / pr.g.Dx
	j := (y - pr.y1) / pr.g.Dy
	if !pr.northwards {
		j = -j
	}
	return i, j
}

func (pr *lambertProj) latLon(i, j float64) (float64, float64) {
	if !pr.northwards {
		j = -j
	}
	x, y := pr.x1+i*pr.g.Dx, pr.y1+j*pr.g.Dy
	rho := math.Copysign(math.Hypot(x, y), pr.n)
	t := math.Atan2(x, -y)
	lat := 2*math.Atan(math.Pow(pr.rf/rho, 1/pr.n)) - math.Pi/2
	lon := pr.g.LoV + t/pr.n*180/math.Pi
	return lat * 180 / math.Pi, math.Mod(lon+540, 360) - 180
}

func (pr *lambertProj) sample(v []float64, lat, lon float64) float64 {
	fi, fj := pr.index(lat, lon)
	nx, ny := pr.g.Nx, pr.g.Ny
	if fi < 0 || fj < 0 || fi > float64(nx-1) || fj > float64(ny-1) {
		return math.NaN()
	}
	i0, j0 := min(int(fi), nx-2), min(int(fj), ny-2)
	di, dj := fi-float64(i0), fj-float64(j0)
	at := func(i, j int) float64 { return v[j*nx+i] }
	return (at(i0, j0)*(1-di)+at(i0+1, j0)*di)*(1-dj) + (at(i0, j0+1)*(1-di)+at(i0+1, j0+1)*di)*dj
}

func (pr *lambertProj) earthWind(u, v, lon float64) (float64, float64) {
	if pr.g.EarthRelUV {
		return u, v
	}
	t := pr.theta(lon)
	c, s := math.Cos(t), math.Sin(t)
	return c*u + s*v, -s*u + c*v
}
