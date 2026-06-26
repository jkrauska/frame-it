package wallpaper

import (
	"context"
	"fmt"
	"strings"
)

// Source names a wallpaper provider.
type Source string

const (
	SourceWallhaven Source = "wallhaven"
	SourceUnsplash  Source = "unsplash"
	SourcePixabay   Source = "pixabay"
)

// Options configures a wallpaper fetch.
type Options struct {
	Source Source
	Query  string
	ID     string
	Sort   string // wallhaven only; unsplash/pixabay use random search when empty

	WallhavenKey string
	UnsplashKey  string
	PixabayKey   string
}

// Fetch returns a wallpaper from the configured source.
func Fetch(ctx context.Context, opts Options) (Image, error) {
	switch opts.Source {
	case SourceWallhaven, "":
		return fetchWallhaven(ctx, opts)
	case SourceUnsplash:
		return fetchUnsplash(ctx, opts)
	case SourcePixabay:
		return fetchPixabay(ctx, opts)
	default:
		return Image{}, fmt.Errorf("unknown wallpaper source %q (use wallhaven, unsplash, or pixabay)", opts.Source)
	}
}

func ParseSource(s string) (Source, error) {
	switch Source(strings.ToLower(strings.TrimSpace(s))) {
	case SourceWallhaven, "":
		return SourceWallhaven, nil
	case SourceUnsplash:
		return SourceUnsplash, nil
	case SourcePixabay:
		return SourcePixabay, nil
	default:
		return "", fmt.Errorf("unknown source %q", s)
	}
}

func (s Source) String() string {
	if s == "" {
		return string(SourceWallhaven)
	}
	return string(s)
}

func (s Source) Label() string {
	switch s {
	case SourceUnsplash:
		return "Unsplash"
	case SourcePixabay:
		return "Pixabay"
	default:
		return "Wallhaven"
	}
}
