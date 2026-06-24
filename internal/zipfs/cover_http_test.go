package zipfs

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"imgproxy_plus/internal/config"
)

// makeCbz builds a real cbz on disk with the given entries (1x1 PNG pixels).
func makeCbz(t *testing.T, path string, entries map[string]color.RGBA) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		var pb bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		png.Encode(&pb, img)
		w.Write(pb.Bytes())
	}
	zw.Close()
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, buf.Bytes(), 0644)
}

func TestCoverFallbackHTTP(t *testing.T) {
	dir := t.TempDir()
	cbz := filepath.Join(dir, "gallery.cbz")
	makeCbz(t, cbz, map[string]color.RGBA{
		"010_page.png":  {},
		"002_page.png":  {},
		"005_cover.png": {},
	})

	cfg := &config.Config{DataRoot: dir}
	cfg.ZipExts = map[string]bool{"zip": true, "cbz": true}
	h := Handler(cfg)

	// Missing __cover.jfif -> falls back to 005_cover.png (HTTP 200).
	req := httptest.NewRequest(http.MethodGet, "/zip/gallery.cbz/__cover.jfif", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback cover: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Non-existent NON-cover file -> stays 404.
	req2 := httptest.NewRequest(http.MethodGet, "/zip/gallery.cbz/missing_page.png", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("non-cover 404: expected 404, got %d", rec2.Code)
	}
}

func TestCoverQueryParam(t *testing.T) {
	dir := t.TempDir()
	cbz := filepath.Join(dir, "g.cbz")
	makeCbz(t, cbz, map[string]color.RGBA{
		"__cover.jfif": {},
		"002_page.png": {},
	})

	cfg := &config.Config{DataRoot: dir}
	cfg.ZipExts = map[string]bool{"zip": true, "cbz": true}
	h := Handler(cfg)

	// ?cover=1 (no innerPath) -> 200 cover.
	req := httptest.NewRequest(http.MethodGet, "/zip/g.cbz?cover=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("?cover=1: expected 200, got %d", rec.Code)
	}

	// Bare cbz without cover param -> 404 (no implicit cover).
	req2 := httptest.NewRequest(http.MethodGet, "/zip/g.cbz", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("bare cbz without cover param: expected 404, got %d", rec2.Code)
	}

	// Empty archive ?cover=1 -> 404.
	emptyCbz := filepath.Join(dir, "empty.cbz")
	makeCbz(t, emptyCbz, map[string]color.RGBA{})
	h2 := Handler(&config.Config{DataRoot: dir, ZipExts: map[string]bool{"zip": true, "cbz": true}})
	req3 := httptest.NewRequest(http.MethodGet, "/zip/empty.cbz?cover=1", nil)
	rec3 := httptest.NewRecorder()
	h2.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("empty ?cover=1: expected 404, got %d", rec3.Code)
	}
}
