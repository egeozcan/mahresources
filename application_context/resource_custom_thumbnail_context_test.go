package application_context

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/disintegration/imaging"

	"mahresources/models"
	"mahresources/models/query_models"
)

func makeCreator(ownerID uint, name string) *query_models.ResourceCreator {
	return &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:    name,
			OwnerId: ownerID,
		},
	}
}

// makeColorJPEG returns JPEG bytes of dimensions w×h filled with the given
// solid color so tests can assert which image actually got served.
func makeColorJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(95)); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// sampleCenterColor decodes JPEG bytes and returns the color of the center
// pixel. JPEG compression introduces small deltas; callers compare with a
// tolerance.
func sampleCenterColor(t *testing.T, data []byte) color.RGBA {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode jpeg: %v", err)
	}
	b := img.Bounds()
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	r, g, bl, a := img.At(cx, cy).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
}

// colorClose reports whether a and b are within tol on every channel.
func colorClose(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

func TestSetCustomThumbnail_ReplacesAutoForImage(t *testing.T) {
	ctx := newThumbnailTestContext(t, "custom_thumb_replace_image_test")

	// Resource itself is solid red — the auto path would serve red thumbs.
	red := color.RGBA{R: 255, A: 255}
	res := &models.Resource{Name: "red", Hash: "redhash", ContentType: "image/jpeg", Width: 800, Height: 600}
	// Use AddResource so the file lands in the in-memory FS for auto path.
	owner := &models.Group{Name: "owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	added, err := ctx.AddResource(newBytesFile(makeColorJPEG(t, 800, 600, red)), "red.jpg",
		makeCreator(owner.ID, "red"))
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	res = added

	// Generate the auto thumbnail at 200×0 → should be red.
	auto, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 200, 0, context.Background())
	if err != nil {
		t.Fatalf("auto thumb: %v", err)
	}
	if got := sampleCenterColor(t, auto.Data); !colorClose(got, red, 25) {
		t.Fatalf("auto thumbnail center is %v; want close to red %v", got, red)
	}

	// Upload a custom thumbnail that is solid blue.
	blue := color.RGBA{B: 255, A: 255}
	customBytes := makeColorJPEG(t, 1024, 768, blue)
	if err := ctx.SetCustomThumbnail(context.Background(), res.ID, bytes.NewReader(customBytes)); err != nil {
		t.Fatalf("SetCustomThumbnail: %v", err)
	}

	// Now requests at any size must return blue.
	for _, dim := range [][2]uint{{200, 0}, {400, 300}, {0, 100}} {
		got, err := ctx.LoadOrCreateThumbnailForResource(res.ID, dim[0], dim[1], context.Background())
		if err != nil {
			t.Fatalf("thumb at %vx%v: %v", dim[0], dim[1], err)
		}
		if c := sampleCenterColor(t, got.Data); !colorClose(c, blue, 25) {
			t.Errorf("thumbnail at %vx%v center is %v; want close to blue %v", dim[0], dim[1], c, blue)
		}
	}
}

func TestSetCustomThumbnail_RejectsNonImage(t *testing.T) {
	ctx := newThumbnailTestContext(t, "custom_thumb_reject_test")
	owner := &models.Group{Name: "owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	res, err := ctx.AddResource(newBytesFile(makeColorJPEG(t, 100, 100, color.RGBA{R: 200, A: 255})), "src.jpg",
		makeCreator(owner.ID, "src"))
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	err = ctx.SetCustomThumbnail(context.Background(), res.ID, strings.NewReader("not an image"))
	if err == nil {
		t.Fatal("expected error from non-image upload")
	}
	var invalid *InvalidThumbnailError
	if !errors.As(err, &invalid) {
		t.Errorf("expected *InvalidThumbnailError; got %T: %v", err, err)
	}
}

