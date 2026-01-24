package application_context

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"
	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// LoadOrCreateThumbnailForResource generates or retrieves a thumbnail for a given resource.
// It respects the provided context for cancellation and timeout.
func (ctx *MahresourcesContext) LoadOrCreateThumbnailForResource(
	resourceId, width, height uint,
	httpContext context.Context,
) (*models.Preview, error) {
	// Acquire the ThumbnailGenerationLock for the given resourceId with context support
	if err := ctx.locks.ThumbnailGenerationLock.AcquireContext(httpContext, resourceId); err != nil {
		return nil, fmt.Errorf("failed to acquire thumbnail generation lock: %w", err)
	}
	defer ctx.locks.ThumbnailGenerationLock.Release(resourceId)

	// Ensure width and height do not exceed maximum allowed values
	width = uint(math.Min(constants.MaxThumbWidth, float64(width)))
	height = uint(math.Min(constants.MaxThumbHeight, float64(height)))

	// Attempt to retrieve an existing thumbnail with the specified dimensions
	existingThumbnail, err := ctx.getExistingThumbnail(resourceId, width, height, httpContext)
	if err == nil {
		return &existingThumbnail, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("error retrieving existing thumbnail: %w", err)
	}

	// Retrieve the resource from the database
	resource, err := ctx.getResourceForThumbnail(resourceId, httpContext)
	if err != nil {
		return nil, fmt.Errorf("error retrieving resource: %w", err)
	}

	// Get the filesystem interface for the resource's storage location
	fs, storageError := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if storageError != nil {
		return nil, fmt.Errorf("error getting filesystem: %w", storageError)
	}

	// Attempt to find or create a null thumbnail (original image without size)
	nullThumbnail, fileBytes, err := ctx.getOrCreateNullThumbnail(resource, fs, httpContext)
	if err != nil {
		return nil, fmt.Errorf("error handling null thumbnail: %w", err)
	}

	// Depending on the resource's content type, generate the appropriate thumbnail
	if nullThumbnail.ID != 0 {
		// If a null thumbnail exists, resize it to the desired dimensions
		fileBytes, err = ctx.generateImageThumbnail(nullThumbnail.Data, width, height)
		if err != nil {
			return nil, fmt.Errorf("error generating image thumbnail from null thumbnail: %w", err)
		}
	} else if strings.HasPrefix(resource.ContentType, "image/") {
		// Handle image resources by generating a thumbnail directly from the image file
		fileBytes, err = ctx.generateImageThumbnailFromFile(fs, resource.GetCleanLocation(), width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating image thumbnail from file: %w", err)
		}
	} else if strings.HasPrefix(resource.ContentType, "video/") {
		// Handle video resources by generating a thumbnail from the video
		fileBytes, err = ctx.generateVideoThumbnail(resource, fs, width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating video thumbnail: %w", err)
		}
	} else {
		// Unsupported content type; no thumbnail to generate
		return nil, nil
	}

	// Create and save the new preview (thumbnail) to the database
	preview := &models.Preview{
		Data:        fileBytes,
		Width:       width,
		Height:      height,
		ContentType: "image/jpeg",
		ResourceId:  &resource.ID,
	}

	if err := ctx.db.WithContext(httpContext).Save(preview).Error; err != nil {
		return nil, fmt.Errorf("error saving new thumbnail to database: %w", err)
	}

	return preview, nil
}

////////////////////////////////////////////////////////////////////////////////
// Helper Functions
////////////////////////////////////////////////////////////////////////////////

// getExistingThumbnail retrieves an existing thumbnail with the specified dimensions.
func (ctx *MahresourcesContext) getExistingThumbnail(
	resourceId, width, height uint,
	httpContext context.Context,
) (models.Preview, error) {
	var thumbnail models.Preview
	err := ctx.db.WithContext(httpContext).
		Where(&models.Preview{
			Width:      width,
			Height:     height,
			ResourceId: &resourceId,
		}).
		Omit(clause.Associations).
		First(&thumbnail).Error
	return thumbnail, err
}

// getResourceForThumbnail retrieves the resource from the database for thumbnail generation.
func (ctx *MahresourcesContext) getResourceForThumbnail(
	resourceId uint,
	httpContext context.Context,
) (models.Resource, error) {
	var resource models.Resource
	err := ctx.db.WithContext(httpContext).
		Omit(clause.Associations).
		First(&resource, resourceId).Error
	return resource, err
}

