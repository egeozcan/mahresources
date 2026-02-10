package application_context

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"
	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
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
	} else if resource.ContentType == "image/svg+xml" {
		// Handle SVG resources with dedicated SVG renderer
		fileBytes, err = ctx.generateSVGThumbnailFromFile(fs, resource.GetCleanLocation(), width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating SVG thumbnail from file: %w", err)
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
	} else if isOfficeDocument(resource.ContentType) {
		// Handle office documents (docx, xlsx, pptx, etc.) using LibreOffice
		fileBytes, err = ctx.generateOfficeDocumentThumbnail(resource, fs, width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating office document thumbnail: %w", err)
		}
		if fileBytes == nil {
			// LibreOffice not available, skip thumbnail generation
			return nil, nil
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

// truncateStderr limits stderr output to a reasonable length for error messages.
func truncateStderr(stderr string, maxLen int) string {
	if len(stderr) <= maxLen {
		return stderr
	}
	return stderr[:maxLen] + "... (truncated)"
}

// parseFFmpegError analyzes ffmpeg stderr to determine if the error indicates
// that the video format requires seeking (and thus needs temp file fallback).
// Returns true if temp file fallback should be attempted, along with an error category.
func parseFFmpegError(stderr string) (needsTempFile bool, errorCategory string) {
	stderrLower := strings.ToLower(stderr)

	// Patterns indicating the format requires seeking (temp file needed)
	seekPatterns := []struct {
		pattern  string
		category string
	}{
		{"moov atom not found", "moov atom not found"},
		{"invalid data found when processing input", "invalid input data"},
		{"could not find codec parameters", "codec parameters not found"},
		{"error while opening encoder", "encoder error"},
		{"pipe:: end of file", "pipe EOF"},
		{"pipe:: invalid data", "pipe invalid data"},
		{"immediate exit requested", "immediate exit"},
		{"invalid argument", "invalid argument"},
	}

	for _, p := range seekPatterns {
		if strings.Contains(stderrLower, p.pattern) {
			return true, p.category
		}
	}

	return false, ""
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
		fmt.Println("Warning: ImageMagick not found. Install ImageMagick to enable HEIC/AVIF thumbnail support.")
		return nil, errors.New("ImageMagick not available (install ImageMagick for HEIC/AVIF support)")
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
		return nil, fmt.Errorf("ImageMagick conversion failed: %w (stderr: %s)", err, truncateStderr(stderr.String(), 200))
	}

	// Decode the PNG output
	img, _, err := image.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ImageMagick output: %w", err)
	}

	return img, nil
}

// preprocessSVG cleans up SVG content to work around oksvg limitations.
// Specifically, it removes percentage-based width/height attributes which
// cause oksvg to fail reading the viewBox.
func preprocessSVG(data []byte) []byte {
	content := string(data)

	// Remove width="100%" or width='100%' (and similar percentages)
	// This allows oksvg to fall back to viewBox dimensions
	for _, attr := range []string{"width", "height"} {
		for _, quote := range []string{`"`, `'`} {
			// Match patterns like width="100%" or height="50%"
			start := 0
			for {
				attrStart := strings.Index(content[start:], attr+"="+quote)
				if attrStart == -1 {
					break
				}
				attrStart += start
				valueStart := attrStart + len(attr) + 2 // skip attr="
				valueEnd := strings.Index(content[valueStart:], quote)
				if valueEnd == -1 {
					break
				}
				valueEnd += valueStart
				value := content[valueStart:valueEnd]

				// If value contains %, remove the entire attribute
				if strings.Contains(value, "%") {
					// Remove from attr= to closing quote (inclusive)
					content = content[:attrStart] + content[valueEnd+1:]
					// Don't advance start since we removed content
				} else {
					start = valueEnd + 1
				}
			}
		}
	}

	return []byte(content)
}

// decodeSVG renders an SVG file to a raster image using the oksvg/rasterx libraries.
// It reads the SVG, determines its dimensions, and rasterizes it to an RGBA image.
func (ctx *MahresourcesContext) decodeSVG(file io.Reader) (image.Image, error) {
	// Read all SVG data for preprocessing
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read SVG data: %w", err)
	}

	// Preprocess to fix oksvg limitations with percentage dimensions
	data = preprocessSVG(data)

	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SVG: %w", err)
	}

	// Get the SVG's viewbox dimensions
	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)

	// Track if we have a valid viewbox for scaling
	hasViewBox := w > 0 && h > 0

	// Use default dimensions if viewbox is missing
	if !hasViewBox {
		w = 800
		h = 600
	}

	// Cap dimensions to prevent excessive memory usage
	maxDim := 2000
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / math.Max(float64(w), float64(h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}

	// Only set target if we have a valid viewbox to scale from.
	// When viewbox is 0x0, SetTarget breaks scaling math - let content render at natural coords.
	if hasViewBox {
		icon.SetTarget(0, 0, float64(w), float64(h))
	}

	// Create an RGBA image to draw onto
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))

	// Fill with white background (SVGs often have transparent backgrounds)
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)

	// Create a scanner/rasterizer and draw the SVG
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return rgba, nil
}

