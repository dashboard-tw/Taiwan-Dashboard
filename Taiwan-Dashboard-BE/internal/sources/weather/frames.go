package weather

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type Frame struct {
	Product    string
	ObservedAt time.Time
	Ext        string // .jpg or .png
	Data       []byte
	Width      int
	Height     int
	Bounds     *[4]float64
	Name       string
}

func FrameFile(t time.Time, ext string) string { return t.UTC().Format("20060102T1504Z") + ext }

func frameExists(ctx context.Context, env pipeline.Env, product string, t time.Time) (bool, error) {
	var exists bool
	err := env.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM data.weather_image WHERE product = $1 AND observed_at = $2)`,
		product, t).Scan(&exists)
	return exists, err
}

func saveFrame(ctx context.Context, env pipeline.Env, root string, f Frame) error {
	dir := filepath.Join(root, f.Product)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := f.Name
	if name == "" {
		name = FrameFile(f.ObservedAt, f.Ext)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(f.Data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	var w, s, e, n any
	if b := f.Bounds; b != nil {
		w, s, e, n = b[0], b[1], b[2], b[3]
	}
	sum := sha256.Sum256(f.Data)
	_, err = env.DB.ExecContext(ctx, `
		INSERT INTO data.weather_image (product, observed_at, file, width, height, bytes, sha256, west, south, east, north)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (product, observed_at) DO NOTHING`,
		f.Product, f.ObservedAt, f.Product+"/"+name, f.Width, f.Height, len(f.Data), hex.EncodeToString(sum[:]),
		w, s, e, n)
	return err
}

func pruneFrames(ctx context.Context, env pipeline.Env, root, product string) error {
	cutoff := env.Now().Add(-ImageRetention)
	rows, err := env.DB.QueryContext(ctx,
		`SELECT file FROM data.weather_image WHERE product = $1 AND observed_at < $2`, product, cutoff)
	if err != nil {
		return err
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return err
		}
		files = append(files, f)
	}
	rows.Close()
	for _, f := range files {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(f))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err = env.DB.ExecContext(ctx,
		`DELETE FROM data.weather_image WHERE product = $1 AND observed_at < $2`, product, cutoff)
	return err
}