// getOrCreateNullThumbnail attempts to retrieve a null thumbnail or create one if it doesn't exist.
func (ctx *MahresourcesContext) getOrCreateNullThumbnail(
	resource models.Resource,
	fs afero.Fs,
	httpContext context.Context,
) (models.Preview, []byte, error) {
	var nullThumbnail models.Preview
	var fileBytes []byte

	err := ctx.db.WithContext(httpContext).
		Where(&models.Preview{
			Width:      0,
			Height:     0,
			ResourceId: &resource.ID,
		}).
		Omit(clause.Associations).
		First(&nullThumbnail).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Null thumbnail doesn't exist; attempt to create it from the original image
		name := resource.GetCleanLocation() + constants.ThumbFileSuffix
		fmt.Println("Attempting to open", name)

		file, fopenErr := fs.Open(name)
		if fopenErr != nil {
			fmt.Println("Failed to open file:", fopenErr)
			return nullThumbnail, fileBytes, nil // Return empty fileBytes; no null thumbnail
		}
		defer file.Close()

		// Read the file bytes
		fileBytes, readErr := io.ReadAll(file)
		if readErr != nil {
			return nullThumbnail, fileBytes, fmt.Errorf("failed to read null thumbnail file: %w", readErr)
		}

		// Initialize nullThumbnail correctly
		nullThumbnail = models.Preview{
			Data:        fileBytes,
			Width:       0,
			Height:      0,
			ContentType: "image/jpeg",
			ResourceId:  &resource.ID,
		}

		// Save the null thumbnail to the database
		if saveErr := ctx.db.WithContext(httpContext).Save(&nullThumbnail).Error; saveErr != nil {
			return nullThumbnail, fileBytes, fmt.Errorf("failed to save null thumbnail to database: %w", saveErr)
		}
	} else if err != nil {
		// An unexpected error occurred while retrieving the null thumbnail
		return nullThumbnail, fileBytes, fmt.Errorf("error retrieving null thumbnail: %w", err)
	}

	return nullThumbnail, fileBytes, nil
}

// getJPEGQuality returns an appropriate JPEG quality based on thumbnail dimensions.
// Smaller thumbnails can use lower quality without noticeable artifacts.
func getJPEGQuality(width, height uint) int {
	maxDim := width
	if height > width {
		maxDim = height
	}

	switch {
	case maxDim <= 100:
		return 70
	case maxDim <= 200:
		return 75
	case maxDim <= 400:
		return 80
	default:
		return 85
	}
}

// generateImageThumbnail resizes the provided image data to the desired dimensions.
func (ctx *MahresourcesContext) generateImageThumbnail(
	imageData []byte,
	width, height uint,
) ([]byte, error) {
	originalImage, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image data: %w", err)
	}

	// Use imaging library for faster, high-quality resize with Lanczos filter
	newImage := imaging.Resize(originalImage, int(width), int(height), imaging.Lanczos)

	// Use adaptive JPEG quality based on dimensions
	quality := getJPEGQuality(width, height)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, newImage, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("failed to encode resized image: %w", err)
	}

	return buf.Bytes(), nil
}

// decodeImageWithFallback attempts to decode an image using Go's standard decoders,
// falling back to ImageMagick for unsupported formats like HEIC/AVIF.
func (ctx *MahresourcesContext) decodeImageWithFallback(
	httpContext context.Context,
	file io.ReadSeeker,
) (image.Image, error) {
	// First, try standard Go decoders (PNG, JPEG, GIF, WebP, BMP, TIFF)
	img, _, err := image.Decode(file)
	if err == nil {
		return img, nil
	}

	// Reset file position for fallback attempt
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return nil, fmt.Errorf("failed to seek file: %w", seekErr)
	}

	// Try ImageMagick as fallback for HEIC, AVIF, and other formats
	img, fallbackErr := ctx.decodeWithImageMagick(httpContext, file)
	if fallbackErr == nil {
		return img, nil
	}

	// Return original error if all decoders fail
	return nil, fmt.Errorf("failed to decode image (tried standard decoders and ImageMagick): %w", err)
}

