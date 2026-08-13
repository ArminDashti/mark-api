package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	_ "golang.org/x/image/webp"
)

const (
	MinSize = 16
	MaxSize = 2048
)

// ClampSize returns a valid square size (default 128).
func ClampSize(size int) int {
	if size <= 0 {
		return 128
	}
	if size < MinSize {
		return MinSize
	}
	if size > MaxSize {
		return MaxSize
	}
	return size
}

// DecodeInfo is metadata from an original file.
type DecodeInfo struct {
	Width    int
	Height   int
	HasAlpha bool
	MIME     string
}

// Inspect decodes an original for dimensions and alpha.
func Inspect(data []byte, mime string) (DecodeInfo, error) {
	info := DecodeInfo{MIME: mime}
	if isSVG(mime, data) {
		icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
		if err != nil {
			return info, fmt.Errorf("svg: %w", err)
		}
		w, h := icon.ViewBox.W, icon.ViewBox.H
		if w <= 0 {
			w = 256
		}
		if h <= 0 {
			h = 256
		}
		info.Width = int(math.Round(w))
		info.Height = int(math.Round(h))
		info.HasAlpha = true
		info.MIME = "image/svg+xml"
		return info, nil
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return info, err
	}
	b := img.Bounds()
	info.Width = b.Dx()
	info.Height = b.Dy()
	info.HasAlpha = imageHasAlpha(img, format)
	switch format {
	case "jpeg":
		info.MIME = "image/jpeg"
	case "png":
		info.MIME = "image/png"
	case "webp":
		info.MIME = "image/webp"
	default:
		if info.MIME == "" {
			info.MIME = "image/" + format
		}
	}
	return info, nil
}

// PNGSquare contain-fits the original into a transparent size×size PNG.
func PNGSquare(data []byte, mime string, size int) ([]byte, error) {
	size = ClampSize(size)
	src, err := decodeImage(data, mime, size)
	if err != nil {
		return nil, err
	}
	fitted := imaging.Fit(src, size, size, imaging.Lanczos)
	canvas := imaging.New(size, size, color.NRGBA{0, 0, 0, 0})
	out := imaging.PasteCenter(canvas, fitted)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeImage(data []byte, mime string, size int) (image.Image, error) {
	if isSVG(mime, data) {
		return rasterizeSVG(data, size)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func rasterizeSVG(data []byte, size int) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := icon.ViewBox.W, icon.ViewBox.H
	if w <= 0 {
		w = float64(size)
	}
	if h <= 0 {
		h = float64(size)
	}
	scale := math.Min(float64(size)/w, float64(size)/h)
	dw := int(math.Ceil(w * scale))
	dh := int(math.Ceil(h * scale))
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	icon.SetTarget(0, 0, float64(dw), float64(dh))
	rgba := image.NewRGBA(image.Rect(0, 0, dw, dh))
	scanner := rasterx.NewScannerGV(dw, dh, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(dw, dh, scanner)
	icon.Draw(raster, 1.0)
	return rgba, nil
}

func isSVG(mime string, data []byte) bool {
	if strings.Contains(strings.ToLower(mime), "svg") {
		return true
	}
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 {
		return false
	}
	s := strings.ToLower(string(trim[:min(len(trim), 256)]))
	return strings.Contains(s, "<svg")
}

func imageHasAlpha(img image.Image, format string) bool {
	if format == "jpeg" {
		return false
	}
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.Gray16, *image.CMYK:
		return false
	case *image.NRGBA, *image.NRGBA64, *image.RGBA, *image.RGBA64, *image.Alpha, *image.Alpha16:
		return true
	}
	if img.ColorModel() == color.NRGBAModel || img.ColorModel() == color.RGBAModel {
		return true
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}
