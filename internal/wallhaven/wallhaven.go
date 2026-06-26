// Package wallhaven provides a minimal client for the Wallhaven API v1.
package wallhaven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const baseURL = "https://wallhaven.cc/api/v1"

// Wallpaper is a search result or detail record from Wallhaven.
type Wallpaper struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Resolution string `json:"resolution"`
	Purity     string `json:"purity"`
	Category   string `json:"category"`
	Path       string `json:"path"`
	FileType   string `json:"file_type"`
}

// Client calls the Wallhaven API.
type Client struct {
	apiKey string
	http   *http.Client
}

// NewClient creates a Wallhaven API client. apiKey is optional for SFW content.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SearchOptions configures a wallpaper search.
type SearchOptions struct {
	Query      string
	Sorting    string // random, toplist, date_added, views, favorites
	AtLeast    string // e.g. 3840x2160
	Ratios     string // e.g. 16x9
	Categories string // 100=general, 101=anime, 111=all
	Purity     string // 100=sfw, 110=sfw+sketchy, 111=all (nsfw needs API key)
	Page       int
	Seed       string // for paginating random results
}

// Search returns wallpapers matching the options.
func (c *Client) Search(ctx context.Context, opts SearchOptions) ([]Wallpaper, string, error) {
	params := url.Values{}
	if opts.Query != "" {
		params.Set("q", opts.Query)
	}
	if opts.Sorting != "" {
		params.Set("sorting", opts.Sorting)
	}
	if opts.AtLeast != "" {
		params.Set("atleast", opts.AtLeast)
	}
	if opts.Ratios != "" {
		params.Set("ratios", opts.Ratios)
	}
	if opts.Categories != "" {
		params.Set("categories", opts.Categories)
	}
	if opts.Purity != "" {
		params.Set("purity", opts.Purity)
	}
	if opts.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	if opts.Seed != "" {
		params.Set("seed", opts.Seed)
	}

	var envelope struct {
		Data []Wallpaper `json:"data"`
		Meta struct {
			Seed string `json:"seed"`
		} `json:"meta"`
	}
	if err := c.get(ctx, "/search", params, &envelope); err != nil {
		return nil, "", err
	}
	return envelope.Data, envelope.Meta.Seed, nil
}

// Get returns details for a wallpaper ID.
func (c *Client) Get(ctx context.Context, id string) (Wallpaper, error) {
	var envelope struct {
		Data Wallpaper `json:"data"`
	}
	if err := c.get(ctx, "/w/"+url.PathEscape(id), nil, &envelope); err != nil {
		return Wallpaper{}, err
	}
	return envelope.Data, nil
}

// Download saves a wallpaper image to destPath.
func (c *Client) Download(ctx context.Context, wp Wallpaper, destPath string) error {
	if wp.Path == "" {
		return fmt.Errorf("wallpaper %s has no download path", wp.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wp.Path, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// TempFile creates a temp file with an extension derived from the wallpaper.
func TempFile(wp Wallpaper) (*os.File, error) {
	ext := ".jpg"
	switch {
	case strings.Contains(wp.FileType, "png"):
		ext = ".png"
	case strings.Contains(wp.FileType, "jpeg"), strings.Contains(wp.FileType, "jpg"):
		ext = ".jpg"
	default:
		if strings.HasSuffix(strings.ToLower(wp.Path), ".png") {
			ext = ".png"
		}
	}
	return os.CreateTemp("", "frame-it-wallhaven-*"+ext)
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	u, err := url.Parse(baseURL + path)
	if err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	if c.apiKey != "" {
		params.Set("apikey", c.apiKey)
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wallhaven request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("wallhaven rate limit exceeded (45 req/min)")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("wallhaven unauthorized — check your API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallhaven HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse wallhaven response: %w", err)
	}
	return nil
}