// decodeWithImageMagick uses ImageMagick's convert command to decode unsupported formats.
func (ctx *MahresourcesContext) decodeWithImageMagick(
	httpContext context.Context,
	file io.Reader,
) (image.Image, error) {
	// Check if ImageMagick is available by looking for 'convert' or 'magick' command
	convertPath := "convert"
	if _, err := exec.LookPath("magick"); err == nil {
		convertPath = "magick"
	} else if _, err := exec.LookPath("convert"); err == nil {
		convertPath = "convert"
	} else {
		return nil, errors.New("ImageMagick not available")
	}

	// Use ImageMagick to convert to PNG (lossless, supports transparency)
	cmd := exec.CommandContext(httpContext, convertPath,
		"-",      // read from stdin
		"-strip", // remove metadata for smaller output
		"png:-",  // output PNG to stdout
	)

	cmd.Stdin = file
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if httpContext.Err() != nil {
			return nil, httpContext.Err()
		}
		return nil, fmt.Errorf("ImageMagick conversion failed: %w (stderr: %s)", err, stderr.String())
	}

	// Decode the PNG output
	img, _, err := image.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ImageMagick output: %w", err)
	}

	return img, nil
}

// generateImageThumbnailFromFile generates a thumbnail from the image file at the given location.
func (ctx *MahresourcesContext) generateImageThumbnailFromFile(
	fs afero.Fs,
	location string,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	file, err := fs.Open(location)
	if err != nil {
		return nil, fmt.Errorf("failed to open image file: %w", err)
	}
	defer file.Close()

	// Use fallback decoder for better format support (HEIC, AVIF, etc.)
	originalImage, err := ctx.decodeImageWithFallback(httpContext, file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image file: %w", err)
	}

	// Use imaging library for faster, high-quality resize with Lanczos filter
	newImage := imaging.Resize(originalImage, int(width), int(height), imaging.Lanczos)

	// Use adaptive JPEG quality based on dimensions
	quality := getJPEGQuality(width, height)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, newImage, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("failed to encode resized image: %w", err)
	}

	return buf.Bytes(), nil
}

// generateVideoThumbnail generates a thumbnail from the video resource.
func (ctx *MahresourcesContext) generateVideoThumbnail(
	resource models.Resource,
	fs afero.Fs,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	// Determine runTimeout based on context's deadline
	runTimeout := 10 * time.Second // default run timeout

	// Check if the context has a deadline and adjust the runTimeout accordingly
	if deadline, ok := httpContext.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < runTimeout {
			runTimeout = remaining
		}
	}

	var fileBytes []byte

	// Execute thumbnail generation with lock and timeout using RunWithLockTimeout
	lockAcquired, err := ctx.locks.VideoThumbnailGenerationLock.RunWithLockTimeout(
		resource.ID,    // Assuming resourceId is of type T comparable (uint)
		30*time.Second, // lockTimeout
		runTimeout,     // runTimeout
		func() error {
			// Perform thumbnail generation
			file, err := fs.Open(resource.GetCleanLocation())
			if err != nil {
				return fmt.Errorf("failed to open video file: %w", err)
			}
			defer file.Close()

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Create thumbnail from video
			resultBuffer := bytes.NewBuffer([]byte{})
			if err := ctx.createThumbFromVideo(httpContext, file, resultBuffer); err != nil {
				return fmt.Errorf("failed to create thumbnail from video: %w", err)
			}

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Decode the generated thumbnail image
			originalImage, _, err := image.Decode(resultBuffer)
			if err != nil {
				return fmt.Errorf("failed to decode thumbnail image: %w", err)
			}

			// Use imaging library for faster, high-quality resize with Lanczos filter
			newImage := imaging.Resize(originalImage, int(width), int(height), imaging.Lanczos)

			// Use adaptive JPEG quality based on dimensions
			quality := getJPEGQuality(width, height)
			var buf bytes.Buffer
			if err := imaging.Encode(&buf, newImage, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
				return fmt.Errorf("failed to encode resized image: %w", err)
			}

			fileBytes = buf.Bytes()

			// Indicate success
			return nil
		},
	)

	// Check if the lock was successfully acquired
	if !lockAcquired {
		return nil, errors.New("failed to acquire video thumbnail generation lock")
	}

	// Handle potential errors from thumbnail generation
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("video thumbnail generation timed out")
		}
		return nil, fmt.Errorf("video thumbnail generation error: %w", err)
	}

	return fileBytes, nil
}

