package application_context

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"mahresources/auth"
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
	case "image/bmp":
		ext = ".bmp"
	case "image/tiff":
		ext = ".tiff"
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

//lint:ignore U1000 kept for symmetry with makeJPEG; used in future PNG crop tests
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

	err := ctx.CropResource(context.Background(), resourceID, 10, 20, 40, 30, "")
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

	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 10, 10, "  headshot crop  "))

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
	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 20, 20, ""))

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
			err := ctx.CropResource(context.Background(), resourceID, tc.x, tc.y, tc.w, tc.h, "")
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

	err := ctx.CropResource(context.Background(), r.ID, 0, 0, 10, 10, "")
	require.Error(t, err)
	// The refusal used to be a bare "resource is not an image", which
	// statusCodeForError could not classify — so cropping a text, JSON, PDF,
	// video or audio resource answered HTTP 500. Crop now shares rotate's and
	// recalculate's gate and wording, which maps to 415 and names the format.
	assert.Contains(t, err.Error(), "not a raster image format")
	assert.Contains(t, err.Error(), "text/plain", "the message must name the format the caller supplied")
}

func TestCropResource_ResourceNotFound(t *testing.T) {
	ctx := setupCropTestCtx(t)
	err := ctx.CropResource(context.Background(), 99999, 0, 0, 10, 10, "")
	require.Error(t, err)
	assert.True(t, strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(strings.ToLower(err.Error()), "record"), "expected not-found error, got %v", err)
}

