package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

func encodePNG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, m))
	return buf.Bytes()
}

// pngHeader returns a PNG that has only a signature and an IHDR chunk for a
// w x h RGBA image: enough for png.DecodeConfig, not for png.Decode.
func pngHeader(w, h uint32) []byte {
	ihdr := make([]byte, 0, 17)
	ihdr = append(ihdr, "IHDR"...)
	ihdr = binary.BigEndian.AppendUint32(ihdr, w)
	ihdr = binary.BigEndian.AppendUint32(ihdr, h)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // 8-bit RGBA, no interlace
	b := []byte("\x89PNG\r\n\x1a\n")
	b = binary.BigEndian.AppendUint32(b, 13)
	b = append(b, ihdr...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(ihdr))
}

// gifEmptyFrame returns a GIF with a 10x100 logical screen and one w x h frame
// at the origin that has no pixel data, so it decodes only when w or h is 0.
func gifEmptyFrame(w, h uint16) []byte {
	b := []byte("GIF89a")
	b = binary.LittleEndian.AppendUint16(b, 10) // logical screen
	b = binary.LittleEndian.AppendUint16(b, 100)
	b = append(b, 0x80, 0, 0, 0xff, 0, 0, 0, 0, 0xff) // a global palette: red, blue
	b = append(b, 0x2c, 0, 0, 0, 0)                   // a frame at (0, 0)
	b = binary.LittleEndian.AppendUint16(b, w)
	b = binary.LittleEndian.AppendUint16(b, h)
	b = append(b, 0, 2, 1, 0x2c, 0) // LZW with 2-bit codes: clear, end of information
	return append(b, 0x3b)          // trailer
}

func TestRenderImageLayout(t *testing.T) {
	// 2 wide, 3 high: the last row has no pixel below it.
	m := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	m.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	m.Set(1, 0, color.NRGBA{0, 255, 0, 255})
	m.Set(0, 1, color.NRGBA{0, 0, 255, 255})
	m.Set(1, 1, color.NRGBA{255, 255, 255, 255})
	m.Set(0, 2, color.NRGBA{10, 20, 30, 255})
	m.Set(1, 2, color.NRGBA{255, 255, 255, 0}) // transparent: drawn over black

	got, err := renderImage(imageCodecs["image/png"], encodePNG(t, m), 80, 48)
	require.NoError(t, err)
	assert.Equal(t,
		"\x1b[48;2;255;0;0m\x1b[38;2;0;0;255m▄"+
			"\x1b[48;2;0;255;0m\x1b[38;2;255;255;255m▄"+
			"\x1b[0m\n"+
			"\x1b[49m\x1b[38;2;10;20;30m▀"+
			"\x1b[49m\x1b[38;2;0;0;0m▀"+
			"\x1b[0m\n",
		got)
}

func TestRenderImageScalesDownToFit(t *testing.T) {
	// Left half red, right half blue.
	m := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= 4 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			m.Set(x, y, c)
		}
	}

	// 8x4 into 4 columns and 4 pixel rows: 4x2, one line of four cells, red at
	// the left edge and blue at the right (cropping would give four red cells).
	got, err := renderImage(imageCodecs["image/png"], encodePNG(t, m), 4, 4)
	require.NoError(t, err)
	line, ok := strings.CutSuffix(got, "\x1b[0m\n")
	require.True(t, ok, "%q", got)
	require.NotContains(t, line, "\n")
	cells := strings.SplitAfter(line, "▄")
	require.Len(t, cells, 5, "%q", got) // four cells and the empty rest
	assert.Equal(t, "\x1b[48;2;255;0;0m\x1b[38;2;255;0;0m▄", cells[0])
	assert.Equal(t, "\x1b[48;2;0;0;255m\x1b[38;2;0;0;255m▄", cells[3])
}

func TestFitWithin(t *testing.T) {
	for _, c := range []struct{ w, h, maxW, maxH, wantW, wantH int }{
		{2, 2, 80, 48, 2, 2},         // fits: never scaled up
		{80, 48, 80, 48, 80, 48},     // exactly fits
		{160, 48, 80, 48, 80, 24},    // too wide
		{80, 96, 80, 48, 40, 48},     // too high
		{8, 4, 4, 4, 4, 2},           // wider than the box
		{4, 8, 4, 4, 2, 4},           // higher than the box
		{1000, 1, 80, 48, 80, 1},     // a thin line stays at least one pixel high
		{1, 1000, 80, 48, 1, 48},     // and at least one pixel wide
		{10, 10, 0, 0, 1, 1},         // an empty terminal size still draws one cell
		{1 << 25, 1, 80, 100, 80, 1}, // 2^25 x 100 needs 64 bits
	} {
		w, h := fitWithin(c.w, c.h, c.maxW, c.maxH)
		assert.Equal(t, [2]int{c.wantW, c.wantH}, [2]int{w, h}, "fitWithin(%d, %d, %d, %d)", c.w, c.h, c.maxW, c.maxH)
	}
}

func TestRenderImageRefusesHugeImagesBeforeDecoding(t *testing.T) {
	for _, c := range []struct {
		w, h    uint32
		refused bool
	}{
		{8193, 4096, true},   // one column over the pixel limit
		{8192, 4096, false},  // exactly at the pixel limit
		{16385, 1, true},     // within the pixel limit, one column over the side limit
		{1, 16385, true},     // and one row over it
		{1 << 25, 1, true},   // the scaler would allocate about 2 GiB
		{1, 1 << 25, true},   // and about 3 GiB
		{16384, 2048, false}, // exactly at both limits
		{2048, 16384, false},
	} {
		// Every header fails: a refused one from the header alone, an allowed one
		// only because the body stops after the header.
		_, err := renderImage(imageCodecs["image/png"], pngHeader(c.w, c.h), 80, 48)
		require.Error(t, err, "%dx%d", c.w, c.h)
		if c.refused {
			assert.EqualError(t, err, fmt.Sprintf("image is %dx%d; the limit is 33554432 pixels, at most 16384 on a side", c.w, c.h))
		} else {
			assert.NotContains(t, err.Error(), "the limit is", "%dx%d", c.w, c.h)
		}
	}
}

func TestRenderImageRefusesAnEmptyGIFFrame(t *testing.T) {
	// The logical screen passes the header check, but the first frame, which is
	// what is drawn, has no pixels: refused, not drawn as black rows.
	for _, c := range []struct{ w, h uint16 }{{0, 100}, {10, 0}, {0, 0}} {
		got, err := renderImage(imageCodecs["image/gif"], gifEmptyFrame(c.w, c.h), 80, 48)
		assert.EqualError(t, err, fmt.Sprintf("image is %dx%d; nothing to draw", c.w, c.h))
		assert.Empty(t, got)
	}
}

func TestRenderImageDecodesByContentType(t *testing.T) {
	assert.ElementsMatch(t, []string{"image/png", "image/jpeg", "image/gif", "image/webp"}, maps.Keys(imageCodecs))

	// The declared type picks the decoder: a PNG body under any other image type
	// is refused, not sniffed and decoded as PNG.
	b := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	for _, ct := range []string{"image/jpeg", "image/gif", "image/webp"} {
		_, err := renderImage(imageCodecs[ct], b, 80, 48)
		assert.Error(t, err, ct)
	}
}

func TestRenderImageWebP(t *testing.T) {
	// A 1x1 lossy WebP of mid grey.
	b, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)

	got, err := renderImage(imageCodecs["image/webp"], b, 80, 48)
	require.NoError(t, err)
	assert.Equal(t, "\x1b[49m\x1b[38;2;128;128;128m▀\x1b[0m\n", got)
}
