package application_context

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// newThumbnailTestContext builds an isolated in-memory context for thumbnail tests.
// Uses a unique DSN so concurrent tests don't share state.
func newThumbnailTestContext(t *testing.T, name string) *MahresourcesContext {
	t.Helper()

	dsn := "file:" + name + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Group{}, &models.Category{}, &models.NoteType{}, &models.Preview{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.ResourceCategory{},
		&models.Series{}, &models.NoteBlock{}, &models.PluginKV{}, &models.ResourceVersion{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(fs, db, readOnlyDB, cfg)

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	ctx.DefaultResourceCategoryID = defaultRC.ID

	return ctx
}

// makeJPEGBytes builds a JPEG of the given dimensions filled with a simple
// color gradient so the bytes are non-trivial (not a uniform color).
func makeJPEGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 255),
				G: uint8(y % 255),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// addJPEGResource uploads a generated JPEG via AddResource so the resource row
// gets its Width/Height populated by the same code path as production.
func addJPEGResource(t *testing.T, ctx *MahresourcesContext, w, h int, name string) *models.Resource {
	t.Helper()
	data := makeJPEGBytes(t, w, h)
	owner := &models.Group{Name: name + "-owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}
	res, err := ctx.AddResource(newBytesFile(data), name+".jpg", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:    name,
			OwnerId: owner.ID,
		},
	})
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	if res.Width == 0 || res.Height == 0 {
		t.Fatalf("expected resource Width/Height to be populated; got %dx%d", res.Width, res.Height)
	}
	if int(res.Width) != w || int(res.Height) != h {
		t.Fatalf("resource dims mismatch: got %dx%d, want %dx%d", res.Width, res.Height, w, h)
	}
	return res
}

// decodeJPEGOrFail decodes JPEG bytes and returns the actual pixel dims.
func decodeJPEGOrFail(t *testing.T, data []byte) (int, int) {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode jpeg: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// TestThumbnail_SquareRequestDoesNotPollutePortraitLookup reproduces the
// original prod bug: a forced-square thumbnail (e.g. from MRQL) used to
// poison subsequent height-only requests via GORM's zero-value Where bug.
// After the fix, the second request must produce an aspect-correct thumbnail
// and the DB must not contain any (Width=0, Height>0) rows for the resource.
func TestThumbnail_SquareRequestDoesNotPollutePortraitLookup(t *testing.T) {
	ctx := newThumbnailTestContext(t, "thumb_pollute_test")
	res := addJPEGResource(t, ctx, 1920, 1080, "landscape")

	// First: force a 64×64 square (mimics MRQL/widget callers).
	square, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 64, 64, context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if square == nil {
		t.Fatal("first call returned nil preview")
	}
	if square.Width != 64 || square.Height != 64 {
		t.Errorf("forced-square thumbnail stored as %dx%d; want 64x64", square.Width, square.Height)
	}

	// Second: ask for height=400 with width unspecified — must NOT inherit the square aspect.
	tall, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 0, 400, context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if tall == nil {
		t.Fatal("second call returned nil preview")
	}

	// The stored row must reflect the actual JPEG dimensions, not (0, 400).
	actualW, actualH := decodeJPEGOrFail(t, tall.Data)
	if actualW != int(tall.Width) || actualH != int(tall.Height) {
		t.Errorf("stored dims (%d,%d) do not match decoded JPEG dims (%d,%d)",
			tall.Width, tall.Height, actualW, actualH)
	}
	if tall.Width == tall.Height {
		t.Errorf("height-only request returned a square thumbnail %dx%d", tall.Width, tall.Height)
	}
	// 1920×1080 → height=400 → width≈711 with no implicit cap on derived axis.
	if actualH != 400 {
		t.Errorf("expected actual height=400; got %d", actualH)
	}
	expectedW := 711
	if actualW != expectedW {
		t.Errorf("expected actual width=%d for 16:9 at h=400; got %d", expectedW, actualW)
	}

	// Verify no zero-width row was persisted by the new save path.
	var zeroWidthCount int64
	if err := ctx.db.Model(&models.Preview{}).
		Where("resource_id = ? AND width = 0", res.ID).
		Count(&zeroWidthCount).Error; err != nil {
		t.Fatalf("count zero-width previews: %v", err)
	}
	if zeroWidthCount != 0 {
		t.Errorf("expected 0 zero-width preview rows; got %d", zeroWidthCount)
	}
}

// TestThumbnail_CrossAspectPollutedRowIgnored simulates a row left over from
// the buggy prod code: width=0, height=400, data is a 400×400 square. The
// new lookup uses the computed actual dims (e.g. (711, 400)) so the polluted
// row is never returned.
func TestThumbnail_CrossAspectPollutedRowIgnored(t *testing.T) {
	ctx := newThumbnailTestContext(t, "thumb_polluted_row_test")
	res := addJPEGResource(t, ctx, 1920, 1080, "landscape2")

	// Insert a malformed row that mimics what the old buggy code would persist.
	bogusSquare := makeJPEGBytes(t, 400, 400)
	bogus := &models.Preview{
		Data:        bogusSquare,
		Width:       0,
		Height:      400,
		ContentType: "image/jpeg",
		ResourceId:  &res.ID,
	}
	if err := ctx.db.Save(bogus).Error; err != nil {
		t.Fatalf("insert bogus row: %v", err)
	}

	// Request the same height. The new code computes actual target dims
	// (711, 400) for a 16:9 source, so it should NOT pick up the (0, 400) row.
	out, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 0, 400, context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreateThumbnailForResource: %v", err)
	}
	if out.ID == bogus.ID {
		t.Fatal("returned the polluted (0, 400) row instead of generating a fresh one")
	}
	if out.Width == out.Height {
		t.Errorf("served a square thumbnail %dx%d for a 16:9 resource", out.Width, out.Height)
	}
	w, h := decodeJPEGOrFail(t, out.Data)
	if w == h {
		t.Errorf("decoded JPEG was square %dx%d; expected aspect-preserving", w, h)
	}
}

