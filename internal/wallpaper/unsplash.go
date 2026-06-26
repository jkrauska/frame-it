package wallpaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const unsplashBase = "https://api.unsplash.com"

func fetchUnsplash(ctx context.Context, opts Options) (Image, error) {
	if opts.UnsplashKey == "" {
		return Image{}, fmt.Errorf("unsplash requires an API key ($UNSPLASH_ACCESS_KEY or --api-key)")
	}

	var photo unsplashPhoto
	var err error

	if opts.ID != "" {
		photo, err = unsplashGet(ctx, opts.UnsplashKey, "/photos/"+url.PathEscape(opts.ID))
		if err != nil {
			return Image{}, err
		}
	} else {
		query := opts.Query
		if query == "" {
			query = "nature"
		}
		params := url.Values{
			"query":          {query},
			"orientation":    {"landscape"},
			"content_filter": {"high"},
		}

		sort := opts.Sort
		if sort == "" {
			sort = "random"
		}

		if sort == "search" {
			params.Set("per_page", "1")
			var result struct {
				Results []unsplashPhoto `json:"results"`
			}
			if err := unsplashRequest(ctx, opts.UnsplashKey, "/search/photos", params, &result); err != nil {
				return Image{}, err
			}
			if len(result.Results) == 0 {
				return Image{}, fmt.Errorf("no matching unsplash photos")
			}
			photo = result.Results[0]
		} else {
			var photoResult unsplashPhoto
			if err := unsplashRequest(ctx, opts.UnsplashKey, "/photos/random", params, &photoResult); err != nil {
				return Image{}, err
			}
			photo = photoResult
		}
	}

	downloadURL := unsplashDownloadURL(photo.URLs.Raw)
	if downloadURL == "" {
		downloadURL = photo.URLs.Full
	}

	credit := photo.User.Name
	if photo.User.Username != "" {
		credit = credit + " (https://unsplash.com/@" + photo.User.Username + ")"
	}

	return Image{
		Source:      string(SourceUnsplash),
		ID:          photo.ID,
		PageURL:     photo.Links.HTML,
		Resolution:  fmt.Sprintf("%dx%d", photo.Width, photo.Height),
		DownloadURL: downloadURL,
		FileType:    "image/jpeg",
		Credit:      credit,
	}, nil
}

type unsplashPhoto struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URLs   struct {
		Raw  string `json:"raw"`
		Full string `json:"full"`
	} `json:"urls"`
	Links struct {
		HTML string `json:"html"`
	} `json:"links"`
	User struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"user"`
}

func unsplashGet(ctx context.Context, key, path string) (unsplashPhoto, error) {
	var photo unsplashPhoto
	err := unsplashRequest(ctx, key, path, nil, &photo)
	return photo, err
}

func unsplashRequest(ctx context.Context, key, path string, params url.Values, out any) error {
	u, err := url.Parse(unsplashBase + path)
	if err != nil {
		return err
	}
	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Client-ID "+key)
	req.Header.Set("Accept-Version", "v1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unsplash request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("unsplash rate limit exceeded")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unsplash unauthorized — check your API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unsplash HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse unsplash response: %w", err)
	}
	return nil
}

func unsplashDownloadURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("w", strconv.Itoa(3840))
	q.Set("h", strconv.Itoa(2160))
	q.Set("fit", "crop")
	q.Set("fm", "jpg")
	u.RawQuery = q.Encode()
	return u.String()
}
