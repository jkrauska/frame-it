package wallpaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const pixabayBase = "https://pixabay.com/api/"

func fetchPixabay(ctx context.Context, opts Options) (Image, error) {
	if opts.PixabayKey == "" {
		return Image{}, fmt.Errorf("pixabay requires an API key ($PIXABAY_API_KEY or --api-key)")
	}

	params := url.Values{
		"key":          {opts.PixabayKey},
		"image_type":   {"photo"},
		"orientation":  {"horizontal"},
		"category":     {"nature"},
		"min_width":    {"3840"},
		"min_height":   {"2160"},
		"safesearch":   {"true"},
		"per_page":     {"3"},
	}

	if opts.ID != "" {
		params.Set("id", opts.ID)
	} else {
		query := opts.Query
		if query == "" {
			query = "nature"
		}
		params.Set("q", query)
		if opts.Sort == "latest" {
			params.Set("order", "latest")
		}
	}

	var result struct {
		TotalHits int           `json:"totalHits"`
		Hits      []pixabayHit  `json:"hits"`
	}
	if err := pixabayRequest(ctx, params, &result); err != nil {
		return Image{}, err
	}
	if len(result.Hits) == 0 {
		return Image{}, fmt.Errorf("no matching pixabay photos (try a different query)")
	}

	hit := result.Hits[0]
	downloadURL := hit.bestURL()
	if downloadURL == "" {
		return Image{}, fmt.Errorf("pixabay photo %d has no download URL", hit.ID)
	}

	return Image{
		Source:      string(SourcePixabay),
		ID:          fmt.Sprintf("%d", hit.ID),
		PageURL:     hit.PageURL,
		Resolution:  fmt.Sprintf("%dx%d", hit.ImageWidth, hit.ImageHeight),
		DownloadURL: downloadURL,
		FileType:    "image/jpeg",
		Credit:      hit.User,
	}, nil
}

type pixabayHit struct {
	ID           int    `json:"id"`
	PageURL      string `json:"pageURL"`
	ImageWidth   int    `json:"imageWidth"`
	ImageHeight  int    `json:"imageHeight"`
	ImageURL     string `json:"imageURL"`
	FullHDURL    string `json:"fullHDURL"`
	LargeImageURL string `json:"largeImageURL"`
	User         string `json:"user"`
}

func (h pixabayHit) bestURL() string {
	switch {
	case h.ImageURL != "":
		return h.ImageURL
	case h.FullHDURL != "":
		return h.FullHDURL
	default:
		return h.LargeImageURL
	}
}

func pixabayRequest(ctx context.Context, params url.Values, out any) error {
	u, err := url.Parse(pixabayBase)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pixabay request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("pixabay rate limit exceeded (100 req/min)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pixabay HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse pixabay response: %w", err)
	}
	return nil
}
