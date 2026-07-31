package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/models"
)

// Phase 3 guard for WS1 of the 2026-07-29 UI bug hunt (docs/todo.md).
//
// image_transform_test.go and preview_fallback_test.go pin the specific
// payloads the findings named. This file pins the *class*, and it does so by
// deriving its table from models.RasterImageContentTypes rather than repeating
// it — which is the part the per-finding tests cannot do. Adding a format to
// that allowlist without teaching the pipeline to decode it, or dropping one
// the UI still offers, fails here rather than in a user's browser.
//
// Three rules, one per defect the batch found:
//
//  1. No pixel endpoint may answer 5xx for content it cannot decode. Findings
//     10 and 11 were HTTP 500 "image: unknown format" and "encountered errors
//     during dimension calculation".
//  2. A rejection must name the format. "not an image" is not actionable, and
//     relaying the decoder's own message is what reached users.
//  3. GET /v1/resource/preview may never answer 200 with a zero dimension.
//     Findings 69/72/73: one dimensionless request persisted a 0x0 row that
//     every later request at every size then resized from, permanently.

// nonRasterPayloads are content types the application genuinely stores and the
// pixel pipeline genuinely cannot decode. Kept wider than the findings' own
// list so the sweep covers the media types a real library holds.
var nonRasterPayloads = []struct {
	name        string
	fileName    string
	contentType string
	content     []byte
}{
	{"svg", "guard.svg", "image/svg+xml", []byte(testSVG)},
	{"plain text", "guard.txt", "text/plain; charset=utf-8", []byte("just some text\n")},
	{"markdown", "guard.md", "text/markdown", []byte("# heading\n")},
	{"csv", "guard.csv", "text/csv", []byte("a,b\n1,2\n")},
	{"json", "guard.json", "application/json", []byte(`{"a":1}`)},
	{"pdf", "guard.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n")},
	{"zip", "guard.zip", "application/zip", []byte("PK\x03\x04nope")},
	{"octet stream", "guard.bin", "application/octet-stream", []byte{0x00, 0x01, 0x02}},
	{"mp4", "guard.mp4", "video/mp4", []byte("\x00\x00\x00\x18ftypmp42")},
	{"mp3", "guard.mp3", "audio/mpeg", []byte("ID3\x03\x00")},
	{"zero byte", "guard.empty", "text/plain", []byte{}},
}