// generateSVGThumbnailFromFile generates a thumbnail from an SVG file.
func (ctx *MahresourcesContext) generateSVGThumbnailFromFile(
	fs afero.Fs,
	location string,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	file, err := fs.Open(location)
	if err != nil {
		return nil, fmt.Errorf("failed to open SVG file: %w", err)
	}
	defer file.Close()

	// Decode SVG to raster image
	originalImage, err := ctx.decodeSVG(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SVG file: %w", err)
	}

	// Resize using imaging library with Lanczos filter
	newImage := imaging.Resize(originalImage, int(width), int(height), imaging.Lanczos)

	// Encode as JPEG with adaptive quality
	quality := getJPEGQuality(width, height)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, newImage, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("failed to encode resized SVG image: %w", err)
	}

	return buf.Bytes(), nil
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

// resolveLocalFilePath attempts to resolve the real OS path for a file in an afero filesystem.
// Returns the absolute path and true if the filesystem is a BasePathFs wrapping a real OS filesystem.
// Returns ("", false) for non-local filesystems (MemMapFs, CopyOnWriteFs, etc.).
func resolveLocalFilePath(fs afero.Fs, name string) (string, bool) {
	bpFs, ok := fs.(*afero.BasePathFs)
	if !ok {
		return "", false
	}

	realPath, err := bpFs.RealPath(name)
	if err != nil {
		return "", false
	}

	if _, err := os.Stat(realPath); err != nil {
		return "", false
	}

	return realPath, true
}

