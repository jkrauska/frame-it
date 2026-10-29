// Package gallery serves a local, Lightroom-style contact sheet of the images
// most recently sent to the TV, with the overlay text they were shown with.
package gallery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jkrauska/frame-it/internal/archive"
	"github.com/jkrauska/frame-it/internal/overlay"
)

// DefaultLimit is the number of recent images shown when none is configured.
const DefaultLimit = 20

// Entry is one archived image plus the metadata shown beneath it.
type Entry struct {
	Name        string // base filename, used in thumb/full URLs
	Path        string
	When        time.Time
	Source      string
	ID          string
	Query       string
	PageURL     string
	Resolution  string
	Credit      string
	Description string
	Caption     string // bottom-left overlay text
	DateStamp   string // bottom-right overlay text
	SizeBytes   int64
	HasMeta     bool
}

// List returns the newest limit images in dir, newest first. Entries with a
// metadata sidecar carry the real overlay text; older ones fall back to what the
// archive filename encodes (timestamp, source, id).
func List(dir string, limit int, loc *time.Location) ([]Entry, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if loc == nil {
		loc = time.Local
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read images dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !isImage(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}

	sort.Sort(sort.Reverse(sort.StringSlice(names))) // newest first (timestamp prefix)
	if len(names) > limit {
		names = names[:limit]
	}

	out := make([]Entry, 0, len(names))
	for _, name := range names {
		out = append(out, newEntry(dir, name, loc))
	}
	return out, nil
}

func newEntry(dir, name string, loc *time.Location) Entry {
	path := filepath.Join(dir, name)
	e := Entry{Name: name, Path: path}

	when, source, id, parsed := parseName(name)
	if parsed {
		e.When, e.Source, e.ID = when.In(loc), source, id
	}

	if info, err := os.Stat(path); err == nil {
		e.SizeBytes = info.Size()
		if !parsed {
			e.When = info.ModTime().In(loc)
		}
	}

	if meta, ok := archive.ReadMeta(path); ok {
		e.HasMeta = true
		e.Query, e.PageURL, e.Resolution = meta.Query, meta.PageURL, meta.Resolution
		e.Credit, e.Description = meta.Credit, meta.Description
		e.Caption, e.DateStamp = meta.Caption, meta.DateStamp
		if meta.Source != "" {
			e.Source = meta.Source
		}
		if meta.ID != "" {
			e.ID = meta.ID
		}
		if !meta.ArchivedAt.IsZero() {
			e.When = meta.ArchivedAt.In(loc)
		}
	} else if !e.When.IsZero() {
		// No sidecar: show the date stamp this image would have been given.
		e.DateStamp = overlay.FormatDate(e.When, loc)
	}
	return e
}

// parseName pulls the timestamp, source, and id out of an archive filename of the
// form "20060102-150405-<source>-<id><ext>".
func parseName(name string) (when time.Time, source, id string, ok bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.SplitN(base, "-", 4) // date, time, source, id (ids may contain "-")
	if len(parts) < 3 {
		return time.Time{}, "", "", false
	}

	when, err := time.ParseInLocation("20060102-150405", parts[0]+"-"+parts[1], time.Local)
	if err != nil {
		return time.Time{}, "", "", false
	}
	if len(parts) == 4 {
		id = parts[3]
	}
	return when, parts[2], id, true
}

func isImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

// Clock renders the archive time shown on each tile, e.g. "Mon Aug 24, 11:45am".
func (e Entry) Clock() string {
	if e.When.IsZero() {
		return ""
	}
	return e.When.Format("Mon Jan 2, 3:04pm")
}

// Size renders the file size, e.g. "2.4 MB".
func (e Entry) Size() string {
	switch {
	case e.SizeBytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(e.SizeBytes)/(1<<20))
	case e.SizeBytes > 0:
		return fmt.Sprintf("%.0f KB", float64(e.SizeBytes)/(1<<10))
	default:
		return ""
	}
}
