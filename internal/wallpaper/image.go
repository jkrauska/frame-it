package wallpaper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCaptionRunes = 80

// Image is a wallpaper from any supported source.
type Image struct {
	Source      string
	ID          string
	PageURL     string
	Resolution  string
	DownloadURL string
	FileType    string
	Credit      string
	Description string
}

// Caption returns bottom-left overlay text: "description · author".
func (img Image) Caption() string {
	desc := strings.TrimSpace(img.Description)
	author := authorName(img.Credit)
	return formatCaption(desc, author)
}

func authorName(credit string) string {
	credit = strings.TrimSpace(credit)
	if credit == "" {
		return ""
	}
	if i := strings.Index(credit, " ("); i > 0 {
		return strings.TrimSpace(credit[:i])
	}
	return credit
}

func formatCaption(description, author string) string {
	if utf8.RuneCountInString(description) > maxCaptionRunes {
		description = "..."
	}
	switch {
	case description != "" && author != "":
		return description + " · " + author
	case description != "":
		return description
	case author != "":
		return author
	default:
		return ""
	}
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

// Download saves the image to destPath.
func Download(ctx context.Context, img Image, destPath string) error {
	if img.DownloadURL == "" {
		return fmt.Errorf("%s wallpaper %s has no download URL", img.Source, img.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.DownloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
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

// TempFile creates a temp file with an extension derived from the image metadata.
func TempFile(img Image) (*os.File, error) {
	ext := ".jpg"
	switch {
	case strings.Contains(img.FileType, "png"):
		ext = ".png"
	case strings.Contains(img.FileType, "jpeg"), strings.Contains(img.FileType, "jpg"):
		ext = ".jpg"
	default:
		if strings.HasSuffix(strings.ToLower(img.DownloadURL), ".png") {
			ext = ".png"
		}
	}
	return os.CreateTemp("", "frame-it-"+img.Source+"-*"+ext)
}
