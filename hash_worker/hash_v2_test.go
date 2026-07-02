package hash_worker

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/anthonynsimon/bild/transform"
	"mahresources/models"
)

// asymmetricImage builds a recognizably non-symmetric test image so rotation and
// flipping produce a visibly different result (and thus a different hash unless
// correctly normalized).
func asymmetricImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c color.RGBA
			switch {
			case x < w/2 && y < h/2:
				c = color.RGBA{240, 20, 20, 255} // top-left red
			case x >= w/2 && y < h/2:
				c = color.RGBA{20, 240, 20, 255} // top-right green
			case x < w/2 && y >= h/2:
				c = color.RGBA{20, 20, 240, 255} // bottom-left blue
			default:
				c = color.RGBA{uint8(x % 256), uint8(y % 256), 200, 255} // bottom-right gradient
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

// jpegWithOrientation encodes img as JPEG and injects an EXIF APP1 segment
// carrying the given orientation tag (1..8), so the full ComputeV2Hashes EXIF
// path (readOrientation) is exercised.
func jpegWithOrientation(t *testing.T, img image.Image, orient uint16) []byte {
	t.Helper()
	base := encodeJPEG(t, img, 92)
	if len(base) < 2 || base[0] != 0xFF || base[1] != 0xD8 {
		t.Fatalf("expected JPEG SOI marker")
	}

	// Little-endian TIFF with a single IFD0 entry: Orientation (0x0112), SHORT, count 1.
	tiff := []byte{
		'I', 'I', 0x2A, 0x00, // byte order + magic
		0x08, 0x00, 0x00, 0x00, // offset to IFD0
		0x01, 0x00, // IFD0 entry count = 1
		0x12, 0x01, // tag 0x0112 (Orientation)
		0x03, 0x00, // type SHORT
		0x01, 0x00, 0x00, 0x00, // count 1
		byte(orient), byte(orient >> 8), 0x00, 0x00, // value + padding
		0x00, 0x00, 0x00, 0x00, // next IFD offset
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segLen := len(payload) + 2 // +2 for the length field itself
	app1 := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen)}
	app1 = append(app1, payload...)

	out := make([]byte, 0, len(base)+len(app1))
	out = append(out, 0xFF, 0xD8)   // SOI
	out = append(out, app1...)      // EXIF APP1
	out = append(out, base[2:]...)  // rest of the original JPEG
	return out
}

func TestReadOrientation(t *testing.T) {
	img := asymmetricImage(64, 64)
	for orient := uint16(1); orient <= 8; orient++ {
		data := jpegWithOrientation(t, img, orient)
		if got := readOrientation(data); got != int(orient) {
			t.Errorf("orientation %d: readOrientation = %d", orient, got)
		}
	}
	// A plain JPEG (no EXIF) reads as orientation 1.
	if got := readOrientation(encodeJPEG(t, img, 90)); got != 1 {
		t.Errorf("no-EXIF JPEG: readOrientation = %d, want 1", got)
	}
}

// TestOrientationNormalization: a camera stores pixels rotated 90° CCW and tags
// the file orientation=6 ("rotate 90° CW to display"). After normalization its
// pHash must match the upright original's.
func TestOrientationNormalization(t *testing.T) {
	upright := asymmetricImage(96, 96)

	// Stored pixels = upright rotated 90° CCW (270° CW).
	stored := transform.Rotate(upright, 270, &transform.RotationOptions{ResizeBounds: true})

	rotatedTagged := jpegWithOrientation(t, stored, 6)
	uprightPlain := encodeJPEG(t, upright, 92)

	a, err := ComputeV2Hashes(rotatedTagged)
	if err != nil {
		t.Fatalf("hash rotated: %v", err)
	}
	b, err := ComputeV2Hashes(uprightPlain)
	if err != nil {
		t.Fatalf("hash upright: %v", err)
	}

	// JPEG quantization runs on the rotated pixels in one case and upright pixels
	// in the other, so even a lossless 90° remap leaves a few bits of difference.
	// The contract is that a normalized rotated JPEG stays within the default
	// similarity threshold (10) of its upright twin.
	dist := HammingDistance(a.PHash, b.PHash)
	if dist > 10 {
		t.Errorf("normalized rotated pHash distance = %d, want <= 10", dist)
	}
}