// generateVideoThumbnail generates a thumbnail from the video resource.
// It produces a "null thumbnail" (full-width JPEG) and stores it in the DB,
// so subsequent requests for any size can resize from the cached version without ffmpeg.
func (ctx *MahresourcesContext) generateVideoThumbnail(
	resource models.Resource,
	fs afero.Fs,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	// Determine runTimeout based on config and context's deadline
	runTimeout := ctx.Config.VideoThumbnailTimeout

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
		resource.ID,
		ctx.Config.VideoThumbnailLockTimeout,
		runTimeout,
		func() error {
			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Try to extract a frame from the video
			resultBuffer := bytes.NewBuffer([]byte{})
			var extractErr error

			// Priority 1: Try direct file path with fast seeking (local filesystems only)
			if localPath, ok := resolveLocalFilePath(fs, resource.GetCleanLocation()); ok {
				extractErr = ctx.createThumbFromVideoFileAtTime(httpContext, localPath, resultBuffer, 1)
				if extractErr != nil || resultBuffer.Len() == 0 {
					resultBuffer.Reset()
					extractErr = ctx.createThumbFromVideoFileAtTime(httpContext, localPath, resultBuffer, 0)
				}
			}

			// Priority 2: Fall back to stdin-based approach (non-local filesystems or if local failed)
			if resultBuffer.Len() == 0 {
				resultBuffer.Reset()
				file, err := fs.Open(resource.GetCleanLocation())
				if err != nil {
					return fmt.Errorf("failed to open video file: %w", err)
				}
				defer file.Close()

				extractErr = ctx.createThumbFromVideo(httpContext, file, resultBuffer, &resource)
				if extractErr != nil {
					return fmt.Errorf("failed to create thumbnail from video: %w", extractErr)
				}
			}

			if resultBuffer.Len() == 0 {
				return fmt.Errorf("ffmpeg produced no output for video: %w", extractErr)
			}

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// The ffmpeg output is now JPEG. Store it as a null thumbnail (width=0, height=0)
			// so subsequent requests for any size can resize from this cached version.
			nullThumbData := resultBuffer.Bytes()
			nullPreview := &models.Preview{
				Data:        nullThumbData,
				Width:       0,
				Height:      0,
				ContentType: "image/jpeg",
				ResourceId:  &resource.ID,
			}

			if err := ctx.db.WithContext(httpContext).Save(nullPreview).Error; err != nil {
				log.Printf("Warning: failed to save null thumbnail for resource %d: %v", resource.ID, err)
				// Continue anyway - we can still resize for this request
			}

			// Resize the null thumbnail to the requested dimensions
			resized, err := ctx.generateImageThumbnail(nullThumbData, width, height)
			if err != nil {
				return fmt.Errorf("failed to resize video thumbnail: %w", err)
			}

			fileBytes = resized
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
// It attempts to create a thumbnail at 1 second using stdin piping, and if that fails
// or returns no data, it retries at 0 seconds. If stdin piping fails due to format
// limitations (e.g., MOV with moov atom at end), it falls back to using a temp file.
func (ctx *MahresourcesContext) createThumbFromVideo(
	httpContext context.Context,
	file io.ReadSeeker,
	resultBuffer *bytes.Buffer,
	resource *models.Resource,
) error {
	// First attempt: stdin-based extraction at 1 second
	var stdinErr error
	var stderrContent string

	stdinErr = ctx.createThumbFromVideoAtGivenTime(httpContext, file, resultBuffer, 1)

	// Check if we got a result
	if stdinErr == nil && resultBuffer.Len() > 0 {
		return nil
	}

	// Capture error details for analysis
	if stdinErr != nil {
		stderrContent = stdinErr.Error()
	}

	// Check if the error indicates we need temp file fallback
	needsTempFile, errorCategory := parseFFmpegError(stderrContent)

	// If not a seek-related error, try at 0 seconds with stdin
	if !needsTempFile {
		resultBuffer.Reset()
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return fmt.Errorf("failed to seek video file: %w", seekErr)
		}

		stdinErr = ctx.createThumbFromVideoAtGivenTime(httpContext, file, resultBuffer, 0)
		if stdinErr == nil && resultBuffer.Len() > 0 {
			return nil
		}

		// Re-check error after second attempt
		if stdinErr != nil {
			stderrContent = stdinErr.Error()
			needsTempFile, errorCategory = parseFFmpegError(stderrContent)
		}
	}

	// Fallback: use temp file for formats that require seeking
	if needsTempFile || resultBuffer.Len() == 0 {
		if errorCategory != "" {
			log.Printf("Video thumbnail stdin failed (reason: %s), trying temp file fallback for resource %d", errorCategory, resource.ID)
		} else {
			log.Printf("Video thumbnail stdin produced no output, trying temp file fallback for resource %d", resource.ID)
		}

		resultBuffer.Reset()
		return ctx.createThumbFromVideoWithTempFile(httpContext, file, resultBuffer, resource)
	}

	return fmt.Errorf("failed to create thumbnail: %w", stdinErr)
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
		"-c:v", "mjpeg", // encode to JPEG (faster than PNG)
		"-q:v", "3", // JPEG quality (lower = better, 2-5 is good)
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

// createThumbFromVideoWithTempFile copies the video to a temp file and generates
// a thumbnail using file-based ffmpeg input. This handles video formats that require
// seeking (like MOV with moov atom at end) which don't work with stdin piping.
func (ctx *MahresourcesContext) createThumbFromVideoWithTempFile(
	httpContext context.Context,
	file io.ReadSeeker,
	resultBuffer *bytes.Buffer,
	resource *models.Resource,
) error {
	// Determine file extension for ffmpeg format detection
	ext := filepath.Ext(resource.GetCleanLocation())
	if ext == "" {
		ext = ".mp4" // Default extension
	}

	// Create temp file with appropriate extension
	tempFile, err := os.CreateTemp("", "video-thumb-*"+ext)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Reset source file position
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to seek source file: %w", err)
	}

	// Check context before copying
	select {
	case <-httpContext.Done():
		tempFile.Close()
		return httpContext.Err()
	default:
	}

	// Copy video data from Afero file to temp file
	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to copy to temp file: %w", err)
	}

	// Close temp file before ffmpeg reads it
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Try at 1 second first
	err = ctx.createThumbFromVideoFileAtTime(httpContext, tempPath, resultBuffer, 1)
	if err == nil && resultBuffer.Len() > 0 {
		return nil
	}

	// If first attempt failed, try at 0 seconds
	resultBuffer.Reset()
	err = ctx.createThumbFromVideoFileAtTime(httpContext, tempPath, resultBuffer, 0)
	if err != nil {
		return fmt.Errorf("failed to create thumbnail from temp file: %w", err)
	}

	return nil
}

