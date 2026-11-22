package cli

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// maxImagePixels and maxImageDim cap the size of an image the formatter
// decodes, read from the image header before decoding, so a small response
// cannot make the CLI allocate gigabytes. 2^25 pixels is a little more than 8K
// UHD (7680x4320). The scaler also allocates for every source row and column,
// so a long thin image within the pixel limit (1 x 2^25 takes about 3 GiB)
// needs the per-side limit too.
const (
	maxImagePixels = 1 << 25
	maxImageDim    = 16384
)

type imageCodec struct {
	decode       func(io.Reader) (image.Image, error)
	decodeConfig func(io.Reader) (image.Config, error)
}

// imageCodecs are the image types the formatter displays, keyed by the
// response's Content-Type. The declared type picks the decoder and the bytes
// are not sniffed, so a body in any other format (TIFF, BMP, ...) is never
// decoded.
var imageCodecs = map[string]imageCodec{
	"image/png":  {png.Decode, png.DecodeConfig},
	"image/jpeg": {jpeg.Decode, jpeg.DecodeConfig},
	"image/gif":  {gif.Decode, gif.DecodeConfig},
	"image/webp": {webp.Decode, webp.DecodeConfig},
}

// renderImage draws an image for a terminal with 24-bit color and Unicode half
// blocks, two pixel rows per line. It scales the image down, keeping its
// aspect ratio, to fit maxW columns and maxH pixel rows, and never scales it
// up. Transparent pixels are drawn as if over black.
func renderImage(c imageCodec, b []byte, maxW, maxH int) (string, error) {
	cfg, err := c.decodeConfig(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDim || cfg.Height > maxImageDim ||
		cfg.Width > maxImagePixels/cfg.Height {
		return "", fmt.Errorf("image is %dx%d; the limit is %d pixels, at most %d on a side",
			cfg.Width, cfg.Height, maxImagePixels, maxImageDim)
	}

	src, err := c.decode(bytes.NewReader(b))
	if err != nil {
		return "", err
	}

	// A GIF's first frame may be empty within a nonempty logical screen.
	sb := src.Bounds()
	if sb.Empty() {
		return "", fmt.Errorf("image is %dx%d; nothing to draw", sb.Dx(), sb.Dy())
	}
	w, h := fitWithin(sb.Dx(), sb.Dy(), maxW, maxH)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w == sb.Dx() && h == sb.Dy() {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, draw.Src, nil)
	}

	// image.RGBA is alpha-premultiplied, so its R, G and B are the color over
	// black.
	var out strings.Builder
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			top := dst.RGBAAt(x, y)
			if y+1 < h {
				// Upper pixel as the background, lower pixel as a lower half block.
				bottom := dst.RGBAAt(x, y+1)
				fmt.Fprintf(&out, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm▄",
					top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
			} else {
				// Odd last row: an upper half block on the default background.
				fmt.Fprintf(&out, "\x1b[49m\x1b[38;2;%d;%d;%dm▀", top.R, top.G, top.B)
			}
		}
		out.WriteString("\x1b[0m\n")
	}
	return out.String(), nil
}

// fitWithin returns w x h when it fits in maxW x maxH, and otherwise the
// largest size with the same aspect ratio that fits, at least 1x1.
func fitWithin(w, h, maxW, maxH int) (int, int) {
	maxW, maxH = max(maxW, 1), max(maxH, 1)
	if w <= maxW && h <= maxH {
		return w, h
	}
	if int64(w)*int64(maxH) > int64(h)*int64(maxW) {
		// Wider than the box: the width limit binds.
		return maxW, max(int(int64(h)*int64(maxW)/int64(w)), 1)
	}
	return max(int(int64(w)*int64(maxH)/int64(h)), 1), maxH
}
