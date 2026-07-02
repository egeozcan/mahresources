package hash_worker

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"

	"github.com/Nr90/imgsim"
	"github.com/anthonynsimon/bild/transform"
	"github.com/corona10/goimagehash"
	"github.com/rwcarlsen/goexif/exif"
	"mahresources/models"
)

// HashVersionV2 is the current v2 hash engine version stamped on image_hashes.HashVersion.
const HashVersionV2 = 2

// flatVarianceEpsilon is the grayscale-variance threshold (on a 0..255 scale)
// below which an image is treated as "flat" (solid colour, blank scan) and
// excluded from perceptual matching. Genuinely uniform images have variance 0;
// this small epsilon also catches near-uniform images dominated by sensor noise
// while leaving documents, gradients, and real photos well clear.
const flatVarianceEpsilon = 1.5

// flatGridSize is the edge length of the downsampled grid used for the flat check.
const flatGridSize = 32

// V2Hashes bundles every perceptual hash and status derived from a single decode.
//
// The v2 aHash distance (resource_similarities.a_distance) is derived from the
// legacy imgsim average hash rather than a separate goimagehash aHash: v2 rows
// persist the imgsim aHash in image_hashes.a_hash_int (so v1-vs-v2 comparisons
// keep working), and there is no dedicated v2-aHash column, so computing a second
// average hash per image would be wasted work across millions of rows.
type V2Hashes struct {
	PHash       uint64 // v2 goimagehash pHash (DCT, bilinear)
	LegacyDHash uint64 // legacy imgsim difference hash (dual-write)
	LegacyAHash uint64 // legacy imgsim average hash (dual-write)
	Status      string // models.HashStatusOK or models.HashStatusFlat
}

// ComputeV2Hashes decodes the image bytes once, normalizes it (EXIF orientation,
// alpha flattened onto white), classifies flat images, and computes both the v2
// goimagehash pHash/aHash and the legacy imgsim dHash/aHash from the same pixels.
// Returns an error only when the image cannot be decoded (caller marks failed).
func ComputeV2Hashes(data []byte) (*V2Hashes, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errors.New("nil image after decode")
	}

	// 1. Normalize EXIF orientation (no-op for formats/files without the tag).
	if orient := readOrientation(data); orient > 1 {
		img = applyOrientation(img, orient)
	}

	// 2. Flatten any alpha channel onto a white background so a transparent PNG
	//    hashes the same as its white-matted JPEG twin.
	flat := flattenOntoWhite(img)

	res := &V2Hashes{Status: models.HashStatusOK}

	// 3. Flat detection on a downsampled grayscale grid.
	if isFlat(flat) {
		res.Status = models.HashStatusFlat
	}

	// 4. Perceptual hashes from the same normalized image.
	if pHash, err := goimagehash.PerceptionHash(flat); err == nil {
		res.PHash = pHash.GetHash()
	} else {
		return nil, err
	}

	res.LegacyDHash = uint64(imgsim.DifferenceHash(flat))
	res.LegacyAHash = uint64(imgsim.AverageHash(flat))

	return res, nil
}

// readOrientation returns the EXIF orientation tag (1..8), or 1 when absent or
// unreadable. goexif only parses JPEG/TIFF EXIF; other formats return 1.
func readOrientation(data []byte) int {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	orient, err := tag.Int(0)
	if err != nil || orient < 1 || orient > 8 {
		return 1
	}
	return orient
}

// applyOrientation returns img rotated/flipped so it displays upright, given the
// EXIF orientation value. Mapping follows the standard TIFF orientation table.
// bild's Rotate is clockwise; ResizeBounds keeps the full (dimension-swapped) image.
func applyOrientation(img image.Image, orient int) image.Image {
	keep := &transform.RotationOptions{ResizeBounds: true}
	switch orient {
	case 2:
		return transform.FlipH(img)
	case 3:
		return transform.Rotate(img, 180, keep)
	case 4:
		return transform.FlipV(img)
	case 5: // transpose: flip horizontal, then 90° CCW
		return transform.Rotate(transform.FlipH(img), 270, keep)
	case 6: // rotate 90° CW
		return transform.Rotate(img, 90, keep)
	case 7: // transverse: flip vertical, then 90° CCW
		return transform.Rotate(transform.FlipV(img), 270, keep)
	case 8: // rotate 90° CCW
		return transform.Rotate(img, 270, keep)
	default:
		return img
	}
}

// flattenOntoWhite composites img over an opaque white background. Images with no
// alpha are returned essentially unchanged (drawn onto white, which is a no-op for
// fully opaque pixels).
func flattenOntoWhite(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}

// isFlat reports whether the image has near-zero grayscale variance on a small
// downsampled grid (solid colour, blank page).
func isFlat(img image.Image) bool {
	grid := transform.Resize(img, flatGridSize, flatGridSize, transform.Linear)
	b := grid.Bounds()

	var sum, sumSq float64
	var n float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := grid.At(x, y).RGBA()
			// Rec. 601 luma; RGBA() returns 16-bit values, scale to 0..255.
			gray := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257.0
			sum += gray
			sumSq += gray * gray
			n++
		}
	}
	if n == 0 {
		return true
	}
	mean := sum / n
	variance := sumSq/n - mean*mean
	return variance < flatVarianceEpsilon
}
