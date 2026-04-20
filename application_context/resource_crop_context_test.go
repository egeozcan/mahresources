package application_context

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// seedImageResource writes image bytes to ctx.fs and creates a Resource row
// pointing at that path. Returns the Resource ID.
func seedImageResource(t *testing.T, ctx *MahresourcesContext, contentType string, data []byte, width, height uint) uint {
	t.Helper()
	hash := computeSHA1(data)
	ext := ".img"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	location := buildVersionResourcePath(hash, ext)

	require.NoError(t, ctx.fs.MkdirAll(path.Dir(location), 0755))
	f, err := ctx.fs.Create(location)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	owner := &models.Group{Name: "crop-owner-" + hash[:8]}
	require.NoError(t, ctx.db.Create(owner).Error)

	resource := &models.Resource{
		Name:        "crop-test" + ext,
		Hash:        hash,
		HashType:    "SHA1",
		Location:    location,
		ContentType: contentType,
		FileSize:    int64(len(data)),
		Width:       width,
		Height:      height,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, ctx.db.Create(resource).Error)
	return resource.ID
}

func makePNG(t *testing.T, w, h int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func makeJPEG(t *testing.T, w, h int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func setupCropTestCtx(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := createTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.ResourceVersion{}))
	return ctx
}

func TestCropResource_JPEG_CreatesNewVersion(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 100, 80, color.RGBA{R: 255, G: 100, B: 50, A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 100, 80)

	err := ctx.CropResource(resourceID, 10, 20, 40, 30, "")
	require.NoError(t, err)

	var versions []models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ?", resourceID).Order("version_number").Find(&versions).Error)
	require.Len(t, versions, 2, "expected original-lazy v1 plus cropped v2")

	v2 := versions[1]
	assert.Equal(t, 2, v2.VersionNumber)
	assert.Equal(t, uint(40), v2.Width)
	assert.Equal(t, uint(30), v2.Height)
	assert.Equal(t, "image/jpeg", v2.ContentType)
	assert.Equal(t, "SHA1", v2.HashType)
	assert.Contains(t, v2.Comment, "Cropped to 40×30")

	var updated models.Resource
	require.NoError(t, ctx.db.First(&updated, resourceID).Error)
	assert.Equal(t, uint(40), updated.Width)
	assert.Equal(t, uint(30), updated.Height)
	assert.Equal(t, "image/jpeg", updated.ContentType)
	assert.Equal(t, "SHA1", updated.HashType, "resource hash_type must match the new version (fixes the Rotate bug)")
	require.NotNil(t, updated.CurrentVersionID)
	assert.Equal(t, v2.ID, *updated.CurrentVersionID)
}

func TestCropResource_UsesUserComment(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 50, 50, color.RGBA{A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 50, 50)

	require.NoError(t, ctx.CropResource(resourceID, 0, 0, 10, 10, "  headshot crop  "))

	var v models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ? AND version_number = ?", resourceID, 2).First(&v).Error)
	assert.Equal(t, "headshot crop", v.Comment)
}

