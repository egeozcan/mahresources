package application_context

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"mahresources/constants"
	"mahresources/models"
	"math"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/srwiley/oksvg"
)

// This file holds the pieces the image transform paths (rotate, crop) and the
// thumbnail pipeline share. They used to be duplicated or missing entirely:
// CropResource carried a per-format encoder switch while RotateResource
// hardcoded JPEG, so rotating a transparent PNG silently produced a 7x larger
// JPEG with its alpha flattened (finding 12).

// encodedImage is the result of re-encoding a transformed image: the bytes to
// store, the file extension that matches them, and a note for the version
// comment when the format had to change.
type encodedImage struct {
	Data []byte
	Ext  string
	Note string
}

// encodeInSourceFormat re-encodes img in the format it was decoded from,
// falling back to PNG for formats we can decode but not write. PNG is the
// fallback rather than JPEG because it is lossless and keeps transparency.
//
// format is the string returned by image.Decode, or decodeFallbackFormat for
// images that needed the external decoder (HEIC, AVIF, …).
func encodeInSourceFormat(img image.Image, format string) (encodedImage, error) {
	switch format {
	case "jpeg":
		var buf bytes.Buffer
		if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(95)); err != nil {
			return encodedImage{}, fmt.Errorf("failed to encode JPEG: %w", err)
		}
		return encodedImage{Data: buf.Bytes(), Ext: ".jpg"}, nil

	case "png":
		var buf bytes.Buffer
		if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
			return encodedImage{}, fmt.Errorf("failed to encode PNG: %w", err)
		}
		return encodedImage{Data: buf.Bytes(), Ext: ".png"}, nil

	case "gif":
		return encodeAsPNG(img, " (animation dropped, re-encoded as PNG)")
	case "webp", "bmp", "tiff":
		return encodeAsPNG(img, " (re-encoded as PNG)")
	case decodeFallbackFormat:
		return encodeAsPNG(img, " (decoded via ImageMagick, re-encoded as PNG)")
	}

	return encodedImage{}, fmt.Errorf("image format %q cannot be re-encoded", format)
}

// decodeFallbackFormat marks images that only the external decoder could read.
const decodeFallbackFormat = "fallback"

func encodeAsPNG(img image.Image, note string) (encodedImage, error) {
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
		return encodedImage{}, fmt.Errorf("failed to encode PNG: %w", err)
	}
	return encodedImage{Data: buf.Bytes(), Ext: ".png", Note: note}, nil
}

// errNotRasterImage builds the rejection returned when a transform is asked to
// operate on something it cannot decode. The message names the content type so
// the caller learns what was wrong, and its wording is what
// statusCodeForError maps to 415 Unsupported Media Type.
func errNotRasterImage(operation, contentType string) error {
	return fmt.Errorf(
		"%s needs a raster image, but content type %q is not a raster image format (supported: %s)",
		operation, contentType, strings.Join(models.RasterImageContentTypes, ", "),
	)
}

// errUndecodableImage builds the rejection for a resource that claims a raster
// content type but whose bytes will not decode — a truncated upload, or an
// empty file. Also a 4xx: the request is well-formed, the stored payload is
// not usable.
func errUndecodableImage(operation, contentType string, cause error) error {
	return fmt.Errorf(
		"%s failed: the file stored for this resource (content type %q) could not be decoded as an image: %w",
		operation, contentType, cause,
	)
}

// resizeForThumbnail resizes src to the requested box, deriving any axis the
// caller left at zero from the source image instead of handing the zero
// straight to imaging.Resize.
//
// imaging.Resize(img, 0, 0) returns a 0x0 image. That single behaviour is what
// poisoned the preview cache: a dimensionless request for a resource with
// unknown stored dimensions produced a 0x0 preview, which was then persisted
// with Width=0/Height=0 — the sentinel meaning "canonical custom thumbnail" —
// so every later request at any size resized from the degenerate row and
// returned 0x0 forever (findings 69, 72, 73).
func resizeForThumbnail(src image.Image, width, height uint) image.Image {
	if width > 0 || height > 0 {
		return imaging.Resize(src, int(width), int(height), imaging.Lanczos)
	}

	// Neither axis was requested and the resource's aspect was unknown. Fall
	// back to the source image's own dimensions, scaled down to fit the
	// thumbnail caps — the same box computeActualTargetDims would have
	// produced had the dimensions been known.
	bounds := src.Bounds()
	srcW, srcH := float64(bounds.Dx()), float64(bounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return src
	}

	scale := math.Min(1.0, math.Min(constants.MaxThumbWidth/srcW, constants.MaxThumbHeight/srcH))
	return imaging.Resize(src, int(math.Round(srcW*scale)), int(math.Round(srcH*scale)), imaging.Lanczos)
}

// hasPixels reports whether an encoded image decodes to a non-empty raster.
// Used as the invariant guarding every Preview row we persist.
func hasPixels(data []byte) bool {
	w, h, err := decodeImageDimensions(data)
	return err == nil && w > 0 && h > 0
}

// svgIntrinsicDimensions reads an SVG's natural size from its viewBox (or its
// width/height attributes, which preprocessSVG normalises into one).
//
// The upload path computes dimensions with a Go image decoder, which cannot
// read SVG, so every SVG landed in the database with Width=0/Height=0. That
// unknown aspect is the trigger condition for the preview cache poisoning, so
// recording real dimensions removes the whole class for new uploads.
func svgIntrinsicDimensions(data []byte) (uint, uint, bool) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(preprocessSVG(data)))
	if err != nil {
		return 0, 0, false
	}

	w, h := icon.ViewBox.W, icon.ViewBox.H
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}

	// Guard against absurd viewBoxes overflowing the uint conversion.
	const maxSVGDimension = 1 << 20
	if w > maxSVGDimension || h > maxSVGDimension {
		return 0, 0, false
	}

	return uint(math.Round(w)), uint(math.Round(h)), true
}

// svgIntrinsicDimensionsFromReader is the streaming form used by the upload
// path, which holds an io.Reader rather than the whole payload.
func svgIntrinsicDimensionsFromReader(r io.Reader) (uint, uint, bool) {
	// SVGs are text; cap the read so a hostile upload cannot exhaust memory
	// here. Anything larger than this has a viewBox in its first bytes anyway.
	const maxSVGRead = 8 << 20
	data, err := io.ReadAll(io.LimitReader(r, maxSVGRead))
	if err != nil {
		return 0, 0, false
	}
	return svgIntrinsicDimensions(data)
}
