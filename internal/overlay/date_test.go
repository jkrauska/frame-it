package overlay

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFormatDate(t *testing.T) {
	t.Parallel()

	loc := time.UTC
	when := time.Date(2026, 6, 26, 12, 0, 0, 0, loc)
	got := FormatDate(when, loc)
	want := "Friday, June 26th"
	if got != want {
		t.Fatalf("FormatDate() = %q, want %q", got, want)
	}
}

func TestOrdinal(t *testing.T) {
	t.Parallel()

	tests := map[int]string{
		1: "st", 2: "nd", 3: "rd", 4: "th",
		11: "th", 12: "th", 13: "th",
		21: "st", 22: "nd", 23: "rd",
	}
	for day, want := range tests {
		if got := ordinal(day); got != want {
			t.Fatalf("ordinal(%d) = %q, want %q", day, got, want)
		}
	}
}

func TestStampDateResizesTo4K(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jpg")
	writeTestJPEG(t, path, color.RGBA{R: 20, G: 20, B: 20, A: 255})

	out := filepath.Join(t.TempDir(), "out.jpg")
	when := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if err := StampDateCopy(path, out, when, time.UTC); err != nil {
		t.Fatalf("StampDateCopy: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Width != targetWidth || cfg.Height != targetHeight {
		t.Fatalf("size = %dx%d, want %dx%d", cfg.Width, cfg.Height, targetWidth, targetHeight)
	}
}

func TestStampDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jpg")
	writeTestJPEG(t, path, color.RGBA{R: 20, G: 20, B: 20, A: 255})

	when := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if err := StampDate(path, when, time.UTC); err != nil {
		t.Fatalf("StampDate: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("stamped file is empty")
	}
}

func TestPrepareUploadPathResizesWithoutDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jpg")
	writeTestJPEG(t, path, color.RGBA{R: 20, G: 20, B: 20, A: 255})

	uploadPath, cleanup, err := PrepareUploadPath(path, false, time.Time{}, nil)
	if err != nil {
		t.Fatalf("PrepareUploadPath: %v", err)
	}
	defer cleanup()

	if uploadPath == path {
		t.Fatal("expected temp resized copy, got original path")
	}

	f, err := os.Open(uploadPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Width != targetWidth || cfg.Height != targetHeight {
		t.Fatalf("size = %dx%d, want %dx%d", cfg.Width, cfg.Height, targetWidth, targetHeight)
	}
}

func writeTestJPEG(t *testing.T, path string, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
}
