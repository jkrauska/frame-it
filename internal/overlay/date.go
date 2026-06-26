package overlay

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const luminanceCutoff = 0.55

// FormatDate renders a date like "Friday, June 26th".
func FormatDate(when time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	t := when.In(loc)
	day := t.Day()
	return fmt.Sprintf("%s, %s %d%s", t.Format("Monday"), t.Format("January"), day, ordinal(day))
}

func ordinal(day int) string {
	if day%100 >= 11 && day%100 <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}

// StampDate resizes to 4K and draws the date on the bottom-right (in place).
func StampDate(path string, when time.Time, loc *time.Location) error {
	return processImage(path, path, true, when, loc)
}

// StampDateCopy writes a resized 4K copy of src to dest with the date overlay.
func StampDateCopy(src, dest string, when time.Time, loc *time.Location) error {
	return processImage(src, dest, true, when, loc)
}

// ResizeCopy writes a resized 4K copy of src to dest.
func ResizeCopy(src, dest string) error {
	return processImage(src, dest, false, time.Time{}, nil)
}

// ResizeInPlace scales and center-crops the image file to 3840×2160.
func ResizeInPlace(path string) error {
	return processImage(path, path, false, time.Time{}, nil)
}

func processImage(readPath, writePath string, stamp bool, when time.Time, loc *time.Location) error {
	f, err := os.Open(readPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	img, format, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	rgba := resizeCover(img)

	if stamp {
		text := FormatDate(when, loc)
		face, err := faceAtSize(dateFontSize)
		if err != nil {
			return fmt.Errorf("load font: %w", err)
		}
		defer func() { _ = face.Close() }()

		marginX := targetWidth / 50
		marginY := targetHeight / 45

		textWidth := textWidth(face, text)
		textHeight := face.Metrics().Height.Ceil()
		x := targetWidth - marginX - textWidth
		y := targetHeight - marginY

		sample := rgba.SubImage(image.Rect(
			max(0, x-textWidth/4),
			max(0, y-textHeight),
			targetWidth,
			targetHeight,
		)).(*image.RGBA)

		textColor := color.RGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
		if averageLuminance(sample) < luminanceCutoff {
			textColor = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		}

		d := &font.Drawer{
			Dst:  rgba,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(text)
	}

	out, err := os.Create(writePath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return jpeg.Encode(out, rgba, &jpeg.Options{Quality: 95})
	case "png":
		return png.Encode(out, rgba)
	default:
		return fmt.Errorf("unsupported image format %q", format)
	}
}

// PrepareUploadPath returns a 4K-ready path to upload, optionally with a date stamp.
// User files are copied to a temp file first; temp wallpaper files are processed in place.
func PrepareUploadPath(path string, stamp bool, when time.Time, loc *time.Location) (uploadPath string, cleanup func(), err error) {
	cleanup = func() {}

	base := filepath.Base(path)
	if strings.HasPrefix(base, "frame-it-") {
		if stamp {
			err = StampDate(path, when, loc)
		} else {
			err = ResizeInPlace(path)
		}
		if err != nil {
			return "", cleanup, err
		}
		return path, cleanup, nil
	}

	ext := filepath.Ext(path)
	if ext == "" {
		ext = ".jpg"
	}
	tmp, err := os.CreateTemp("", "frame-it-stamp-*"+ext)
	if err != nil {
		return "", cleanup, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	cleanup = func() { _ = os.Remove(tmpPath) }

	if stamp {
		err = StampDateCopy(path, tmpPath, when, loc)
	} else {
		err = ResizeCopy(path, tmpPath)
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmpPath, cleanup, nil
}

func faceAtSize(size float64) (font.Face, error) {
	ttf, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func textWidth(face font.Face, text string) int {
	return font.MeasureString(face, text).Ceil()
}

func averageLuminance(img *image.RGBA) float64 {
	b := img.Bounds()
	if b.Empty() {
		return 0
	}
	var sum float64
	var n float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			// ITU-R BT.709 luma
			lum := 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
			sum += lum / 255
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
