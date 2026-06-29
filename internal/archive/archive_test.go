package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilename(t *testing.T) {
	when := time.Date(2026, 6, 28, 14, 15, 2, 0, time.UTC)
	got := Filename(when, "unsplash", "abc 123/xyz", ".jpg")
	want := "20260628-141502-unsplash-abc-123-xyz.jpg"
	if got != want {
		t.Fatalf("Filename = %q, want %q", got, want)
	}
}

func TestFilenameEmptyID(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := Filename(when, "wallhaven", "", ".png")
	want := "20260102-030405-wallhaven-image.png"
	if got != want {
		t.Fatalf("Filename = %q, want %q", got, want)
	}
}

func TestStorePrunesToKeep(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	src := filepath.Join(t.TempDir(), "src.jpg")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Store 5 images, retaining only the newest 3.
	for i := 1; i <= 5; i++ {
		name := Filename(time.Date(2026, 6, 28, 0, 0, i, 0, time.UTC), "src", "id", ".jpg")
		if _, err := Store(dir, name, src, 3); err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("retained %d files, want 3", len(entries))
	}
	// The three newest (seconds 3,4,5) should remain; the oldest (1,2) pruned.
	for _, e := range entries {
		if e.Name() < "20260628-000003" {
			t.Fatalf("old file %q was not pruned", e.Name())
		}
	}
}

func TestStoreDisabledWhenKeepZero(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	src := filepath.Join(t.TempDir(), "src.jpg")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := Store(dir, "x.jpg", src, 0)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if saved != "" {
		t.Fatalf("expected no save when keep=0, got %q", saved)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("archive dir should not be created when disabled")
	}
}

func TestStoreCopiesContents(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	src := filepath.Join(t.TempDir(), "src.jpg")
	want := []byte("original-bytes")
	if err := os.WriteFile(src, want, 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := Store(dir, "out.jpg", src, 10)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("archived contents = %q, want %q", got, want)
	}
}
