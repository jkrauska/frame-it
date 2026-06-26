package wallpaper

import (
	"testing"
)

func TestDefaultSource(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	if got := DefaultSource(); got != SourceWallhaven {
		t.Fatalf("DefaultSource() = %q, want wallhaven", got)
	}

	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	if got := DefaultSource(); got != SourceUnsplash {
		t.Fatalf("DefaultSource() = %q, want unsplash", got)
	}
}

func TestResolveSource(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")

	src, err := ResolveSource("")
	if err != nil {
		t.Fatalf("ResolveSource empty: %v", err)
	}
	if src != SourceUnsplash {
		t.Fatalf("ResolveSource empty = %q, want unsplash", src)
	}

	src, err = ResolveSource("pixabay")
	if err != nil {
		t.Fatalf("ResolveSource pixabay: %v", err)
	}
	if src != SourcePixabay {
		t.Fatalf("ResolveSource pixabay = %q, want pixabay", src)
	}

	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	src, err = ResolveSource("")
	if err != nil {
		t.Fatalf("ResolveSource empty/no key: %v", err)
	}
	if src != SourceWallhaven {
		t.Fatalf("ResolveSource empty/no key = %q, want wallhaven", src)
	}
}