// TestTransparentFlattenMatchesWhite: a transparent PNG must hash the same as the
// equivalent image composited onto white and saved as JPEG.
func TestTransparentFlattenMatchesWhite(t *testing.T) {
	const w, h = 96, 96
	// Shape on transparent background.
	transparent := image.NewRGBA(image.Rect(0, 0, w, h))
	// Equivalent shape on white background.
	white := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			white.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= w/4 && x < 3*w/4 && y >= h/4 && y < 3*h/4 {
				c := color.RGBA{30, 90, 200, 255}
				transparent.Set(x, y, c)
				white.Set(x, y, c)
			}
			// else: transparent stays alpha=0, white stays white
		}
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, transparent); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	whiteJPEG := encodeJPEG(t, white, 92)

	a, err := ComputeV2Hashes(pngBuf.Bytes())
	if err != nil {
		t.Fatalf("hash png: %v", err)
	}
	b, err := ComputeV2Hashes(whiteJPEG)
	if err != nil {
		t.Fatalf("hash white jpeg: %v", err)
	}
	if dist := HammingDistance(a.PHash, b.PHash); dist > 4 {
		t.Errorf("transparent-vs-white pHash distance = %d, want <= 4", dist)
	}
}

// TestRecompressedJPEGSimilar: heavy recompression stays within a small pHash distance.
func TestRecompressedJPEGSimilar(t *testing.T) {
	img := asymmetricImage(128, 128)
	high := encodeJPEG(t, img, 95)
	low := encodeJPEG(t, img, 35)

	a, err := ComputeV2Hashes(high)
	if err != nil {
		t.Fatalf("hash high: %v", err)
	}
	b, err := ComputeV2Hashes(low)
	if err != nil {
		t.Fatalf("hash low: %v", err)
	}
	if dist := HammingDistance(a.PHash, b.PHash); dist > 10 {
		t.Errorf("recompressed pHash distance = %d, want <= 10", dist)
	}
}

func TestFlatDetection(t *testing.T) {
	// Solid colour → flat.
	solid := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			solid.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}
	res, err := ComputeV2Hashes(encodeJPEG(t, solid, 95))
	if err != nil {
		t.Fatalf("hash solid: %v", err)
	}
	if res.Status != models.HashStatusFlat {
		t.Errorf("solid image status = %q, want flat", res.Status)
	}

	// Near-flat: uniform with a few 1-level-off pixels → still flat.
	nearFlat := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(128)
			if (x+y)%37 == 0 {
				v = 129
			}
			nearFlat.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	res, err = ComputeV2Hashes(encodeJPEG(t, nearFlat, 95))
	if err != nil {
		t.Fatalf("hash near-flat: %v", err)
	}
	if res.Status != models.HashStatusFlat {
		t.Errorf("near-flat image status = %q, want flat", res.Status)
	}

	// Normal image → not flat.
	res, err = ComputeV2Hashes(encodeJPEG(t, asymmetricImage(64, 64), 92))
	if err != nil {
		t.Fatalf("hash normal: %v", err)
	}
	if res.Status != models.HashStatusOK {
		t.Errorf("normal image status = %q, want ok", res.Status)
	}
}

func TestGIFDecodes(t *testing.T) {
	img := asymmetricImage(64, 64)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("gif encode: %v", err)
	}
	res, err := ComputeV2Hashes(buf.Bytes())
	if err != nil {
		t.Fatalf("hash gif: %v", err)
	}
	if res.PHash == 0 {
		t.Errorf("gif produced zero pHash")
	}
}

func TestCorruptImageErrors(t *testing.T) {
	if _, err := ComputeV2Hashes([]byte("not an image")); err == nil {
		t.Errorf("expected error decoding garbage bytes")
	}
}
