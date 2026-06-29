package wallpaper

import (
	"testing"
)

func TestDefaultSource(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	if got := DefaultSource(false); got != SourceWallhaven {
		t.Fatalf("DefaultSource(false) = %q, want wallhaven", got)
	}

	// A configured key (no env var) should still prefer Unsplash.
	if got := DefaultSource(true); got != SourceUnsplash {
		t.Fatalf("DefaultSource(true) = %q, want unsplash", got)
	}

	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	if got := DefaultSource(false); got != SourceUnsplash {
		t.Fatalf("DefaultSource() with env key = %q, want unsplash", got)
	}
}

func TestResolveSource(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")

	src, err := ResolveSource("", false)
	if err != nil {
		t.Fatalf("ResolveSource empty: %v", err)
	}
	if src != SourceUnsplash {
		t.Fatalf("ResolveSource empty = %q, want unsplash", src)
	}

	src, err = ResolveSource("pixabay", false)
	if err != nil {
		t.Fatalf("ResolveSource pixabay: %v", err)
	}
	if src != SourcePixabay {
		t.Fatalf("ResolveSource pixabay = %q, want pixabay", src)
	}

	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	src, err = ResolveSource("", false)
	if err != nil {
		t.Fatalf("ResolveSource empty/no key: %v", err)
	}
	if src != SourceWallhaven {
		t.Fatalf("ResolveSource empty/no key = %q, want wallhaven", src)
	}

	// No env var, but a configured key is signalled — prefer Unsplash.
	src, err = ResolveSource("", true)
	if err != nil {
		t.Fatalf("ResolveSource empty/config key: %v", err)
	}
	if src != SourceUnsplash {
		t.Fatalf("ResolveSource empty/config key = %q, want unsplash", src)
	}
}
