package overlay

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

const (
	targetWidth  = 3840
	targetHeight = 2160
	dateFontSize = 48
)

// resizeCover scales and center-crops src to 3840×2160.
func resizeCover(src image.Image) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return dst
	}

	scaleX := float64(targetWidth) / float64(srcW)
	scaleY := float64(targetHeight) / float64(srcH)
	scale := scaleX
	if scaleY > scaleX {
		scale = scaleY
	}

	scaledW := int(float64(srcW)*scale + 0.5)
	scaledH := int(float64(srcH)*scale + 0.5)

	tmp := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	xdraw.CatmullRom.Scale(tmp, tmp.Bounds(), src, srcBounds, draw.Over, nil)

	offX := (scaledW - targetWidth) / 2
	offY := (scaledH - targetHeight) / 2
	draw.Draw(dst, dst.Bounds(), tmp, image.Pt(offX, offY), draw.Src)

	return dst
}
