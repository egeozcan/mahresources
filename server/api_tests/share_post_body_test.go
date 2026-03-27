package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShareNote_PostBody_ReadsNoteId(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Share Via POST Body")

	// Bug: POST /v1/note/share uses GetUIntQueryParameter which only reads
	// URL query string params, ignoring POST body. This test sends noteId
	// in the form body (as a POST endpoint should accept).
	formData := url.Values{}
	formData.Set("noteId", fmt.Sprintf("%d", note.ID))

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note/share", formData)

	assert.Equal(t, http.StatusOK, resp.Code,
		"POST /v1/note/share should read noteId from POST body, not just query string")

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.NotEmpty(t, result["shareToken"], "should return a share token")
}

func TestUnshareNote_PostBody_ReadsNoteId(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Unshare Via POST Body")

	// First share the note via query param (known working path)
	shareURL := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
	shareResp := tc.MakeRequest(http.MethodPost, shareURL, nil)
	assert.Equal(t, http.StatusOK, shareResp.Code, "setup: share should succeed")

	// Bug: DELETE /v1/note/share also uses GetUIntQueryParameter.
	// Send noteId in the form body.
	formData := url.Values{}
	formData.Set("noteId", fmt.Sprintf("%d", note.ID))

	resp := tc.MakeFormRequest(http.MethodDelete, "/v1/note/share", formData)

	assert.Equal(t, http.StatusOK, resp.Code,
		"DELETE /v1/note/share should read noteId from form body, not just query string")
}