// pixelEndpoints are every route that decodes a resource's bytes as a raster
// image. Crop is here as well as rotate and recalculate: the plan named two,
// and the third shares the decoder and the gate.
var pixelEndpoints = []struct {
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
		label: "crop",
		path:  "/v1/resources/crop",
		form: func(id uint) url.Values {
			return url.Values{
				"ID": {fmt.Sprint(id)}, "X": {"0"}, "Y": {"0"},
				"Width": {"4"}, "Height": {"4"},
			}
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

// TestPixelEndpoints_NeverAnswer5xxForUndecodableContent sweeps every pixel
// endpoint against every payload the pipeline cannot decode.
func TestPixelEndpoints_NeverAnswer5xxForUndecodableContent(t *testing.T) {
	// The table's premise: none of these is on the allowlist. If someone adds
	// one, this fails here rather than producing an assertion that quietly
	// tests the wrong thing.
	for _, p := range nonRasterPayloads {
		r := models.Resource{ContentType: p.contentType}
		if r.IsRasterImage() {
			t.Fatalf("nonRasterPayloads lists %q, which models.RasterImageContentTypes now "+
				"claims the pipeline can decode. The table's premise is false; move it to "+
				"the raster side or drop it.", p.contentType)
		}
	}

	for _, ep := range pixelEndpoints {
		for _, p := range nonRasterPayloads {
			t.Run(ep.label+"/"+p.name, func(t *testing.T) {
				tc := SetupTestEnv(t)
				if err := tc.DB.AutoMigrate(&models.ResourceVersion{}); err != nil {
					t.Fatalf("migrate versions: %v", err)
				}
				resource := writeResourceFile(t, tc, p.fileName, p.contentType, p.content, 0, 0)

				resp := tc.MakeFormRequest(http.MethodPost, ep.path, ep.form(resource.ID))

				if resp.Code >= 500 {
					t.Fatalf("%s on %s = %d, want 4xx.\n"+
						"An undecodable payload is a client-side mistake, not a server fault.\n"+
						"body: %s", ep.label, p.contentType, resp.Code, resp.Body.String())
				}
				if resp.Code < 400 {
					t.Fatalf("%s on %s = %d, want a rejection. The gate is open.",
						ep.label, p.contentType, resp.Code)
				}

				var payload map[string]any
				if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
					t.Fatalf("%s on %s: error body is not JSON: %s",
						ep.label, p.contentType, resp.Body.String())
				}
				msg, _ := payload["error"].(string)
				base := strings.SplitN(p.contentType, ";", 2)[0]
				if !strings.Contains(msg, base) {
					t.Errorf("%s on %s: message %q does not name the rejected format.\n"+
						"A reader cannot act on \"not an image\".", ep.label, p.contentType, msg)
				}
			})
		}
	}
}

// TestPixelEndpoints_DoNotRejectAnAllowlistedFormat is the positive control the
// sweep above needs, and it is the half that couples to the allowlist.
//
// The bytes are deliberately garbage, so every one of these still fails — the
// point is *which* refusal. A format on RasterImageContentTypes must be refused
// for its content, never for its type, or the UI is offering Rotate and Crop on
// something the server will not attempt.
func TestPixelEndpoints_DoNotRejectAnAllowlistedFormat(t *testing.T) {
	if len(models.RasterImageContentTypes) == 0 {
		t.Fatal("RasterImageContentTypes is empty; this control proves nothing")
	}

	for _, ct := range models.RasterImageContentTypes {
		for _, ep := range pixelEndpoints {
			t.Run(ep.label+"/"+strings.ReplaceAll(ct, "/", "_"), func(t *testing.T) {
				tc := SetupTestEnv(t)
				if err := tc.DB.AutoMigrate(&models.ResourceVersion{}); err != nil {
					t.Fatalf("migrate versions: %v", err)
				}
				name := "allow" + strings.ReplaceAll(strings.TrimPrefix(ct, "image/"), "+", "")
				resource := writeResourceFile(t, tc, name, ct, []byte("not really an image"), 0, 0)

				resp := tc.MakeFormRequest(http.MethodPost, ep.path, ep.form(resource.ID))

				if resp.Code >= 500 {
					t.Fatalf("%s on %s = %d, want 4xx even for corrupt bytes: %s",
						ep.label, ct, resp.Code, resp.Body.String())
				}
				var payload map[string]any
				_ = json.Unmarshal(resp.Body.Bytes(), &payload)
				msg, _ := payload["error"].(string)
				// application_context/image_format.go phrases the type refusal by
				// listing the supported formats. A format that *is* on the list
				// must never produce it.
				if strings.Contains(msg, strings.Join(models.RasterImageContentTypes, ", ")) {
					t.Errorf("%s refused %s as an unsupported format, but it is on "+
						"models.RasterImageContentTypes.\n"+
						"The allowlist and the decoder disagree, so the UI offers an\n"+
						"operation the server will not attempt.\nmessage: %s", ep.label, ct, msg)
				}
			})
		}
	}
}

// TestPreviewGuard_NeverServes200WithAZeroDimension sweeps the preview endpoint
// over the same payload table, at every combination of the width/height
// parameters, and after the request sequence that used to poison the cache.
//
// assertUsablePreview (preview_fallback_test.go) holds the invariant: either a
// redirect to the placeholder, or an image with both dimensions greater than
// zero. Never a 200 carrying 0x0.
//
// What this test pins, stated precisely, because it cannot discriminate as
// finely as it looks: reverting `resizeForThumbnail` alone leaves it green. Two
// independent changes closed this defect — the resize derives the missing axis
// from the source, *and* LoadOrCreateThumbnailForResource refuses to persist a
// zero-dimension row and lets the handler redirect to the placeholder instead —
// and either one on its own satisfies the invariant. Driving this red took
// reverting both. So it guards the composite promise ("no reader is ever served
// a 0x0 preview"), not the mechanism, and a change that removes one of the two
// layers will not be caught here.
func TestPreviewGuard_NeverServes200WithAZeroDimension(t *testing.T) {
	queries := []string{"", "&width=200", "&height=300", "&width=200&height=150"}

	for _, p := range nonRasterPayloads {
		t.Run(p.name, func(t *testing.T) {
			tc := SetupTestEnv(t)
			resource := writeResourceFile(t, tc, p.fileName, p.contentType, p.content, 0, 0)

			// The dimensionless request goes first, because that is the one that
			// persisted the degenerate row. Every later request then reads from
			// whatever it left behind, which is the whole finding.
			for _, q := range append([]string{""}, queries...) {
				label := fmt.Sprintf("%s [%s]", p.name, q)
				probe := probePreview(t, tc, fmt.Sprintf("id=%d%s", resource.ID, q))
				if probe.code >= 500 {
					t.Fatalf("%s: preview = %d, want 200 or a redirect", label, probe.code)
				}
				assertUsablePreview(t, probe, label)
			}
		})
	}
}
