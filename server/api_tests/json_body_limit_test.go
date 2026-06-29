package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
)

// The shared JSON decode path historically had no body size limit (KAN-22).
// MaxJSONBodySize adds an opt-in cap; it is keyed on Content-Type so multipart
// uploads are unaffected. Default 0 keeps the unlimited behaviour.
func TestJSONBodyLimit(t *testing.T) {
	jsonH := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
	small := `{"name":"n"}`
	// Padded with an ignored field so the body is large without tripping the
	// note's own name-length validation (the point under test is body size).
	big := `{"name":"n","pad":"` + strings.Repeat("x", 4096) + `"}`

	t.Run("enforced when configured", func(t *testing.T) {
		tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
			c.MaxJSONBodySize = 64
		})
		// A body within the limit is handled normally (not size-rejected).
		if rr := doReq(tc, http.MethodPost, "/v1/note", jsonH, nil, strings.NewReader(small)); rr.Code >= 400 {
			t.Fatalf("within-limit JSON create should not be rejected, got %d (%s)", rr.Code, rr.Body.String())
		}
		// A body over the limit is rejected (the MaxBytesReader trips the decode).
		if rr := doReq(tc, http.MethodPost, "/v1/note", jsonH, nil, strings.NewReader(big)); rr.Code != http.StatusBadRequest {
			t.Fatalf("over-limit JSON body should be 400, got %d (%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("unlimited by default", func(t *testing.T) {
		tc := SetupTestEnv(t) // MaxJSONBodySize defaults to 0 = unlimited
		if rr := doReq(tc, http.MethodPost, "/v1/note", jsonH, nil, strings.NewReader(big)); rr.Code >= 400 {
			t.Fatalf("with no limit a large JSON create should not be size-rejected, got %d (%s)", rr.Code, rr.Body.String())
		}
	})
}