// TestThumbnail_ImageRegeneratesFromOriginalNotCachedThumb proves Rule 1:
// even though a small cached preview exists, asking for a different-size
// thumbnail regenerates from the original file rather than reusing the cache.
// We use 500×281 (≈16:9, within MaxThumb=600 cap) so the comparison stays
// honest and doesn't get clamped.
func TestThumbnail_ImageRegeneratesFromOriginalNotCachedThumb(t *testing.T) {
	ctx := newThumbnailTestContext(t, "thumb_no_upscale_test")
	res := addJPEGResource(t, ctx, 1920, 1080, "for-upscale-check")

	// Generate a small 64×64 thumbnail first.
	small, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 64, 64, context.Background())
	if err != nil {
		t.Fatalf("small thumb: %v", err)
	}

	// Now ask for 500×281 (much larger, but within MaxThumb cap). With Rule 1
	// enforced, we must regenerate from the original — never upscale from 64×64.
	large, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 500, 281, context.Background())
	if err != nil {
		t.Fatalf("large thumb: %v", err)
	}
	if large.ID == small.ID {
		t.Fatal("large request returned the small cached preview ID")
	}
	if len(large.Data) <= len(small.Data) {
		t.Errorf("large (%d bytes) is not bigger than small (%d bytes); likely served from cache",
			len(large.Data), len(small.Data))
	}
	w, h := decodeJPEGOrFail(t, large.Data)
	if w != 500 || h != 281 {
		t.Errorf("expected 500x281; got %dx%d", w, h)
	}
}

// TestThumbnail_ExactDimsHitCache verifies that requesting the same dimensions
// twice returns the same Preview row without inserting a duplicate.
func TestThumbnail_ExactDimsHitCache(t *testing.T) {
	ctx := newThumbnailTestContext(t, "thumb_cache_hit_test")
	res := addJPEGResource(t, ctx, 1920, 1080, "cache-hit")

	first, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 0, 400, context.Background())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ctx.LoadOrCreateThumbnailForResource(res.ID, 0, 400, context.Background())
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("expected same Preview ID on second call; got %d then %d", first.ID, second.ID)
	}

	var count int64
	if err := ctx.db.Model(&models.Preview{}).
		Where("resource_id = ?", res.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count previews: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 preview row; got %d", count)
	}
}
