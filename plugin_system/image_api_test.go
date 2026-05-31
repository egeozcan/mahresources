package plugin_system

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// encodeTestImage creates a solid-color PNG image and returns it as a data URI.
func encodeTestImage(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// encodeTestImageJPEG creates a solid-color JPEG image and returns it as a data URI.
func encodeTestImageJPEG(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("failed to encode JPEG: %v", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// decodeDataURI decodes a data URI back to an image.Image.
func decodeDataURI(t *testing.T, uri string) image.Image {
	t.Helper()
	commaIdx := strings.Index(uri, ",")
	if commaIdx < 0 {
		t.Fatal("invalid data URI")
	}
	raw, err := base64.StdEncoding.DecodeString(uri[commaIdx+1:])
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("image decode: %v", err)
	}
	return img
}

// assertColor checks the 8-bit RGB at (x, y) against want, allowing a per-channel
// tolerance (lossy formats like JPEG need a non-zero tolerance).
func assertColor(t *testing.T, img image.Image, x, y int, want color.RGBA, tol int, label string) {
	t.Helper()
	r, g, b, _ := img.At(x, y).RGBA()
	gr, gg, gb := int(r>>8), int(g>>8), int(b>>8)
	if abs(gr-int(want.R)) > tol || abs(gg-int(want.G)) > tol || abs(gb-int(want.B)) > tol {
		t.Errorf("%s at (%d,%d): got RGB(%d,%d,%d), want RGB(%d,%d,%d) (tol=%d)",
			label, x, y, gr, gg, gb, want.R, want.G, want.B, tol)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var white = color.RGBA{255, 255, 255, 255}

// Test_padDataURIToAspectRatio_wideToSquare pads a wide (landscape) image to 1:1.
func Test_padDataURIToAspectRatio_wideToSquare(t *testing.T) {
	// 200x100 (ratio 2:1) -> pad to 1:1 -> 200x200, source centered vertically.
	red := color.RGBA{255, 0, 0, 255}
	uri := encodeTestImage(t, 200, 100, red)

	out, w, h, err := padDataURIToAspectRatio(uri, "1:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 200 || h != 200 {
		t.Fatalf("expected 200x200, got %dx%d", w, h)
	}

	img := decodeDataURI(t, out)
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 200 {
		t.Fatalf("decoded dimensions %dx%d, want 200x200", img.Bounds().Dx(), img.Bounds().Dy())
	}
	assertColor(t, img, 100, 0, white, 0, "top border")    // y=0 is padding
	assertColor(t, img, 100, 100, red, 0, "center")        // y=100 is source
	assertColor(t, img, 100, 199, white, 0, "bottom border")
}

// Test_padDataURIToAspectRatio_tallToSquare pads a tall (portrait) image to 1:1.
func Test_padDataURIToAspectRatio_tallToSquare(t *testing.T) {
	// 100x200 (ratio 1:2) -> pad to 1:1 -> 200x200, source centered horizontally.
	green := color.RGBA{0, 255, 0, 255}
	uri := encodeTestImage(t, 100, 200, green)

	out, w, h, err := padDataURIToAspectRatio(uri, "1:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 200 || h != 200 {
		t.Fatalf("expected 200x200, got %dx%d", w, h)
	}

	img := decodeDataURI(t, out)
	assertColor(t, img, 0, 100, white, 0, "left border")    // x=0 is padding
	assertColor(t, img, 100, 100, green, 0, "center")       // x=100 is source
	assertColor(t, img, 199, 100, white, 0, "right border")
}

// Test_padDataURIToAspectRatio_alreadyMatching returns the source unchanged
// (no borders) when it already matches the target ratio within tolerance.
func Test_padDataURIToAspectRatio_alreadyMatching(t *testing.T) {
	blue := color.RGBA{0, 0, 255, 255}
	uri := encodeTestImage(t, 200, 200, blue)

	out, w, h, err := padDataURIToAspectRatio(uri, "1:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 200 || h != 200 {
		t.Fatalf("expected unchanged 200x200, got %dx%d", w, h)
	}

	img := decodeDataURI(t, out)
	// Every corner should still be blue -- no white padding was added.
	assertColor(t, img, 0, 0, blue, 0, "top-left")
	assertColor(t, img, 199, 199, blue, 0, "bottom-right")
}

// Test_padDataURIToAspectRatio_4x3_to_16x9 pillarboxes a 4:3 image into 16:9.
func Test_padDataURIToAspectRatio_4x3_to_16x9(t *testing.T) {
	// 400x300 (1.333) -> 16:9 (1.778): pad left/right -> 533x300.
	purple := color.RGBA{128, 0, 128, 255}
	uri := encodeTestImage(t, 400, 300, purple)

	out, w, h, err := padDataURIToAspectRatio(uri, "16:9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 533 || h != 300 {
		t.Fatalf("expected 533x300, got %dx%d", w, h)
	}
	ratio := float64(w) / float64(h)
	if ratio < 16.0/9.0-0.01 || ratio > 16.0/9.0+0.01 {
		t.Errorf("padded ratio %.4f not close to 16:9", ratio)
	}

	img := decodeDataURI(t, out)
	// offsetX = (533-400)/2 = 66, so x in [66,466) is source.
	assertColor(t, img, 0, 150, white, 0, "left border")
	assertColor(t, img, 266, 150, purple, 0, "center")
	assertColor(t, img, 532, 150, white, 0, "right border")
}

// Test_padDataURIToAspectRatio_16x9_to_9x16 letterboxes a 16:9 image into 9:16.
func Test_padDataURIToAspectRatio_16x9_to_9x16(t *testing.T) {
	// 640x360 (1.778) -> 9:16 (0.5625): pad top/bottom -> 640x1138.
	orange := color.RGBA{255, 128, 0, 255}
	uri := encodeTestImage(t, 640, 360, orange)

	out, w, h, err := padDataURIToAspectRatio(uri, "9:16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 640 || h != 1138 {
		t.Fatalf("expected 640x1138, got %dx%d", w, h)
	}
	ratio := float64(w) / float64(h)
	if ratio < 9.0/16.0-0.01 || ratio > 9.0/16.0+0.01 {
		t.Errorf("padded ratio %.4f not close to 9:16", ratio)
	}

	img := decodeDataURI(t, out)
	// offsetY = (1138-360)/2 = 389, so y in [389,749) is source.
	assertColor(t, img, 320, 0, white, 0, "top border")
	assertColor(t, img, 320, 569, orange, 0, "center")
	assertColor(t, img, 320, 1137, white, 0, "bottom border")
}

// Test_padDataURIToAspectRatio_preservesJPEG verifies a JPEG input is re-encoded
// as JPEG (not inflated to PNG) and stays within tolerance.
func Test_padDataURIToAspectRatio_preservesJPEG(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	uri := encodeTestImageJPEG(t, 200, 100, red)

	out, w, h, err := padDataURIToAspectRatio(uri, "1:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 200 || h != 200 {
		t.Fatalf("expected 200x200, got %dx%d", w, h)
	}
	if !strings.HasPrefix(out, "data:image/jpeg;base64,") {
		t.Errorf("expected JPEG output, got prefix %q", out[:min(len(out), 30)])
	}

	img := decodeDataURI(t, out)
	// JPEG is lossy, so allow tolerance on the color checks.
	assertColor(t, img, 100, 0, white, 8, "top border")
	assertColor(t, img, 100, 100, red, 24, "center")
}

func Test_padDataURIToAspectRatio_errors(t *testing.T) {
	validImg := encodeTestImage(t, 10, 10, color.RGBA{1, 2, 3, 255})

	tests := []struct {
		name    string
		dataURI string
		ratio   string
	}{
		{"bad ratio format", validImg, "16-9"},
		{"non-numeric ratio", validImg, "wide:tall"},
		{"zero ratio", validImg, "0:1"},
		{"negative ratio", validImg, "-16:9"},
		{"missing comma", "data:image/png;base64" + "AAAA", "1:1"},
		{"bad base64", "data:image/png;base64,!!!notbase64!!!", "1:1"},
		{"undecodable image", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image")), "1:1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _, err := padDataURIToAspectRatio(tc.dataURI, tc.ratio)
			if err == nil {
				t.Fatalf("expected error, got nil (out=%q)", out[:min(len(out), 30)])
			}
			if out != "" {
				t.Errorf("expected empty output on error, got %q", out[:min(len(out), 30)])
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
