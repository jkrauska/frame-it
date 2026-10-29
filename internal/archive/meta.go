package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MetaExt is the extension of the JSON sidecar written next to an archived image.
const MetaExt = ".json"

// Meta records what was known about an image when it was archived, including the
// overlay text drawn on the copy sent to the TV. It is stored as a JSON sidecar
// so the gallery can show the same caption and date stamp later.
type Meta struct {
	File        string    `json:"file"`
	ArchivedAt  time.Time `json:"archived_at"`
	Source      string    `json:"source,omitempty"`
	ID          string    `json:"id,omitempty"`
	Query       string    `json:"query,omitempty"`
	PageURL     string    `json:"page_url,omitempty"`
	Resolution  string    `json:"resolution,omitempty"`
	Credit      string    `json:"credit,omitempty"`
	Description string    `json:"description,omitempty"`
	Caption     string    `json:"caption,omitempty"`    // bottom-left overlay, empty when --no-title
	DateStamp   string    `json:"date_stamp,omitempty"` // bottom-right overlay, empty when --no-date
}

// MetaPath returns the sidecar path for an archived image path.
func MetaPath(imagePath string) string {
	return strings.TrimSuffix(imagePath, filepath.Ext(imagePath)) + MetaExt
}

// IsMeta reports whether a filename is a sidecar rather than an archived image.
func IsMeta(name string) bool {
	return strings.EqualFold(filepath.Ext(name), MetaExt)
}

// WriteMeta stores meta as the JSON sidecar for imagePath.
func WriteMeta(imagePath string, meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	if err := os.WriteFile(MetaPath(imagePath), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// ReadMeta loads the sidecar for imagePath. It reports ok=false when there is no
// sidecar (images archived before metadata was recorded) or it cannot be parsed.
func ReadMeta(imagePath string) (meta Meta, ok bool) {
	data, err := os.ReadFile(MetaPath(imagePath))
	if err != nil {
		return Meta{}, false
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, false
	}
	return meta, true
}
