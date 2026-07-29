package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// The image transform endpoints (rotate, recalculateDimensions) used to run
// every resource whose ContentType merely started with "image/" straight into
// a Go image decoder. Vector and non-image payloads therefore surfaced as
// HTTP 500 with a raw decoder message ("image: unknown format"), and rotate
// re-encoded every input as JPEG regardless of its source format.
//
// Findings 10, 11 and 12 of the 2026-07-29 UI bug hunt.

// writeResourceFile stores content in the resource filesystem and returns a
// resource row pointing at it.
func writeResourceFile(t *testing.T, tc *TestContext, name, contentType string, content []byte, w, h uint) *models.Resource {
	t.Helper()

	hash := fmt.Sprintf("%040x", len(content)*7919+int(name[0]))
	location := "/resources/" + hash[0:2] + "/" + hash[2:4] + "/" + hash[4:6] + "/" + hash + path.Ext(name)

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	require.NoError(t, err)
	require.NoError(t, fs.MkdirAll(filepath.Dir(location), 0755))
	f, err := fs.Create(location)
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	owner := tc.CreateDummyGroup("transform-owner-" + name)
	resource := &models.Resource{
		Name:        name,
		Hash:        hash,
		HashType:    "SHA1",
		Location:    location,
		ContentType: contentType,
		FileSize:    int64(len(content)),
		Width:       w,
		Height:      h,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)
	return resource
}

