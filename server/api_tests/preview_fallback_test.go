package api_tests

import (
	"bytes"
	"fmt"
	"image"
	"net/http"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// GET /v1/resource/preview must never answer 200 with a degenerate image.
// Before the fix, computeActualTargetDims returned (0, 0) for any resource
// whose stored dimensions were unknown and whose caller passed neither width
// nor height. imaging.Resize(img, 0, 0) then produced a 0x0 image, which was
// persisted as a models.Preview with Width=0/Height=0 — the very sentinel the
// loader uses to mean "canonical custom thumbnail". Every later request at any
// size resized from that degenerate row, so one ordinary dimensionless request
// permanently broke every preview for that resource.
//
// Findings 69, 72 and 73 of the 2026-07-29 UI bug hunt.

// previewProbe issues a preview request and reports the status, body size and
// decoded dimensions.
type previewProbe struct {
	code   int
	body   []byte
	width  int
	height int
	// decoded reports whether the body parsed as an image at all.
	decoded bool
}

func probePreview(t *testing.T, tc *TestContext, query string) previewProbe {
	t.Helper()

	resp := tc.MakeRequest(http.MethodGet, "/v1/resource/preview?"+query, nil)
	p := previewProbe{code: resp.Code, body: resp.Body.Bytes()}

	if resp.Code == http.StatusOK {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(p.body))
		if err == nil {
			p.decoded = true
			p.width, p.height = cfg.Width, cfg.Height
		}
	}
	return p
}

// assertUsablePreview encodes the invariant: a preview response is either a
// redirect to the placeholder, or a real image with both dimensions > 0.
func assertUsablePreview(t *testing.T, p previewProbe, label string) {
	t.Helper()

	if p.code == http.StatusTemporaryRedirect || p.code == http.StatusFound {
		return // fell back to the placeholder, which is a valid answer
	}

	require.Equal(t, http.StatusOK, p.code, "%s: unexpected status", label)
	require.True(t, p.decoded, "%s: 200 response body did not decode as an image (%d bytes)", label, len(p.body))
	assert.Greater(t, p.width, 0, "%s: served a %dx%d image", label, p.width, p.height)
	assert.Greater(t, p.height, 0, "%s: served a %dx%d image", label, p.width, p.height)
}

// TestPreview_NeverServesDegenerateImage walks every non-rasterisable payload
// the seeder produces and asserts the invariant above.
func TestPreview_NeverServesDegenerateImage(t *testing.T) {
	cases := []struct {
		name        string
		fileName    string
		contentType string
		content     []byte
	}{
		{"svg", "vector.svg", "image/svg+xml", []byte(testSVG)},
		{"plain text", "notes.txt", "text/plain; charset=utf-8", []byte("hello\n")},
		{"csv", "table.csv", "text/csv", []byte("a,b\n1,2\n")},
		{"json", "payload.json", "application/json", []byte(`{"a":1}`)},
		{"zero byte", "empty.bin", "text/plain", []byte{}},
	}

	// Requests with no dimensions at all are the ones that used to poison the
	// cache; sized requests are included so the sweep covers both.
	queries := []string{"", "&width=200", "&height=300", "&width=200&height=150"}

	for _, tt := range cases {
		for _, q := range queries {
			label := tt.name + " [" + q + "]"
			t.Run(label, func(t *testing.T) {
				tc := SetupTestEnv(t)
				resource := writeResourceFile(t, tc, tt.fileName, tt.contentType, tt.content, 0, 0)

				p := probePreview(t, tc, fmt.Sprintf("id=%d%s", resource.ID, q))
				assertUsablePreview(t, p, label)
			})
		}
	}
}

// TestPreview_DimensionlessRequestDoesNotPoisonCache is the sequence test.
// The defect only appears across requests: a correct sized preview, then one
// dimensionless request, then the same sized preview again — which used to
// come back 0x0 and stay that way for every size, forever.
func TestPreview_DimensionlessRequestDoesNotPoisonCache(t *testing.T) {
	tc := SetupTestEnv(t)

	// An SVG has unknown stored dimensions, which is the trigger condition.
	resource := writeResourceFile(t, tc, "vector.svg", "image/svg+xml", []byte(testSVG), 0, 0)
	id := resource.ID

	// 1. A sized request works and establishes the baseline.
	first := probePreview(t, tc, fmt.Sprintf("id=%d&height=300", id))
	assertUsablePreview(t, first, "1. height=300 (baseline)")
	require.Equal(t, http.StatusOK, first.code, "baseline sized preview should render")
	require.True(t, first.decoded, "baseline preview should decode")
	baselineW, baselineH := first.width, first.height

	// 2. The dimensionless request — the poisoning trigger.
	second := probePreview(t, tc, fmt.Sprintf("id=%d", id))
	assertUsablePreview(t, second, "2. no dimensions")

	// 3 & 4. Every subsequent request, at the original size and a new one,
	// must still be usable.
	third := probePreview(t, tc, fmt.Sprintf("id=%d&height=300", id))
	assertUsablePreview(t, third, "3. height=300 after dimensionless request")
	assert.Equal(t, baselineW, third.width, "cached preview changed width after a dimensionless request")
	assert.Equal(t, baselineH, third.height, "cached preview changed height after a dimensionless request")

	fourth := probePreview(t, tc, fmt.Sprintf("id=%d&height=400", id))
	assertUsablePreview(t, fourth, "4. height=400 after dimensionless request")

	// No degenerate row may have been persisted.
	var zeroRows int64
	require.NoError(t, tc.DB.Model(&models.Preview{}).
		Where("resource_id = ? AND (width = 0 OR height = 0)", id).
		Count(&zeroRows).Error)
	assert.Zero(t, zeroRows, "a preview row with a zero dimension was persisted")
}