// createThumbFromVideoFileAtTime generates a thumbnail from a video file path at a specific time.
// Uses -ss before -i for fast seeking (only works reliably with file input, not stdin).
func (ctx *MahresourcesContext) createThumbFromVideoFileAtTime(
	httpContext context.Context,
	filePath string,
	resultBuffer *bytes.Buffer,
	secondsIn int,
) error {
	// Construct ffmpeg command with -ss BEFORE -i for fast seeking
	cmd := exec.CommandContext(httpContext, ctx.Config.FfmpegPath,
		"-ss", fmt.Sprintf("%d", secondsIn), // Seek BEFORE input (fast input seeking)
		"-i", filePath,                      // File input (enables seeking)
		"-vframes", "1",                     // Grab one frame
		"-vf", "scale=640:-1",               // Scale the image
		"-c:v", "mjpeg",                     // Encode to JPEG (faster than PNG)
		"-q:v", "3",                         // JPEG quality (lower = better, 2-5 is good)
		"-f", "image2pipe",                  // Output format
		"pipe:1",                            // Output to stdout
	)

	cmd.Stdout = resultBuffer

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if httpContext.Err() != nil {
			return httpContext.Err()
		}
		return fmt.Errorf("ffmpeg error: %w, stderr: %s", err, truncateStderr(stderr.String(), 500))
	}

	return nil
}