// makeAlphaPNG builds a PNG with a genuinely transparent region so that a
// format-preserving transform can be told apart from a JPEG re-encode.
func makeAlphaPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0}) // fully transparent
			} else {
				img.Set(x, y, color.RGBA{R: 10, G: 200, B: 40, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

const testSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 300" width="600" height="300">
  <rect width="600" height="300" fill="#3355bb"/>
  <circle cx="300" cy="150" r="90" fill="#ffcc00"/>
</svg>`

// TestImageTransforms_RejectNonRasterFormats asserts that both transform
// endpoints refuse content they cannot decode with a 4xx that names the
// offending format, instead of a 500 carrying a decoder's internal message.
func TestImageTransforms_RejectNonRasterFormats(t *testing.T) {
	cases := []struct {
		name        string
		fileName    string
		contentType string
		content     []byte
	}{
		{"svg", "vector.svg", "image/svg+xml", []byte(testSVG)},
		{"plain text", "notes.txt", "text/plain; charset=utf-8", []byte("just some text\n")},
		{"json", "payload.json", "application/json", []byte(`{"a":1}`)},
		{"zero byte", "empty.bin", "text/plain", []byte{}},
		{"zero byte claiming png", "empty.png", "image/png", []byte{}},
	}

	endpoints := []struct {
		label string
		path  string
		form  func(id uint) url.Values
	}{
		{
			label: "rotate",
			path:  "/v1/resources/rotate",
			form: func(id uint) url.Values {
				return url.Values{"ID": {fmt.Sprint(id)}, "Degrees": {"90"}}
			},
		},
		{
			label: "recalculateDimensions",
			path:  "/v1/resource/recalculateDimensions",
			form: func(id uint) url.Values {
				return url.Values{"ID": {fmt.Sprint(id)}}
			},
		},
	}

	for _, ep := range endpoints {
		for _, tt := range cases {
			t.Run(ep.label+"/"+tt.name, func(t *testing.T) {
				tc := SetupTestEnv(t)
				require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

				resource := writeResourceFile(t, tc, tt.fileName, tt.contentType, tt.content, 0, 0)

				resp := tc.MakeFormRequest(http.MethodPost, ep.path, ep.form(resource.ID))

				assert.GreaterOrEqual(t, resp.Code, 400,
					"%s on %s should be rejected, got %d", ep.label, tt.contentType, resp.Code)
				assert.Less(t, resp.Code, 500,
					"%s on %s must not surface as a server error, got %d: %s",
					ep.label, tt.contentType, resp.Code, resp.Body.String())

				// The error must be JSON and must name the format the caller
				// supplied, so the message is actionable.
				var payload map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload),
					"error body should be JSON, got: %s", resp.Body.String())
				msg, _ := payload["error"].(string)
				assert.NotEmpty(t, msg, "error body should carry an 'error' field")

				baseType := strings.SplitN(tt.contentType, ";", 2)[0]
				assert.Contains(t, msg, baseType,
					"error message should name the rejected content type")
				// The raw decoder message may still be wrapped in as the
				// cause — that aids debugging — but it must not be the whole
				// message, which is how it originally reached users.
				assert.NotEqual(t, "image: unknown format", strings.TrimSpace(msg),
					"error message should explain the failure, not just relay the decoder")
				assert.NotEqual(t, "not an image", strings.TrimSpace(msg),
					"error message should name the format, not just say 'not an image'")
			})
		}
	}
}

// TestImageTransforms_AcceptRasterFormats is the positive control for the
// test above: the very same endpoints must still succeed on real raster
// images. Without this, a change that rejected everything would satisfy the
// negative assertions forever.
func TestImageTransforms_AcceptRasterFormats(t *testing.T) {
	t.Run("rotate", func(t *testing.T) {
		tc := SetupTestEnv(t)
		require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

		resource := writeResourceFile(t, tc, "photo.png", "image/png", makeAlphaPNG(t, 8, 8), 8, 8)

		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/rotate",
			url.Values{"ID": {fmt.Sprint(resource.ID)}, "Degrees": {"90"}})
		assert.Less(t, resp.Code, 400, "rotating a PNG should succeed, got %d: %s", resp.Code, resp.Body.String())
	})

	t.Run("recalculateDimensions", func(t *testing.T) {
		tc := SetupTestEnv(t)

		// Store deliberately wrong dimensions so a successful recalculation is
		// observable.
		resource := writeResourceFile(t, tc, "photo2.png", "image/png", makeAlphaPNG(t, 12, 5), 1, 1)

		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resource/recalculateDimensions",
			url.Values{"ID": {fmt.Sprint(resource.ID)}})
		assert.Less(t, resp.Code, 400, "recalculating a PNG should succeed, got %d: %s", resp.Code, resp.Body.String())

		var updated models.Resource
		require.NoError(t, tc.DB.First(&updated, resource.ID).Error)
		assert.Equal(t, uint(12), updated.Width, "width should be recalculated from the file")
		assert.Equal(t, uint(5), updated.Height, "height should be recalculated from the file")
	})
}

// TestRotateResource_PreservesPNGFormat covers finding 12: rotation used to
// encode its output with imgio.JPEGEncoder unconditionally, so a PNG came back
// as JPEG — inflating a 1.4 KB transparent PNG to 10 KB and flattening its
// alpha channel onto black.
func TestRotateResource_PreservesPNGFormat(t *testing.T) {
	tc := SetupTestEnv(t)
	require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

	original := makeAlphaPNG(t, 64, 64)
	// The name carries an extension, which is what getExtensionFromFilename
	// prefers — this is where the stored path and the content type diverged.
	resource := writeResourceFile(t, tc, "transparent.png", "image/png", original, 64, 64)

	require.NoError(t, tc.AppCtx.RotateResource(context.Background(), resource.ID, 90))

	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)

	assert.Equal(t, "image/png", updated.ContentType,
		"rotating a PNG must keep it a PNG")
	assert.Equal(t, ".png", strings.ToLower(path.Ext(updated.Location)),
		"stored file extension must match the content type")

	// Read the rotated bytes back and verify they really are a PNG with alpha.
	fs, err := tc.AppCtx.GetFsForStorageLocation(updated.StorageLocation)
	require.NoError(t, err)
	f, err := fs.Open(updated.GetCleanLocation())
	require.NoError(t, err)
	defer f.Close()

	img, format, err := image.Decode(f)
	require.NoError(t, err, "rotated output should decode")
	assert.Equal(t, "png", format, "rotated output should be encoded as PNG")

	// The left half of the source was fully transparent; after a 90° rotation
	// that band still exists somewhere in the image. A JPEG re-encode would
	// have made every pixel opaque.
	bounds := img.Bounds()
	sawTransparent := false
	for x := bounds.Min.X; x < bounds.Max.X && !sawTransparent; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				sawTransparent = true
				break
			}
		}
	}
	assert.True(t, sawTransparent, "alpha channel must survive rotation")

	// Format preservation should also keep the file from ballooning.
	assert.Less(t, updated.FileSize, int64(len(original))*4,
		"a rotated PNG should not inflate the way a JPEG re-encode did (was 7.3x)")
}

// TestRotateResource_PreservesJPEGFormat is the counterpart control: JPEG
// input must stay JPEG, so the fix is format-preserving rather than
// PNG-forcing.
func TestRotateResource_PreservesJPEGFormat(t *testing.T) {
	tc := SetupTestEnv(t)
	require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for x := 0; x < 32; x++ {
		for y := 0; y < 16; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: 60, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))

	resource := writeResourceFile(t, tc, "photo.jpg", "image/jpeg", buf.Bytes(), 32, 16)

	require.NoError(t, tc.AppCtx.RotateResource(context.Background(), resource.ID, 90))

	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)

	assert.Equal(t, "image/jpeg", updated.ContentType, "rotating a JPEG must keep it a JPEG")
	assert.Equal(t, ".jpg", strings.ToLower(path.Ext(updated.Location)),
		"stored file extension must match the content type")
}

// TestResourcePage_HidesRasterOnlyActions covers finding 86: the resource
// details page rendered "Rotate" and "Recalculate Dimensions" for every
// resource whose content type began with "image/", including SVG — offering
// two buttons whose only possible outcome was an HTTP 500.
func TestResourcePage_HidesRasterOnlyActions(t *testing.T) {
	// Markers unique to the two forms in the image-operations sidebar group.
	const rotateForm = `action="/v1/resources/rotate"`
	const recalculateForm = `/v1/resource/recalculateDimensions`

	t.Run("svg hides them", func(t *testing.T) {
		tc := SetupTestEnv(t)
		resource := writeResourceFile(t, tc, "vector.svg", "image/svg+xml", []byte(testSVG), 600, 300)

		resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/resource?id=%d", resource.ID), nil)
		require.Equal(t, http.StatusOK, resp.Code, "resource page should render")
		body := resp.Body.String()

		assert.NotContains(t, body, rotateForm,
			"an SVG must not be offered Rotate — the server cannot decode it")
		assert.NotContains(t, body, recalculateForm,
			"an SVG must not be offered Recalculate Dimensions")
	})

	// Positive control: without it, a template change that dropped the whole
	// sidebar group would satisfy the assertions above forever.
	t.Run("png still shows them", func(t *testing.T) {
		tc := SetupTestEnv(t)
		resource := writeResourceFile(t, tc, "photo.png", "image/png", makeAlphaPNG(t, 20, 20), 20, 20)

		resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/resource?id=%d", resource.ID), nil)
		require.Equal(t, http.StatusOK, resp.Code, "resource page should render")
		body := resp.Body.String()

		assert.Contains(t, body, rotateForm, "a PNG should still offer Rotate")
		assert.Contains(t, body, recalculateForm, "a PNG should still offer Recalculate Dimensions")
	})
}
