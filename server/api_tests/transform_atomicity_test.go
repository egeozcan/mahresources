package api_tests

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mahresources/models"
)

// RotateResource, CropResource and TrimVideo all back-fill a "version 1" row for
// resources that predate the versioning system. That INSERT ran on ctx.db —
// outside the transaction that follows it — so a failure while writing the new
// version left the back-filled v1 committed on its own. The resource then owned
// a version history it never had before, describing a transform that did not
// happen.
//
// The fault is injected through a GORM callback rather than simulated, so the
// test exercises the real rollback path.

// failResourceVersionCreate makes every ResourceVersion INSERT whose
// VersionNumber is n fail, and returns a function that removes the fault.
func failResourceVersionCreate(t *testing.T, db *gorm.DB, n int) func() {
	t.Helper()
	const name = "test:fail_version_create"
	err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		version, ok := tx.Statement.Dest.(*models.ResourceVersion)
		if !ok || version.VersionNumber != n {
			return
		}
		tx.AddError(errors.New("injected failure writing resource version"))
	})
	require.NoError(t, err)
	return func() { _ = db.Callback().Create().Remove(name) }
}

// seedUnversionedImage creates a resource with a real PNG on disk and no
// ResourceVersion rows — the pre-versioning shape that triggers the back-fill.
func seedUnversionedImage(t *testing.T, tc *TestContext, name string) *models.Resource {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: 90, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	resource := writeResourceFile(t, tc, name, "image/png", buf.Bytes(), 8, 8)

	var count int64
	require.NoError(t, tc.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ?", resource.ID).Count(&count).Error)
	require.Zero(t, count, "fixture should start with no versions")

	return resource
}

func TestRotateResource_FailedTransformLeavesNoVersionRows(t *testing.T) {
	tc := SetupTestEnv(t)
	require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

	resource := seedUnversionedImage(t, tc, "rotate-atomic.png")
	before := snapshotResource(t, tc, resource.ID)

	// Version 2 is the rotated one; failing it means the back-filled v1 has
	// already been written by the time the error surfaces.
	restore := failResourceVersionCreate(t, tc.DB, 2)
	defer restore()

	err := tc.AppCtx.RotateResource(context.Background(), resource.ID, 90)
	require.Error(t, err, "the injected failure should surface")

	assertNoVersionsAndUnchanged(t, tc, resource.ID, before)
}

func TestCropResource_FailedTransformLeavesNoVersionRows(t *testing.T) {
	tc := SetupTestEnv(t)
	require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

	resource := seedUnversionedImage(t, tc, "crop-atomic.png")
	before := snapshotResource(t, tc, resource.ID)

	restore := failResourceVersionCreate(t, tc.DB, 2)
	defer restore()

	err := tc.AppCtx.CropResource(context.Background(), resource.ID, 0, 0, 4, 4, "")
	require.Error(t, err, "the injected failure should surface")

	assertNoVersionsAndUnchanged(t, tc, resource.ID, before)
}

// TestTransforms_SucceedNormally is the positive control: with no fault
// injected, the very same call must create both the back-filled v1 and the new
// version. Without this, a change that made every transform fail early would
// satisfy the assertions above forever.
func TestTransforms_SucceedNormally(t *testing.T) {
	t.Run("rotate", func(t *testing.T) {
		tc := SetupTestEnv(t)
		require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))
		resource := seedUnversionedImage(t, tc, "rotate-ok.png")

		require.NoError(t, tc.AppCtx.RotateResource(context.Background(), resource.ID, 90))

		var versions []models.ResourceVersion
		require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).
			Order("version_number").Find(&versions).Error)
		require.Len(t, versions, 2, "back-filled v1 plus the rotated v2")
		assert.Equal(t, 1, versions[0].VersionNumber)
		assert.Contains(t, versions[0].Comment, "Original")
		assert.Equal(t, 2, versions[1].VersionNumber)
	})

	t.Run("crop", func(t *testing.T) {
		tc := SetupTestEnv(t)
		require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))
		resource := seedUnversionedImage(t, tc, "crop-ok.png")

		require.NoError(t, tc.AppCtx.CropResource(context.Background(), resource.ID, 0, 0, 4, 4, ""))

		var versions []models.ResourceVersion
		require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).
			Order("version_number").Find(&versions).Error)
		require.Len(t, versions, 2, "back-filled v1 plus the cropped v2")
		assert.Equal(t, 1, versions[0].VersionNumber)
		assert.Contains(t, versions[0].Comment, "Original")
	})
}

type resourceSnapshot struct {
	Hash        string
	Location    string
	ContentType string
	Width       uint
	Height      uint
	FileSize    int64
}

func snapshotResource(t *testing.T, tc *TestContext, id uint) resourceSnapshot {
	t.Helper()
	var r models.Resource
	require.NoError(t, tc.DB.First(&r, id).Error)
	return resourceSnapshot{
		Hash: r.Hash, Location: r.Location, ContentType: r.ContentType,
		Width: r.Width, Height: r.Height, FileSize: r.FileSize,
	}
}

// assertNoVersionsAndUnchanged is the whole invariant: a transform that failed
// leaves the resource exactly as it was, with no version history invented for
// it along the way.
func assertNoVersionsAndUnchanged(t *testing.T, tc *TestContext, id uint, before resourceSnapshot) {
	t.Helper()

	var versions []models.ResourceVersion
	require.NoError(t, tc.DB.Where("resource_id = ?", id).Find(&versions).Error)
	assert.Emptyf(t, versions,
		"a failed transform left %d version row(s) behind; the back-filled v1 was committed outside the transaction",
		len(versions))

	assert.Equal(t, before, snapshotResource(t, tc, id),
		"a failed transform must not modify the resource row")
}
