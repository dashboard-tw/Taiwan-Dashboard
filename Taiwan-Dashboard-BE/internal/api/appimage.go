package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	MaxAppImageBytes  = 1 << 20
	appImageMaxW      = 960
	appImageMaxH      = 600
	appImageMaxPixels = 5000 * 5000
	appImageSubdir    = "apps"
)

var appImageName = regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpg)$`)

var errBadImage = errors.New("not a supported image")

func encodeAppImage(data []byte) ([]byte, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		return nil, "", errBadImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > appImageMaxPixels {
		return nil, "", errBadImage
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", errBadImage
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > appImageMaxW || h > appImageMaxH {
		scale := min(float64(appImageMaxW)/float64(w), float64(appImageMaxH)/float64(h))
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == b.Dx() && h == b.Dy() {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	}

	var out bytes.Buffer
	ext := ".jpg"
	if hasAlpha(dst) {
		ext = ".png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, dst)
	} else {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(sum[:16]) + ext, nil
}

func hasAlpha(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return true
		}
	}
	return false
}

func readAppImage(r io.Reader) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(r, MaxAppImageBytes+1))
	if err != nil || len(data) > MaxAppImageBytes || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func saveAppImage(dir, name string, data []byte) error {
	folder := filepath.Join(dir, appImageSubdir)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	final := filepath.Join(folder, name)
	if _, err := os.Stat(final); err == nil {
		return nil
	}
	tmp, err := os.CreateTemp(folder, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), final)
}

func removeAppImage(dir, name string) {
	if dir == "" || !appImageName.MatchString(name) {
		return
	}
	_ = os.Remove(filepath.Join(dir, appImageSubdir, name))
}
