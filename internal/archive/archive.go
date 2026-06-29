// Package archive keeps a rolling collection of the most recent pure (pre-resize,
// pre-overlay) downloaded images on disk.
package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultDir is the archive directory used when none is configured.
const DefaultDir = "images"

// DefaultKeep is the number of images retained when none is configured.
const DefaultKeep = 100

// Filename builds a chronologically sortable archive filename of the form
// "<timestamp>-<source>-<id><ext>", sanitizing source and id for the filesystem.
func Filename(when time.Time, source, id, ext string) string {
	name := when.Format("20060102-150405") + "-" + sanitize(source) + "-" + sanitize(id)
	return name + ext
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
	s = strings.Trim(s, "-")
	if s == "" {
		return "image"
	}
	return s
}

// Store copies srcPath into dir as filename, then prunes the directory to the
// newest keep files. A keep of 0 disables archiving and Store does nothing.
// It returns the saved path (empty when archiving is disabled).
func Store(dir, filename, srcPath string, keep int) (string, error) {
	if keep <= 0 {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create archive dir: %w", err)
	}

	destPath := filepath.Join(dir, filename)
	if err := copyFile(srcPath, destPath); err != nil {
		return "", err
	}

	if err := prune(dir, keep); err != nil {
		// The copy succeeded; a prune failure shouldn't fail the whole operation.
		return destPath, fmt.Errorf("prune archive: %w", err)
	}
	return destPath, nil
}

func copyFile(srcPath, destPath string) error {
	src, err := os.Open(filepath.Clean(srcPath))
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer func() { _ = dest.Close() }()

	if _, err := io.Copy(dest, src); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("write archive file: %w", err)
	}
	return dest.Close()
}

// prune deletes the oldest files so that at most keep remain. Files are ordered
// by name, which is chronological because Store names them with a timestamp prefix.
func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) <= keep {
		return nil
	}

	sort.Strings(names) // oldest first
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
