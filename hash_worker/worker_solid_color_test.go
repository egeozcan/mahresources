package hash_worker_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/Nr90/imgsim"
	"mahresources/hash_worker"
)

// makeSolidPNG creates a 300×300 PNG filled with a single color.
func makeSolidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, c)
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decodePNG decodes a PNG from raw bytes, failing the test on error.
func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// makeGradientPNG creates a 300×300 PNG with an RGB gradient plus an offset.
func makeGradientPNG(t *testing.T, offset int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			r := uint8(x + offset)
			g := uint8(y + offset)
			img.Set(x, y, color.RGBA{R: r, G: g, B: 128, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestSolidColorHashes_BothZero documents the imgsim library behaviour:
// uniform (solid) images produce DHash=0 AND AHash=0, making them indistinguishable
// from each other by hash distance alone. This is the root cause of BH-018.
func TestSolidColorHashes_BothZero(t *testing.T) {
	lightblue := makeSolidPNG(t, color.RGBA{R: 173, G: 216, B: 230, A: 255})
	orange := makeSolidPNG(t, color.RGBA{R: 255, G: 165, B: 0, A: 255})

	dhashA := imgsim.DifferenceHash(decodePNG(t, lightblue))
	dhashB := imgsim.DifferenceHash(decodePNG(t, orange))
	ahashA := imgsim.AverageHash(decodePNG(t, lightblue))
	ahashB := imgsim.AverageHash(decodePNG(t, orange))

	// Both hashes must be zero for all solid colors (library invariant)
	if uint64(dhashA) != 0 || uint64(dhashB) != 0 {
		t.Fatalf("pre-condition: DHash for solid colors must be 0, got %016x / %016x", uint64(dhashA), uint64(dhashB))
	}
	if uint64(ahashA) != 0 || uint64(ahashB) != 0 {
		t.Fatalf("pre-condition: AHash for solid colors must be 0 (imgsim AHash), got %016x / %016x", uint64(ahashA), uint64(ahashB))
	}

	t.Logf("Confirmed: DHash distance=%d, AHash distance=%d (both 0 for all solid colors)",
		imgsim.Distance(dhashA, dhashB), imgsim.Distance(ahashA, ahashB))
}

// TestSimilarity_SolidColorsMustNotMatch is the BH-018 regression test.
// Two different solid-color images have both DHash=0 and AHash=0.
// AreSimilar must return false for them to avoid false positives.
func TestSimilarity_SolidColorsMustNotMatch(t *testing.T) {
	lightblueImg := decodePNG(t, makeSolidPNG(t, color.RGBA{R: 173, G: 216, B: 230, A: 255}))
	orangeImg := decodePNG(t, makeSolidPNG(t, color.RGBA{R: 255, G: 165, B: 0, A: 255}))

	dA := uint64(imgsim.DifferenceHash(lightblueImg))
	aA := uint64(imgsim.AverageHash(lightblueImg))
	dB := uint64(imgsim.DifferenceHash(orangeImg))
	aB := uint64(imgsim.AverageHash(orangeImg))

	dhashThr := uint64(10) // matches HashSimilarityThreshold default
	ahashThr := uint64(5)  // BH-018 threshold

	similar := hash_worker.AreSimilar(dA, aA, dB, aB, dhashThr, ahashThr)
	if similar {
		t.Fatalf("BH-018: lightblue and orange solid PNGs must NOT be recorded as similar (both have zero hashes)")
	}
}

// TestSimilarity_NearDupesStillMatch verifies the fix does not suppress real near-duplicates.
func TestSimilarity_NearDupesStillMatch(t *testing.T) {
	base := makeGradientPNG(t, 0)
	near := makeGradientPNG(t, 3) // tiny perturbation

	dA := uint64(imgsim.DifferenceHash(decodePNG(t, base)))
	aA := uint64(imgsim.AverageHash(decodePNG(t, base)))
	dB := uint64(imgsim.DifferenceHash(decodePNG(t, near)))
	aB := uint64(imgsim.AverageHash(decodePNG(t, near)))

	similar := hash_worker.AreSimilar(dA, aA, dB, aB, 10, 5)
	if !similar {
		dDist := hash_worker.HammingDistance(dA, dB)
		aDist := hash_worker.HammingDistance(aA, aB)
		t.Fatalf("near-duplicate gradients must still register as similar after BH-018 fix (dDist=%d, aDist=%d)", dDist, aDist)
	}
}
