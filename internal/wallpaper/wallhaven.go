package wallpaper

import (
	"context"
	"fmt"

	"github.com/jkrauska/frame-it/internal/wallhaven"
)

func fetchWallhaven(ctx context.Context, opts Options) (Image, error) {
	client := wallhaven.NewClient(opts.WallhavenKey)

	if opts.ID != "" {
		wp, err := client.Get(ctx, opts.ID)
		if err != nil {
			return Image{}, err
		}
		return wallhavenImage(wp), nil
	}

	sort := opts.Sort
	if sort == "" {
		sort = "random"
	}
	results, _, err := client.Search(ctx, wallhaven.SearchOptions{
		Query:      opts.Query,
		Sorting:    sort,
		AtLeast:    "3840x2160",
		Ratios:     "16x9",
		Categories: "100",
		Purity:     "100",
	})
	if err != nil {
		return Image{}, err
	}
	if len(results) == 0 {
		return Image{}, fmt.Errorf("no matching wallhaven wallpapers")
	}
	return wallhavenImage(results[0]), nil
}

func wallhavenImage(wp wallhaven.Wallpaper) Image {
	return Image{
		Source:      string(SourceWallhaven),
		ID:          wp.ID,
		PageURL:     wp.URL,
		Resolution:  wp.Resolution,
		DownloadURL: wp.Path,
		FileType:    wp.FileType,
	}
}