// TestPreview_RasterResourceSurvivesDimensionlessRequest is the positive
// control on ordinary images: the same sequence must hold, and the previews
// must actually be served (never the placeholder), so the sequence test above
// cannot be satisfied by refusing to render anything.
func TestPreview_RasterResourceSurvivesDimensionlessRequest(t *testing.T) {
	tc := SetupTestEnv(t)

	// Width/Height deliberately left unknown, as they are for any format the
	// upload path could not decode.
	resource := writeResourceFile(t, tc, "photo.png", "image/png", makeAlphaPNG(t, 400, 300), 0, 0)
	id := resource.ID

	first := probePreview(t, tc, fmt.Sprintf("id=%d&height=150", id))
	require.Equal(t, http.StatusOK, first.code, "a PNG preview should render, not redirect")
	require.True(t, first.decoded, "PNG preview should decode")
	assert.Greater(t, first.width, 0)
	assert.Greater(t, first.height, 0)

	dimensionless := probePreview(t, tc, fmt.Sprintf("id=%d", id))
	require.Equal(t, http.StatusOK, dimensionless.code, "a dimensionless PNG preview should render, not redirect")
	require.True(t, dimensionless.decoded, "dimensionless PNG preview should decode")
	assert.Greater(t, dimensionless.width, 0, "dimensionless preview was %dx%d", dimensionless.width, dimensionless.height)
	assert.Greater(t, dimensionless.height, 0, "dimensionless preview was %dx%d", dimensionless.width, dimensionless.height)

	again := probePreview(t, tc, fmt.Sprintf("id=%d&height=150", id))
	require.Equal(t, http.StatusOK, again.code, "sized preview should still render after a dimensionless request")
	require.True(t, again.decoded, "sized preview should still decode")
	assert.Equal(t, first.width, again.width, "cached preview width changed")
	assert.Equal(t, first.height, again.height, "cached preview height changed")
}

// TestPreview_RepairsAlreadyPoisonedRows covers deployments that already have
// the degenerate rows on disk: a 0x0 preview row must not be treated as the
// canonical custom thumbnail, and must not survive a read.
func TestPreview_RepairsAlreadyPoisonedRows(t *testing.T) {
	tc := SetupTestEnv(t)

	resource := writeResourceFile(t, tc, "photo.png", "image/png", makeAlphaPNG(t, 400, 300), 400, 300)
	id := resource.ID

	// Reproduce the poisoned state exactly: a Width=0/Height=0 row whose bytes
	// decode to a 0x0 image.
	poison := degenerateJPEG(t)
	require.NoError(t, tc.DB.Create(&models.Preview{
		Data:        poison,
		Width:       0,
		Height:      0,
		ContentType: "image/jpeg",
		ResourceId:  &id,
	}).Error)

	p := probePreview(t, tc, fmt.Sprintf("id=%d&height=150", id))
	require.Equal(t, http.StatusOK, p.code, "preview should recover from a poisoned row")
	require.True(t, p.decoded, "recovered preview should decode")
	assert.Greater(t, p.width, 0, "recovered preview was %dx%d", p.width, p.height)
	assert.Greater(t, p.height, 0, "recovered preview was %dx%d", p.width, p.height)

	var zeroRows int64
	require.NoError(t, tc.DB.Model(&models.Preview{}).
		Where("resource_id = ? AND (width = 0 OR height = 0)", id).
		Count(&zeroRows).Error)
	assert.Zero(t, zeroRows, "the poisoned row should have been repaired away on read")
}

// degenerateJPEG builds the 0x0 JPEG the old pipeline produced — literally
// imaging.Resize(img, 0, 0) followed by a JPEG encode — so the repair test
// operates on the real artifact rather than an approximation.
func degenerateJPEG(t *testing.T) []byte {
	t.Helper()

	src, _, err := image.Decode(bytes.NewReader(makeAlphaPNG(t, 40, 30)))
	require.NoError(t, err)

	zero := imaging.Resize(src, 0, 0, imaging.Lanczos)
	require.Zero(t, zero.Bounds().Dx(), "fixture should be 0 wide")
	require.Zero(t, zero.Bounds().Dy(), "fixture should be 0 tall")

	var buf bytes.Buffer
	require.NoError(t, imaging.Encode(&buf, zero, imaging.JPEG, imaging.JPEGQuality(80)))
	require.NotEmpty(t, buf.Bytes())
	return buf.Bytes()
}