func TestSetCustomThumbnail_ResizesOversizedUploads(t *testing.T) {
	ctx := newThumbnailTestContext(t, "custom_thumb_resize_test")
	owner := &models.Group{Name: "owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	res, err := ctx.AddResource(newBytesFile(makeColorJPEG(t, 200, 200, color.RGBA{R: 200, A: 255})), "src.jpg",
		makeCreator(owner.ID, "src2"))
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	// 4000×3000 → long edge must come down to 1920.
	huge := makeColorJPEG(t, 4000, 3000, color.RGBA{G: 200, A: 255})
	if err := ctx.SetCustomThumbnail(context.Background(), res.ID, bytes.NewReader(huge)); err != nil {
		t.Fatalf("SetCustomThumbnail: %v", err)
	}

	// Inspect the stored null thumbnail.
	var stored models.Preview
	if err := ctx.db.Where("resource_id = ? AND width = 0 AND height = 0", res.ID).First(&stored).Error; err != nil {
		t.Fatalf("read stored null thumb: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(stored.Data))
	if err != nil {
		t.Fatalf("decode stored thumb: %v", err)
	}
	b := img.Bounds()
	if b.Dx() > maxCustomThumbnailDimension || b.Dy() > maxCustomThumbnailDimension {
		t.Errorf("stored thumb %dx%d exceeds max edge %d", b.Dx(), b.Dy(), maxCustomThumbnailDimension)
	}
	// Aspect ratio (4:3) must be roughly preserved (within rounding).
	ratio := float64(b.Dx()) / float64(b.Dy())
	const want = 4.0 / 3.0
	if delta := ratio - want; delta < -0.02 || delta > 0.02 {
		t.Errorf("aspect ratio %v drifted from 4:3 (%v)", ratio, want)
	}
}

func TestClearThumbnails_RestoresAutoForImage(t *testing.T) {
	ctx := newThumbnailTestContext(t, "custom_thumb_clear_test")

	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	owner := &models.Group{Name: "owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	res, err := ctx.AddResource(newBytesFile(makeColorJPEG(t, 800, 600, red)), "red.jpg",
		makeCreator(owner.ID, "redclear"))
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	if err := ctx.SetCustomThumbnail(context.Background(), res.ID, bytes.NewReader(makeColorJPEG(t, 800, 600, blue))); err != nil {
		t.Fatalf("SetCustomThumbnail: %v", err)
	}
	gotBlue, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 200, 0, context.Background())
	if err != nil {
		t.Fatalf("after set: %v", err)
	}
	if c := sampleCenterColor(t, gotBlue.Data); !colorClose(c, blue, 25) {
		t.Fatalf("expected blue after set, got %v", c)
	}

	if err := ctx.ClearThumbnails(context.Background(), res.ID); err != nil {
		t.Fatalf("ClearThumbnails: %v", err)
	}

	// All previews must be gone — including the resized cached variant.
	var remaining int64
	ctx.db.Model(&models.Preview{}).Where("resource_id = ?", res.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("expected 0 previews after clear; got %d", remaining)
	}

	// Auto path regenerates → red.
	gotRed, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 200, 0, context.Background())
	if err != nil {
		t.Fatalf("after clear: %v", err)
	}
	if c := sampleCenterColor(t, gotRed.Data); !colorClose(c, red, 25) {
		t.Errorf("expected red after clear, got %v", c)
	}
}

func TestLatestPreviewVersion_ChangesAfterMutation(t *testing.T) {
	ctx := newThumbnailTestContext(t, "custom_thumb_version_test")

	owner := &models.Group{Name: "owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	res, err := ctx.AddResource(newBytesFile(makeColorJPEG(t, 400, 300, color.RGBA{R: 255, A: 255})), "v.jpg",
		makeCreator(owner.ID, "v"))
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	v0 := ctx.LatestPreviewVersion(context.Background(), res.ID)
	if v0 != 0 {
		t.Fatalf("expected version 0 before any preview; got %d", v0)
	}

	if _, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 200, 0, context.Background()); err != nil {
		t.Fatalf("load thumb: %v", err)
	}
	v1 := ctx.LatestPreviewVersion(context.Background(), res.ID)
	if v1 == v0 {
		t.Fatalf("version did not change after auto thumb generation: %d", v1)
	}

	if err := ctx.SetCustomThumbnail(context.Background(), res.ID, bytes.NewReader(makeColorJPEG(t, 400, 300, color.RGBA{B: 255, A: 255}))); err != nil {
		t.Fatalf("SetCustomThumbnail: %v", err)
	}
	v2 := ctx.LatestPreviewVersion(context.Background(), res.ID)
	if v2 == v1 {
		t.Fatalf("version did not change after custom thumb upload: %d", v2)
	}
}