// isOfficeDocument checks if the content type is a supported office document format.
func isOfficeDocument(contentType string) bool {
	officeTypes := []string{
		// Microsoft Office (OpenXML)
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",       // docx
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",             // xlsx
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",     // pptx
		// OpenDocument
		"application/vnd.oasis.opendocument.text",         // odt
		"application/vnd.oasis.opendocument.spreadsheet",  // ods
		"application/vnd.oasis.opendocument.presentation", // odp
		// Legacy Microsoft Office
		"application/msword",                                                      // doc
		"application/vnd.ms-excel",                                                // xls
		"application/vnd.ms-powerpoint",                                           // ppt
	}

	for _, t := range officeTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// findLibreOfficePath returns the path to the LibreOffice executable.
// It first checks the configured path, then looks for 'soffice' or 'libreoffice' in PATH.
func (ctx *MahresourcesContext) findLibreOfficePath() string {
	// Use configured path if provided
	if ctx.Config.LibreOfficePath != "" {
		return ctx.Config.LibreOfficePath
	}

	// Try to find in PATH
	if path, err := exec.LookPath("soffice"); err == nil {
		return path
	}
	if path, err := exec.LookPath("libreoffice"); err == nil {
		return path
	}

	return ""
}

// generateOfficeDocumentThumbnail generates a thumbnail from an office document using LibreOffice.
// Returns nil, nil if LibreOffice is not available.
func (ctx *MahresourcesContext) generateOfficeDocumentThumbnail(
	resource models.Resource,
	fs afero.Fs,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	libreOfficePath := ctx.findLibreOfficePath()
	if libreOfficePath == "" {
		// LibreOffice not available, skip silently
		return nil, nil
	}

	// Determine runTimeout based on context's deadline
	runTimeout := 30 * time.Second // default timeout for office documents

	if deadline, ok := httpContext.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < runTimeout {
			runTimeout = remaining
		}
	}

	var fileBytes []byte

	// Execute thumbnail generation with lock and timeout
	lockAcquired, err := ctx.locks.OfficeDocumentGenerationLock.RunWithLockTimeout(
		resource.ID,
		30*time.Second, // lockTimeout
		runTimeout,     // runTimeout
		func() error {
			// Open the source file from the filesystem
			file, openErr := fs.Open(resource.GetCleanLocation())
			if openErr != nil {
				return fmt.Errorf("failed to open office document: %w", openErr)
			}
			defer file.Close()

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Create temp directory for input and output
			tempDir, err := os.MkdirTemp("", "office-thumb-*")
			if err != nil {
				return fmt.Errorf("failed to create temp directory: %w", err)
			}
			defer os.RemoveAll(tempDir)

			// Determine file extension from original filename or content type
			ext := filepath.Ext(resource.GetCleanLocation())
			if ext == "" {
				ext = getOfficeExtension(resource.ContentType)
			}

			// Copy file to temp location (LibreOffice needs file path)
			tempFile := filepath.Join(tempDir, "input"+ext)
			dst, err := os.Create(tempFile)
			if err != nil {
				return fmt.Errorf("failed to create temp file: %w", err)
			}

			if _, err := io.Copy(dst, file); err != nil {
				dst.Close()
				return fmt.Errorf("failed to copy to temp file: %w", err)
			}
			dst.Close()

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Run LibreOffice to convert to PNG
			outputDir := filepath.Join(tempDir, "output")
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			cmd := exec.CommandContext(httpContext, libreOfficePath,
				"--headless",
				"--convert-to", "png",
				"--outdir", outputDir,
				tempFile,
			)

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				if httpContext.Err() != nil {
					return httpContext.Err()
				}
				return fmt.Errorf("LibreOffice conversion failed: %w (stderr: %s)", err, truncateStderr(stderr.String(), 200))
			}

			// Find the generated PNG file
			pngFiles, err := filepath.Glob(filepath.Join(outputDir, "*.png"))
			if err != nil || len(pngFiles) == 0 {
				return errors.New("LibreOffice did not generate a PNG file")
			}

			// Read the generated PNG
			pngData, err := os.ReadFile(pngFiles[0])
			if err != nil {
				return fmt.Errorf("failed to read generated PNG: %w", err)
			}

			// Decode and resize to requested dimensions
			img, _, err := image.Decode(bytes.NewReader(pngData))
			if err != nil {
				return fmt.Errorf("failed to decode generated PNG: %w", err)
			}

			newImage := imaging.Resize(img, int(width), int(height), imaging.Lanczos)

			quality := getJPEGQuality(width, height)
			var buf bytes.Buffer
			if err := imaging.Encode(&buf, newImage, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
				return fmt.Errorf("failed to encode resized image: %w", err)
			}

			fileBytes = buf.Bytes()
			return nil
		},
	)

	if !lockAcquired {
		return nil, errors.New("failed to acquire office document generation lock")
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("office document thumbnail generation timed out")
		}
		return nil, fmt.Errorf("office document thumbnail generation error: %w", err)
	}

	return fileBytes, nil
}

// getOfficeExtension returns the file extension for a given office document content type.
func getOfficeExtension(contentType string) string {
	extensions := map[string]string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
		"application/vnd.oasis.opendocument.text":                                   ".odt",
		"application/vnd.oasis.opendocument.spreadsheet":                            ".ods",
		"application/vnd.oasis.opendocument.presentation":                           ".odp",
		"application/msword":                                                        ".doc",
		"application/vnd.ms-excel":                                                  ".xls",
		"application/vnd.ms-powerpoint":                                             ".ppt",
	}

	if ext, ok := extensions[contentType]; ok {
		return ext
	}
	return ".tmp"
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