func TestCropResource_PNG_PreservesTransparency(t *testing.T) {
	ctx := setupCropTestCtx(t)
	// Make a PNG with a fully transparent upper-left quadrant and an opaque red lower-right
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for x := 0; x < 40; x++ {
		for y := 0; y < 40; y++ {
			if x < 20 && y < 20 {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			} else {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	resourceID := seedImageResource(t, ctx, "image/png", buf.Bytes(), 40, 40)

	// Crop the fully-transparent quadrant
	require.NoError(t, ctx.CropResource(resourceID, 0, 0, 20, 20, ""))

	var v models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ? AND version_number = ?", resourceID, 2).First(&v).Error)
	assert.Equal(t, "image/png", v.ContentType, "PNG source must stay PNG to preserve transparency")

	cropped, err := ctx.fs.Open(v.Location)
	require.NoError(t, err)
	defer cropped.Close()
	decoded, err := png.Decode(cropped)
	require.NoError(t, err)

	_, _, _, a := decoded.At(5, 5).RGBA()
	assert.Equal(t, uint32(0), a, "transparent pixels must remain transparent after crop")
}

func TestCropResource_InvalidRect(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 100, 100, color.RGBA{A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 100, 100)

	cases := []struct {
		name       string
		x, y, w, h int
		contains   string
	}{
		{"zero width", 0, 0, 0, 10, "width must be positive"},
		{"zero height", 0, 0, 10, 0, "height must be positive"},
		{"negative width", 0, 0, -5, 10, "width must be positive"},
		{"negative x", -1, 0, 10, 10, "origin must be non-negative"},
		{"negative y", 0, -1, 10, 10, "origin must be non-negative"},
		{"out of bounds width", 50, 0, 60, 10, "must be within image bounds"},
		{"out of bounds height", 0, 50, 10, 60, "must be within image bounds"},
		{"origin out of bounds", 200, 0, 10, 10, "must be within image bounds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ctx.CropResource(resourceID, tc.x, tc.y, tc.w, tc.h, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.contains)

			var count int64
			ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&count)
			assert.Equal(t, int64(0), count, "no version should be created on invalid rect")
		})
	}
}

func TestCropResource_NotAnImage(t *testing.T) {
	ctx := setupCropTestCtx(t)

	owner := &models.Group{Name: "non-image-owner"}
	require.NoError(t, ctx.db.Create(owner).Error)
	r := &models.Resource{
		Name:        "a.txt",
		Hash:        "deadbeef",
		HashType:    "SHA1",
		Location:    "/doc.txt",
		ContentType: "text/plain",
		FileSize:    3,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, ctx.db.Create(r).Error)

	err := ctx.CropResource(r.ID, 0, 0, 10, 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an image")
}

func TestCropResource_ResourceNotFound(t *testing.T) {
	ctx := setupCropTestCtx(t)
	err := ctx.CropResource(99999, 0, 0, 10, 10, "")
	require.Error(t, err)
	assert.True(t, strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(strings.ToLower(err.Error()), "record"), "expected not-found error, got %v", err)
}

func TestCropResource_UnsupportedFormat(t *testing.T) {
	ctx := setupCropTestCtx(t)

	// SVG is content-type image/* and IsImage() returns true, but image.Decode
	// has no registered SVG handler → decode error → "image cannot be cropped".
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="red"/></svg>`)
	resourceID := seedImageResource(t, ctx, "image/svg+xml", svg, 40, 40)

	err := ctx.CropResource(resourceID, 0, 0, 10, 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be cropped")
}

func TestCropResource_LazyMigrationCreatesV1(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 60, 40, color.RGBA{A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 60, 40)

	// Precondition: no versions before the crop
	var before int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&before)
	require.Equal(t, int64(0), before)

	require.NoError(t, ctx.CropResource(resourceID, 0, 0, 30, 20, ""))

	var versions []models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ?", resourceID).Order("version_number").Find(&versions).Error)
	require.Len(t, versions, 2)
	assert.Equal(t, 1, versions[0].VersionNumber)
	assert.Contains(t, versions[0].Comment, "before crop", "v1 should note it's the pre-crop original")
	assert.Equal(t, 2, versions[1].VersionNumber)
}

func TestCropResource_ClearsPreviews(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 80, 60, color.RGBA{A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 80, 60)

	// Seed a fake preview row
	preview := &models.Preview{
		ResourceId:  &resourceID,
		Width:       10,
		Height:      10,
		ContentType: "image/jpeg",
		Data:        []byte{0xff},
	}
	require.NoError(t, ctx.db.Create(preview).Error)

	require.NoError(t, ctx.CropResource(resourceID, 0, 0, 10, 10, ""))

	var remaining int64
	ctx.db.Model(&models.Preview{}).Where("resource_id = ?", resourceID).Count(&remaining)
	assert.Equal(t, int64(0), remaining, "previews must be cleared so they regenerate from the new current version")
}
