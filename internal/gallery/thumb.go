package gallery

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png" // decode PNG archives
	"os"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

const (
	thumbWidth   = 900
	thumbQuality = 82
)

type thumbKey struct {
	path string
	mod  time.Time
	size int64
}

// thumbCache holds downscaled JPEGs in memory so scrolling the grid doesn't
// re-decode 4K originals. It is keyed by path plus mtime/size, so a replaced
// file is re-rendered rather than served stale.
type thumbCache struct {
	mu   sync.Mutex
	data map[thumbKey][]byte
}

func newThumbCache() *thumbCache {
	return &thumbCache{data: make(map[thumbKey][]byte)}
}

// Get returns a JPEG thumbnail for path, rendering and caching it on first use.
func (c *thumbCache) Get(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := thumbKey{path: path, mod: info.ModTime(), size: info.Size()}

	c.mu.Lock()
	cached, ok := c.data[key]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}

	data, err := renderThumb(path)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.data[key] = data
	c.mu.Unlock()
	return data, nil
}

func renderThumb(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	b := src.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil, fmt.Errorf("empty image %s", path)
	}

	w, h := b.Dx(), b.Dy()
	if w > thumbWidth {
		h = h * thumbWidth / w
		w = thumbWidth
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
