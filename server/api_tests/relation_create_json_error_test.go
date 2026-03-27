package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationCreateError_ReturnsJSONNot303 verifies that POST /v1/relation
// with invalid data returns a JSON error response with an appropriate HTTP
// status code (400), not a 303 redirect.
//
// Bug: The AddRelation handler has custom error handling that checks
// Accept/Content-Type headers to decide between a redirect and a JSON error.
// When a form-encoded request arrives without an explicit Accept header
// containing "application/json", the handler responds with 303 See Other
// (an HTML-form redirect), even though every other write handler in the
// codebase uses http_utils.HandleError which properly returns JSON for
// non-HTML requests.
func TestRelationCreateError_ReturnsJSONNot303(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("form-encoded request without Accept header gets JSON error not redirect", func(t *testing.T) {
		// Send a form-encoded POST with missing required fields (FromGroupId,
		// ToGroupId are both 0). This simulates a non-browser API client using
		// form encoding without setting Accept: application/json.
		body := strings.NewReader("FromGroupId=0&ToGroupId=0&GroupRelationTypeId=1")
		req, err := http.NewRequest(http.MethodPost, "/v1/relation", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// Deliberately NOT setting Accept header

		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)

		// Should NOT be a 303 redirect — that masks the error
		assert.NotEqual(t, http.StatusSeeOther, rr.Code,
			"POST /v1/relation with invalid data should not return 303 redirect; "+
				"should return a proper error status code")

		// Should be a 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, rr.Code,
			"POST /v1/relation with missing required fields should return 400")

		// Response body should be valid JSON with an error message
		var respBody map[string]string
		err = json.Unmarshal(rr.Body.Bytes(), &respBody)
		assert.NoError(t, err, "response body should be valid JSON")
		assert.NotEmpty(t, respBody["error"], "JSON response should contain an error message")
	})

	t.Run("form-encoded request with Accept */* gets JSON error not redirect", func(t *testing.T) {
		// Same test but with Accept: */* which many HTTP clients send by default
		body := strings.NewReader("FromGroupId=0&ToGroupId=0&GroupRelationTypeId=1")
		req, err := http.NewRequest(http.MethodPost, "/v1/relation", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "*/*")

		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)

		assert.NotEqual(t, http.StatusSeeOther, rr.Code,
			"POST /v1/relation with Accept: */* should not return 303 redirect")
		assert.Equal(t, http.StatusBadRequest, rr.Code,
			"POST /v1/relation with missing required fields should return 400")
	})
}
