package api

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
)

func testPNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0x80
		if i%4 == 3 {
			img.Pix[i] = alpha
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestEncodeAppImage(t *testing.T) {
	_, name, err := encodeAppImage(testPNG(t, 200, 100, 0xff))
	if err != nil || !strings.HasSuffix(name, ".jpg") || !appImageName.MatchString(name) {
		t.Errorf("opaque: %q %v", name, err)
	}
	_, name2, err := encodeAppImage(testPNG(t, 200, 100, 0x40))
	if err != nil || !strings.HasSuffix(name2, ".png") {
		t.Errorf("transparent: %q %v", name2, err)
	}
	if _, again, _ := encodeAppImage(testPNG(t, 200, 100, 0xff)); again != name {
		t.Errorf("names differ: %s %s", name, again)
	}

	out, _, err := encodeAppImage(testPNG(t, 2000, 500, 0xff))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 960 || cfg.Height != 240 {
		t.Errorf("shrunk to %dx%d (%v), want 960x240", cfg.Width, cfg.Height, err)
	}

	var g bytes.Buffer
	pal := image.NewPaletted(image.Rect(0, 0, 4, 4), []color.Color{color.Black, color.White})
	if err := gif.Encode(&g, pal, nil); err != nil {
		t.Fatal(err)
	}
	for label, data := range map[string][]byte{
		"text":      []byte("<svg onload=alert(1)>"),
		"gif":       g.Bytes(),
		"truncated": testPNG(t, 50, 50, 0xff)[:60],
	} {
		if _, _, err := encodeAppImage(data); err != errBadImage {
			t.Errorf("%s: %v, want errBadImage", label, err)
		}
	}
}

func TestReadAppImageLimit(t *testing.T) {
	if _, ok := readAppImage(bytes.NewReader(make([]byte, MaxAppImageBytes+1))); ok {
		t.Error("accepted a file over the limit")
	}
	if _, ok := readAppImage(bytes.NewReader(nil)); ok {
		t.Error("accepted an empty file")
	}
}

func TestShowcaseImageUpload(t *testing.T) {
	e := newOpenEnv(t)
	dir := t.TempDir()
	e.router = NewRouter(Deps{DB: fakePinger{}, ImageDir: dir, Auth: AuthDeps{
		Store: e.store, Providers: auth.Providers(false, false), Origins: []string{testOrigin},
	}})
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	admin := e.signInAs(t, auth.RoleAdmin, "active")

	e.newClient(t, a, "x")
	mine := decode[[]myApp](t, e.send("POST", "/api/v1/me/apps", a, validApp))
	id := strconv.FormatInt(mine[0].ID, 10)
	upload := func(session string, data []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/api/v1/me/apps/"+id+"/image", bytes.NewReader(data))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("Origin", testOrigin)
		req.AddCookie(&http.Cookie{Name: "dtw_session", Value: session})
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, req)
		return w
	}

	if w := upload(a, []byte("not an image")); w.Code != http.StatusBadRequest {
		t.Errorf("bad file: %d, want 400", w.Code)
	}
	if w := upload(a, make([]byte, MaxAppImageBytes+1)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("big file: %d, want 413", w.Code)
	}
	if w := upload(b, testPNG(t, 100, 100, 0xff)); w.Code != http.StatusNotFound {
		t.Errorf("B uploads to A's app: %d, want 404", w.Code)
	}
	files, _ := os.ReadDir(filepath.Join(dir, appImageSubdir))
	if len(files) != 0 {
		t.Errorf("B's upload left %d files", len(files))
	}

	w := upload(a, testPNG(t, 100, 100, 0xff))
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	first := decode[[]myApp](t, w)[0].Image
	if _, err := os.Stat(filepath.Join(dir, appImageSubdir, first)); err != nil {
		t.Fatalf("image not stored: %v", err)
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "approve"}); w.Code != http.StatusNoContent {
		t.Fatal(w.Body)
	}
	pub := decode[[]publicApp](t, e.send("GET", "/api/v1/apps", "", nil))
	if len(pub) != 1 || pub[0].ImageURL == nil || *pub[0].ImageURL != "/api/v1/app-images/"+first {
		t.Fatalf("public image: %+v", pub)
	}
	if w := e.send("GET", *pub[0].ImageURL, "", nil); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("serve image: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := e.send("GET", "/api/v1/app-images/..%2F..%2Fsecret.png", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("odd name: %d, want 404", w.Code)
	}

	w = upload(a, testPNG(t, 120, 80, 0xff))
	second := decode[[]myApp](t, w)[0].Image
	if _, err := os.Stat(filepath.Join(dir, appImageSubdir, first)); err != nil {
		t.Error("the published image was deleted while the new one waits for review")
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "approve"}); w.Code != http.StatusNoContent {
		t.Fatal(w.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, appImageSubdir, first)); !os.IsNotExist(err) {
		t.Error("the replaced image was kept")
	}

	if w := e.send("DELETE", "/api/v1/me/apps/"+id, a, nil); w.Code != http.StatusNoContent {
		t.Fatal(w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, appImageSubdir, second)); !os.IsNotExist(err) {
		t.Error("the image of a withdrawn app was kept")
	}
}