// createThumbFromVideo generates a thumbnail from a video file.
// It attempts to create a thumbnail at 1 second, and if that fails or returns no data,
// it retries at 0 seconds.
func (ctx *MahresourcesContext) createThumbFromVideo(
	context context.Context,
	file io.ReadSeeker,
	resultBuffer *bytes.Buffer,
) error {
	// First attempt to create thumbnail at 1 second
	err := ctx.createThumbFromVideoAtGivenTime(context, file, resultBuffer, 1)

	// If the first attempt fails or returns no data, try again at 0 seconds
	if err != nil || resultBuffer.Len() == 0 {
		resultBuffer.Reset()

		// Reset the reader back to the beginning
		_, err := file.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek video file: %w", err)
		}

		err = ctx.createThumbFromVideoAtGivenTime(context, file, resultBuffer, 0)
		if err != nil {
			return fmt.Errorf("failed to create thumbnail at 0 seconds: %w", err)
		}
	}

	return nil
}

// createThumbFromVideoAtGivenTime generates a thumbnail from a specific time in the video.
func (ctx *MahresourcesContext) createThumbFromVideoAtGivenTime(
	context context.Context,
	file io.Reader,
	resultBuffer *bytes.Buffer,
	secondsIn int,
) error {
	// Construct the ffmpeg command with context support
	cmd := exec.CommandContext(context, ctx.Config.FfmpegPath,
		"-i", "pipe:0", // input from stdin
		"-ss", fmt.Sprintf("00:00:%02d", secondsIn), // capture frame at secondsIn
		"-vframes", "1", // grab one frame
		"-vf", "scale=640:-1", // scale the image if needed
		"-c:v", "png", // encode to PNG
		"-f", "image2pipe", // output format
		"pipe:1", // output to stdout (pipe)
	)

	// Pipe the video data to ffmpeg's stdin
	cmd.Stdin = file

	// Set ffmpeg's stdout to write to resultBuffer
	cmd.Stdout = resultBuffer

	// Optionally, capture stderr for debugging purposes
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the ffmpeg command
	err := cmd.Run()
	if err != nil {
		// Check if the error is due to context cancellation
		if context.Err() != nil {
			return context.Err()
		}
		return fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

func (ctx *MahresourcesContext) GetFsForStorageLocation(storageLocation *string) (afero.Fs, error) {
	if storageLocation != nil {
		altFs, ok := ctx.altFileSystems[*storageLocation]

		if !ok {
			return nil, errors.New(fmt.Sprintf("alt fs '%v' is not attached", *storageLocation))
		}

		return altFs, nil
	}

	return ctx.fs, nil
}

func (ctx *MahresourcesContext) RecalculateResourceDimensions(query *query_models.EntityIdQuery) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, query.ID).Error; err != nil {
		return err
	}

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)

	if storageErr != nil {
		return storageErr
	}

	file, openErr := fs.Open(resource.GetCleanLocation())

	if openErr != nil {
		return openErr
	}

	defer file.Close()

	img, _, err := image.Decode(file)

	if err != nil {
		return err
	}

	bounds := img.Bounds()

	resource.Width = uint(bounds.Max.X)
	resource.Height = uint(bounds.Max.Y)

	return ctx.db.Save(&resource).Error
}

func (ctx *MahresourcesContext) SetResourceDimensions(resourceId uint, width, height uint) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return err
	}

	resource.Width = width
	resource.Height = height

	return ctx.db.Save(&resource).Error
}

func (ctx *MahresourcesContext) RotateResource(resourceId uint, degrees int) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return err
	}

	if !resource.IsImage() {
		return errors.New("not an image")
	}

	f, err := ctx.fs.Open(resource.GetCleanLocation())

	if err != nil {
		return err
	}

	img, _, err := image.Decode(f)

	if err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	rotatedImage := transform.Rotate(img, float64(degrees), &transform.RotationOptions{ResizeBounds: true})

	var buf bytes.Buffer
	if err := imgio.JPEGEncoder(100)(&buf, rotatedImage); err != nil {
		return err
	}

	newFile, err := ctx.fs.Create(resource.GetCleanLocation() + ".rotated")
	if err != nil {
		return err
	}

	if _, err := io.Copy(newFile, &buf); err != nil {
		return err
	}

	if err := ctx.fs.Remove(resource.GetCleanLocation()); err != nil {
		return err
	}

	if err := newFile.Close(); err != nil {
		return err
	}

	if err := ctx.fs.Rename(resource.GetCleanLocation()+".rotated", resource.GetCleanLocation()); err != nil {
		return err
	}

	// delete the thumbnail(s)
	if err := ctx.db.Where("resource_id = ?", resourceId).Delete(&models.Preview{}).Error; err != nil {
		return err
	}

	return ctx.RecalculateResourceDimensions(&query_models.EntityIdQuery{ID: resourceId})
}
