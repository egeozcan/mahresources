package application_context

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/disintegration/imaging"
	"gorm.io/gorm"

	"mahresources/models"
)

// maxCustomThumbnailDimension is the upper bound (long edge, in pixels)
// applied to a user-uploaded custom thumbnail before it is stored. Anything
// larger is resized down; smaller images pass through unchanged. The value
// matches a typical lightbox-ready size and keeps the stored BLOB modest.
const maxCustomThumbnailDimension = 1920

// customThumbnailJPEGQuality is the JPEG quality used when re-encoding a
// user-uploaded image into the canonical Preview format.
const customThumbnailJPEGQuality = 85

// InvalidThumbnailError signals that an uploaded thumbnail could not be
// decoded as an image. Handlers should map this to HTTP 400.
type InvalidThumbnailError struct {
	Err error
}

func (e *InvalidThumbnailError) Error() string {
	if e.Err == nil {
		return "invalid thumbnail image"
	}
	return "invalid thumbnail image: " + e.Err.Error()
}

func (e *InvalidThumbnailError) Unwrap() error {
	return e.Err
}

// SetCustomThumbnail replaces all previews for the given resource with a
// single canonical "null thumbnail" (Width=0, Height=0) generated from the
// user-supplied image data. Resizes the upload down to maxCustomThumbnailDimension
// on its long edge before storing, and re-encodes as JPEG.
//
// The upload is validated by attempting to decode it; undecodable input
// returns *InvalidThumbnailError. Storage is transactional: existing previews
// are deleted only if the new row insert succeeds.
func (ctx *MahresourcesContext) SetCustomThumbnail(
	httpContext context.Context,
	resourceID uint,
	reader io.Reader,
) error {
	if resourceID == 0 {
		return errors.New("resource id is required")
	}

	// Confirm the resource exists so we surface a clean error rather than
	// orphaning a Preview row referencing a missing ID.
	var resource models.Resource
	if err := ctx.db.WithContext(httpContext).First(&resource, resourceID).Error; err != nil {
		return err
	}

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read uploaded thumbnail: %w", err)
	}
	if len(raw) == 0 {
		return &InvalidThumbnailError{Err: errors.New("empty upload")}
	}

	img, _, decodeErr := image.Decode(bytes.NewReader(raw))
	if decodeErr != nil {
		return &InvalidThumbnailError{Err: decodeErr}
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return &InvalidThumbnailError{Err: errors.New("decoded image has zero dimensions")}
	}

	// Resize down if either edge exceeds the cap. imaging.Resize preserves
	// aspect ratio when one axis is 0.
	if w > maxCustomThumbnailDimension || h > maxCustomThumbnailDimension {
		if w >= h {
			img = imaging.Resize(img, maxCustomThumbnailDimension, 0, imaging.Lanczos)
		} else {
			img = imaging.Resize(img, 0, maxCustomThumbnailDimension, imaging.Lanczos)
		}
	}

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(customThumbnailJPEGQuality)); err != nil {
		return fmt.Errorf("failed to encode custom thumbnail: %w", err)
	}

	resID := resourceID
	preview := &models.Preview{
		Data:        buf.Bytes(),
		Width:       0,
		Height:      0,
		ContentType: "image/jpeg",
		ResourceId:  &resID,
	}

	return ctx.db.WithContext(httpContext).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", resourceID).Delete(&models.Preview{}).Error; err != nil {
			return fmt.Errorf("failed to clear existing previews: %w", err)
		}
		if err := tx.Create(preview).Error; err != nil {
			return fmt.Errorf("failed to save custom thumbnail: %w", err)
		}
		return nil
	})
}

// ClearThumbnails deletes every Preview row for the resource. The next
// thumbnail GET re-runs the automatic pipeline (ffmpeg for videos, image
// decode for images, etc.).
func (ctx *MahresourcesContext) ClearThumbnails(
	httpContext context.Context,
	resourceID uint,
) error {
	if resourceID == 0 {
		return errors.New("resource id is required")
	}
	if err := ctx.db.WithContext(httpContext).
		Where("resource_id = ?", resourceID).
		Delete(&models.Preview{}).Error; err != nil {
		return fmt.Errorf("failed to clear thumbnails: %w", err)
	}
	return nil
}

// LatestPreviewVersion returns a monotonically-increasing token that changes
// whenever the resource's thumbnail set is mutated (set, cleared, or filled
// on demand). Callers use this in cache headers (ETag) so clients invalidate
// their cached thumbnails after a SetCustomThumbnail or ClearThumbnails.
//
// Implementation: max(id) over previews for the resource. New inserts
// monotonically increase the value; deletions drop it back to 0, which is
// also distinguishable from any prior populated state.
func (ctx *MahresourcesContext) LatestPreviewVersion(
	httpContext context.Context,
	resourceID uint,
) uint {
	var maxID uint
	row := ctx.db.WithContext(httpContext).
		Model(&models.Preview{}).
		Where("resource_id = ?", resourceID).
		Select("COALESCE(MAX(id), 0)").
		Row()
	_ = row.Scan(&maxID)
	return maxID
}
