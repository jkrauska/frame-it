package gallery

import (
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jkrauska/frame-it/internal/archive"
)

func TestParseName(t *testing.T) {
	t.Parallel()

	when, source, id, ok := parseName("20260824-114500-unsplash-ZVbv1akA-l4.jpg")
	if !ok {
		t.Fatal("parseName failed on a well-formed archive name")
	}
	if source != "unsplash" {
		t.Fatalf("source = %q, want unsplash", source)
	}
	if id != "ZVbv1akA-l4" { // ids may contain dashes
		t.Fatalf("id = %q, want ZVbv1akA-l4", id)
	}
	if got := when.Format("2006-01-02 15:04:05"); got != "2026-08-24 11:45:00" {
		t.Fatalf("when = %s, want 2026-08-24 11:45:00", got)
	}

	if _, _, _, ok := parseName("not-an-archive-name.jpg"); ok {
		t.Fatal("parseName accepted a non-archive name")
	}
}

func TestListNewestFirstWithLimit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"20260824-090000-unsplash-aaa.jpg",
		"20260824-100000-unsplash-bbb.jpg",
		"20260824-110000-unsplash-ccc.jpg",
	} {
		writeJPEG(t, filepath.Join(dir, name))
	}
	// Non-images and sidecars must not become entries.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := List(dir, 2, time.UTC)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].ID != "ccc" || entries[1].ID != "bbb" {
		t.Fatalf("got %q, %q; want newest first (ccc, bbb)", entries[0].ID, entries[1].ID)
	}
	if entries[0].HasMeta {
		t.Fatal("entry without a sidecar reported HasMeta")
	}
	if entries[0].DateStamp == "" {
		t.Fatal("entry without a sidecar should fall back to a derived date stamp")
	}
}

func TestListUsesSidecarMetadata(t *testing.T) {
	dir := t.TempDir()
	name := "20260824-112149-unsplash-d3pTF3r.jpg"
	path := filepath.Join(dir, name)
	writeJPEG(t, path)

	meta := archive.Meta{
		File:        name,
		ArchivedAt:  time.Date(2026, 8, 24, 11, 21, 49, 0, time.UTC),
		Source:      "unsplash",
		ID:          "d3pTF3r",
		Query:       "mountains",
		Caption:     "mountains covered with snow · Erol Ahmed",
		DateStamp:   "Monday, August 24th - 11am",
		Description: "mountains covered with snow",
	}
	if err := archive.WriteMeta(path, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	entries, err := List(dir, 10, time.UTC)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (sidecar must not be listed as an image)", len(entries))
	}
	e := entries[0]
	if !e.HasMeta || e.Caption != meta.Caption || e.DateStamp != meta.DateStamp || e.Query != "mountains" {
		t.Fatalf("sidecar metadata not applied: %+v", e)
	}
}

func TestHandlerServesGridAndThumb(t *testing.T) {
	dir := t.TempDir()
	name := "20260824-114500-unsplash-abc.jpg"
	writeJPEG(t, filepath.Join(dir, name))

	h := Handler(Options{Dir: dir, Limit: 5, Loc: time.UTC})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), name) {
		t.Fatal("index does not mention the archived image")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/thumb/"+name, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("thumb status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("thumb content-type = %q", ct)
	}
}

func TestHandlerRejectsPathsOutsideDir(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "20260824-114500-unsplash-abc.jpg"))

	secret := filepath.Join(t.TempDir(), "secret.jpg")
	writeJPEG(t, secret)

	h := Handler(Options{Dir: dir, Limit: 5, Loc: time.UTC})
	for _, path := range []string{
		"/full/" + filepath.Base(secret),
		"/full/%2e%2e%2fsecret.jpg",
		"/thumb/sub/dir/img.jpg",
		"/full/20260824-114500-unsplash-abc.json",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Fatalf("%s was served, want rejection", path)
		}
	}
}

func writeJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for y := 0; y < 36; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 60, B: 90, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
}
