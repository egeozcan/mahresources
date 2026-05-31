package plugin_system

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"

	// Register additional image decoders for image.Decode (jpeg is imported
	// normally below so we can also encode it).
	_ "image/gif"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// parseAspectRatio parses a "W:H" string (e.g. "16:9") into a float ratio W/H.
func parseAspectRatio(s string) (float64, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid ratio format: expected 'W:H' like '16:9'")
	}
	w, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	h, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, fmt.Errorf("invalid ratio values: both must be positive numbers")
	}
	return w / h, nil
}

// padToAspectRatio pads srcImg with white borders so its dimensions exactly
// match the target aspect ratio (ratio = W/H), centering the original content
// without stretching or cropping. If the source already matches the ratio
// within tolerance, srcImg is returned unchanged. Returns the (possibly new)
// image and its dimensions.
func padToAspectRatio(srcImg image.Image, ratio float64) (image.Image, int, int) {
	srcBounds := srcImg.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	srcRatio := float64(srcW) / float64(srcH)

	// If the source already matches the target ratio within tolerance, no
	// padding is needed.
	const epsilon = 0.001
	if math.Abs(srcRatio-ratio) < epsilon {
		return srcImg, srcW, srcH
	}

	// Calculate padded dimensions.
	// If source is wider than target ratio -> letterbox (pad top/bottom).
	// If source is narrower than target ratio -> pillarbox (pad left/right).
	var paddedW, paddedH int
	if srcRatio > ratio {
		// Source is too wide -- pad top and bottom.
		paddedW = srcW
		paddedH = int(math.Round(float64(srcW) / ratio))
	} else {
		// Source is too tall -- pad left and right.
		paddedH = srcH
		paddedW = int(math.Round(float64(srcH) * ratio))
	}

	// Create a new white canvas at the padded dimensions.
	paddedImg := image.NewRGBA(image.Rect(0, 0, paddedW, paddedH))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(paddedImg, paddedImg.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Center the source image on the white canvas. Offsets are always >= 0
	// because the padded dimension is never smaller than the source dimension.
	offsetX := (paddedW - srcW) / 2
	offsetY := (paddedH - srcH) / 2
	dstRect := image.Rect(offsetX, offsetY, offsetX+srcW, offsetY+srcH)
	draw.Draw(paddedImg, dstRect, srcImg, srcBounds.Min, draw.Over)

	return paddedImg, paddedW, paddedH
}

// encodeImage encodes img back to bytes, preserving JPEG for photographic
// sources (much smaller payloads than lossless PNG) and using PNG for
// everything else. Returns the encoded bytes and the matching MIME type.
func encodeImage(img image.Image, srcFormat string) ([]byte, string, error) {
	var buf bytes.Buffer
	if srcFormat == "jpeg" {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", fmt.Errorf("JPEG encode failed: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	}
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", fmt.Errorf("PNG encode failed: %w", err)
	}
	return buf.Bytes(), "image/png", nil
}

// padDataURIToAspectRatio decodes a "data:<mime>;base64,..." image, pads it
// with white borders to the target "W:H" aspect ratio, and returns a new data
// URI plus the padded dimensions. Photographic (JPEG) inputs are re-encoded as
// JPEG to avoid ballooning the payload; everything else is encoded as PNG.
func padDataURIToAspectRatio(dataURI, targetRatio string) (string, int, int, error) {
	ratio, err := parseAspectRatio(targetRatio)
	if err != nil {
		return "", 0, 0, err
	}

	// Strip the data URI prefix to get raw base64.
	commaIdx := strings.Index(dataURI, ",")
	if commaIdx < 0 {
		return "", 0, 0, fmt.Errorf("invalid data URI: missing comma separator")
	}
	rawBytes, err := base64.StdEncoding.DecodeString(dataURI[commaIdx+1:])
	if err != nil {
		return "", 0, 0, fmt.Errorf("base64 decode failed: %w", err)
	}

	srcImg, format, err := image.Decode(bytes.NewReader(rawBytes))
	if err != nil {
		return "", 0, 0, fmt.Errorf("image decode failed: %w", err)
	}
	if srcImg.Bounds().Dx() <= 0 || srcImg.Bounds().Dy() <= 0 {
		return "", 0, 0, fmt.Errorf("source image has invalid dimensions")
	}

	paddedImg, w, h := padToAspectRatio(srcImg, ratio)

	encoded, mime, err := encodeImage(paddedImg, format)
	if err != nil {
		return "", 0, 0, err
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(encoded), w, h, nil
}

// registerImageModule registers the mah.image sub-table with image processing
// utilities for Lua plugins.
func (pm *PluginManager) registerImageModule(L *lua.LState, mahMod *lua.LTable) {
	imgMod := L.NewTable()

	// mah.image.pad_to_aspect_ratio(data_uri, target_ratio) -> padded_data_uri, new_width, new_height
	// Pads the image with white borders to exactly match the target aspect ratio
	// without stretching or cropping the original content.
	//   data_uri:     "data:image/png;base64,..."
	//   target_ratio: "16:9", "1:1", "4:3", etc.
	// Returns padded_data_uri, new_width, new_height.
	// On error, returns nil, error_string (the caller must check the first
	// return value before using it).
	imgMod.RawSetString("pad_to_aspect_ratio", L.NewFunction(func(L *lua.LState) int {
		dataURI := L.CheckString(1)
		targetRatio := L.CheckString(2)

		paddedURI, w, h, err := padDataURIToAspectRatio(dataURI, targetRatio)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LString(paddedURI))
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 3
	}))

	mahMod.RawSetString("image", imgMod)
}