func TestCropResource_UnsupportedFormat(t *testing.T) {
	ctx := setupCropTestCtx(t)

	// SVG is content-type image/* — IsImage() returns true — and image.Decode has
	// no registered SVG handler. It used to reach the decoder and come back with
	// a decode error; it is now refused by the content-type gate before any file
	// is opened, exactly as rotate and dimension recalculation refuse it
	// (findings 10 and 11). Either way it must be a 4xx that names the format.
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="red"/></svg>`)
	resourceID := seedImageResource(t, ctx, "image/svg+xml", svg, 40, 40)

	err := ctx.CropResource(context.Background(), resourceID, 0, 0, 10, 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image/svg+xml", "the message must name the format the caller supplied")
	assert.Contains(t, err.Error(), "not a raster image format")
}

func TestCropResource_LazyMigrationCreatesV1(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 60, 40, color.RGBA{A: 255})
	resourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 60, 40)

	// Precondition: no versions before the crop
	var before int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&before)
	require.Equal(t, int64(0), before)

	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 30, 20, ""))

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

	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 10, 10, ""))

	var remaining int64
	ctx.db.Model(&models.Preview{}).Where("resource_id = ?", resourceID).Count(&remaining)
	assert.Equal(t, int64(0), remaining, "previews must be cleared so they regenerate from the new current version")
}

func TestCropResource_BMP_ConvertsToPNG(t *testing.T) {
	ctx := setupCropTestCtx(t)
	src := image.NewRGBA(image.Rect(0, 0, 30, 20))
	for x := 0; x < 30; x++ {
		for y := 0; y < 20; y++ {
			src.Set(x, y, color.RGBA{R: 10, G: 200, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, bmp.Encode(&buf, src))
	resourceID := seedImageResource(t, ctx, "image/bmp", buf.Bytes(), 30, 20)

	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 10, 10, ""))

	var v models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ? AND version_number = ?", resourceID, 2).First(&v).Error)
	assert.Equal(t, "image/png", v.ContentType, "BMP source is re-encoded as PNG since we have no BMP encoder")
	assert.Contains(t, v.Comment, "re-encoded as PNG")
}

func TestCropResourceToNewResource_LeavesSourceUntouched(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 100, 80, color.RGBA{R: 12, G: 200, B: 90, A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 100, 80)

	var before models.Resource
	require.NoError(t, ctx.db.First(&before, sourceID).Error)

	created, err := ctx.CropResourceToNewResource(context.Background(), sourceID, 10, 20, 40, 30, "")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEqual(t, sourceID, created.ID, "the crop must be a distinct resource")

	// The whole point of the mode: the source keeps its bytes, dimensions and
	// version history. A stray version row here means the version path leaked in.
	var after models.Resource
	require.NoError(t, ctx.db.First(&after, sourceID).Error)
	assert.Equal(t, before.Hash, after.Hash)
	assert.Equal(t, before.Location, after.Location)
	assert.Equal(t, uint(100), after.Width)
	assert.Equal(t, uint(80), after.Height)
	assert.Equal(t, before.CurrentVersionID, after.CurrentVersionID)

	var sourceVersions int64
	require.NoError(t, ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", sourceID).Count(&sourceVersions).Error)
	assert.Equal(t, int64(0), sourceVersions, "cropping to a new resource must not version the source")

	assert.Equal(t, uint(40), created.Width)
	assert.Equal(t, uint(30), created.Height)
	assert.Equal(t, "image/jpeg", created.ContentType)
	assert.Contains(t, created.Name, "(cropped)")
	assert.Contains(t, created.Description, "#"+strconv.FormatUint(uint64(sourceID), 10),
		"the new resource must record where it was cropped from")

	// AddResource's own v1 bookkeeping applies to the crop like any other upload.
	var newVersions []models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ?", created.ID).Order("version_number").Find(&newVersions).Error)
	require.Len(t, newVersions, 1)
	assert.Equal(t, 1, newVersions[0].VersionNumber)
	assert.Equal(t, uint(40), newVersions[0].Width)
	assert.Equal(t, uint(30), newVersions[0].Height)
}

func TestCropResourceToNewResource_InheritsOwnerGroupsAndTags(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 60, 60, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 60, 60)

	extraGroup := &models.Group{Name: "crop-extra-group"}
	require.NoError(t, ctx.db.Create(extraGroup).Error)
	tag := &models.Tag{Name: "crop-tag"}
	require.NoError(t, ctx.db.Create(tag).Error)

	var source models.Resource
	require.NoError(t, ctx.db.First(&source, sourceID).Error)
	require.NoError(t, ctx.db.Model(&source).Association("Groups").Append(&[]*models.Group{extraGroup}))
	require.NoError(t, ctx.db.Model(&source).Association("Tags").Append(&[]*models.Tag{tag}))

	created, err := ctx.CropResourceToNewResource(context.Background(), sourceID, 0, 0, 20, 20, "")
	require.NoError(t, err)

	var loaded models.Resource
	require.NoError(t, ctx.db.Preload("Groups").Preload("Tags").First(&loaded, created.ID).Error)

	require.NotNil(t, loaded.OwnerId)
	require.NotNil(t, source.OwnerId)
	assert.Equal(t, *source.OwnerId, *loaded.OwnerId, "owner must carry over so the crop stays in the same subtree")

	groupIDs := make([]uint, 0, len(loaded.Groups))
	for _, g := range loaded.Groups {
		groupIDs = append(groupIDs, g.ID)
	}
	assert.Contains(t, groupIDs, extraGroup.ID)

	require.Len(t, loaded.Tags, 1)
	assert.Equal(t, tag.ID, loaded.Tags[0].ID)
}

func TestCropResourceToNewResource_UsesCommentInDescription(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 50, 50, color.RGBA{R: 90, G: 90, B: 200, A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 50, 50)

	created, err := ctx.CropResourceToNewResource(context.Background(), sourceID, 0, 0, 10, 10, "  headshot crop  ")
	require.NoError(t, err)
	assert.Contains(t, created.Description, "headshot crop")
}

func TestCropResourceToNewResource_DuplicateContentIsRefused(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 70, 70, color.RGBA{R: 5, G: 150, B: 250, A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 70, 70)

	first, err := ctx.CropResourceToNewResource(context.Background(), sourceID, 0, 0, 25, 25, "")
	require.NoError(t, err)

	// Identical rectangle, identical bytes: AddResource's hash dedupe owns this,
	// and it names the resource the caller already has instead of twinning it.
	_, err = ctx.CropResourceToNewResource(context.Background(), sourceID, 0, 0, 25, 25, "")
	require.Error(t, err)
	var existsErr *ResourceExistsError
	require.ErrorAs(t, err, &existsErr)
	assert.Equal(t, first.ID, existsErr.ResourceID)
}

func TestCropResourceToNewResource_ScopedPrincipalDropsOutOfSubtreeGroups(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 60, 60, color.RGBA{R: 30, G: 170, B: 90, A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 60, 60)

	var source models.Resource
	require.NoError(t, ctx.db.First(&source, sourceID).Error)
	require.NotNil(t, source.OwnerId)
	scopeGroupID := *source.OwnerId

	// The source is inside the principal's subtree by owner, but also belongs to
	// a group outside it — a resource can be in groups its owner has nothing to
	// do with. Inheriting that group would write into a subtree the principal
	// cannot see; refusing the crop outright would block legitimate work.
	outside := &models.Group{Name: "crop-outside-subtree"}
	require.NoError(t, ctx.db.Create(outside).Error)
	// A second group inside the subtree, so "dropped every group" cannot pass
	// this test: the in-scope one has to survive the filtering.
	inside := &models.Group{Name: "crop-inside-subtree", OwnerId: &scopeGroupID}
	require.NoError(t, ctx.db.Create(inside).Error)
	require.NoError(t, ctx.db.Model(&source).Association("Groups").Append(&[]*models.Group{outside, inside}))

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, ScopeGroupID: &scopeGroupID})

	created, err := scoped.CropResourceToNewResource(context.Background(), sourceID, 0, 0, 20, 20, "")
	require.NoError(t, err)

	var loaded models.Resource
	require.NoError(t, ctx.db.Preload("Groups").First(&loaded, created.ID).Error)
	groupIDs := make([]uint, 0, len(loaded.Groups))
	for _, g := range loaded.Groups {
		groupIDs = append(groupIDs, g.ID)
	}
	assert.NotContains(t, groupIDs, outside.ID, "a group outside the principal's subtree must not be inherited")
	assert.Contains(t, groupIDs, inside.ID, "a group inside the subtree must still be inherited")
	require.NotNil(t, loaded.OwnerId)
	assert.Equal(t, scopeGroupID, *loaded.OwnerId, "the crop stays inside the subtree it came from")
}

func TestCropResourceToNewResource_DoesNotRefileAnUnrelatedMatch(t *testing.T) {
	ctx := setupCropTestCtx(t)

	// Two different images that agree exactly on the region being cropped, so the
	// crops come out byte-identical while the sources do not. PNG, not JPEG: the
	// lossy encoder bleeds the colour boundary into the cropped quadrant, so two
	// JPEG sources never produce identical crops. Each seeded source gets its own
	// owner group.
	red := color.RGBA{R: 220, G: 20, B: 20, A: 255}
	blue := color.RGBA{R: 20, G: 20, B: 220, A: 255}
	solid := makePNG(t, 60, 60, red)

	mixed := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for x := 0; x < 60; x++ {
		for y := 0; y < 60; y++ {
			if x < 20 && y < 20 {
				mixed.Set(x, y, red)
			} else {
				mixed.Set(x, y, blue)
			}
		}
	}
	var mixedBuf bytes.Buffer
	require.NoError(t, png.Encode(&mixedBuf, mixed))

	firstID := seedImageResource(t, ctx, "image/png", solid, 60, 60)
	secondID := seedImageResource(t, ctx, "image/png", mixedBuf.Bytes(), 60, 60)

	first, err := ctx.CropResourceToNewResource(context.Background(), firstID, 0, 0, 20, 20, "")
	require.NoError(t, err)

	var ownerBefore models.Resource
	require.NoError(t, ctx.db.Preload("Groups").First(&ownerBefore, first.ID).Error)
	groupsBefore := len(ownerBefore.Groups)

	// AddResource's upload-oriented collision branch would file the existing
	// resource into the second source's owner group and hand it back with no
	// error — the caller would be told a crop was created when nothing was, and
	// an unrelated resource would have quietly gained a group.
	created, err := ctx.CropResourceToNewResource(context.Background(), secondID, 0, 0, 20, 20, "")
	require.Error(t, err)
	assert.Nil(t, created)
	var existsErr *ResourceExistsError
	require.ErrorAs(t, err, &existsErr)
	assert.Equal(t, first.ID, existsErr.ResourceID)

	var ownerAfter models.Resource
	require.NoError(t, ctx.db.Preload("Groups").First(&ownerAfter, first.ID).Error)
	assert.Len(t, ownerAfter.Groups, groupsBefore, "the refused crop must not re-file the resource it collided with")
}

func TestCropResourceToNewResource_RejectsInvalidInput(t *testing.T) {
	ctx := setupCropTestCtx(t)
	jpegBytes := makeJPEG(t, 100, 100, color.RGBA{A: 255})
	sourceID := seedImageResource(t, ctx, "image/jpeg", jpegBytes, 100, 100)

	cases := []struct {
		name       string
		x, y, w, h int
		contains   string
	}{
		{"zero width", 0, 0, 0, 10, "width must be positive"},
		{"negative origin", -1, 0, 10, 10, "origin must be non-negative"},
		{"out of bounds", 50, 0, 60, 10, "must be within image bounds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := ctx.CropResourceToNewResource(context.Background(), sourceID, tc.x, tc.y, tc.w, tc.h, "")
			require.Error(t, err)
			assert.Nil(t, created)
			assert.Contains(t, err.Error(), tc.contains)
		})
	}

	t.Run("non-raster source", func(t *testing.T) {
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="red"/></svg>`)
		svgID := seedImageResource(t, ctx, "image/svg+xml", svg, 40, 40)
		created, err := ctx.CropResourceToNewResource(context.Background(), svgID, 0, 0, 10, 10, "")
		require.Error(t, err)
		assert.Nil(t, created)
		assert.Contains(t, err.Error(), "not a raster image format")
		assert.Contains(t, err.Error(), "image/svg+xml")
	})
}

func TestCropResource_TIFF_ConvertsToPNG(t *testing.T) {
	ctx := setupCropTestCtx(t)
	src := image.NewRGBA(image.Rect(0, 0, 30, 20))
	for x := 0; x < 30; x++ {
		for y := 0; y < 20; y++ {
			src.Set(x, y, color.RGBA{R: 40, G: 40, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, tiff.Encode(&buf, src, nil))
	resourceID := seedImageResource(t, ctx, "image/tiff", buf.Bytes(), 30, 20)

	require.NoError(t, ctx.CropResource(context.Background(), resourceID, 0, 0, 10, 10, ""))

	var v models.ResourceVersion
	require.NoError(t, ctx.db.Where("resource_id = ? AND version_number = ?", resourceID, 2).First(&v).Error)
	assert.Equal(t, "image/png", v.ContentType)
	assert.Contains(t, v.Comment, "re-encoded as PNG")
}
