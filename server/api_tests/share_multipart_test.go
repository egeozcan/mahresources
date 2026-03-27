package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShareNoteMultipartFormData(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Multipart Share Note")

	t.Run("Share via multipart/form-data", func(t *testing.T) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		_ = writer.WriteField("noteId", fmt.Sprintf("%d", note.ID))
		writer.Close()

		req, _ := http.NewRequest(http.MethodPost, "/v1/note/share", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code,
			"POST /v1/note/share with multipart/form-data should succeed, got: %s", rr.Body.String())

		var result map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &result)
		assert.NoError(t, err)
		assert.NotEmpty(t, result["shareToken"])
		assert.NotEmpty(t, result["shareUrl"])
	})

	t.Run("Unshare via multipart/form-data", func(t *testing.T) {
		// First share the note via query param (known working)
		url := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
		tc.MakeRequest(http.MethodPost, url, nil)

		// Now unshare via multipart/form-data
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		_ = writer.WriteField("noteId", fmt.Sprintf("%d", note.ID))
		writer.Close()

		req, _ := http.NewRequest(http.MethodDelete, "/v1/note/share", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code,
			"DELETE /v1/note/share with multipart/form-data should succeed, got: %s", rr.Body.String())
	})

	t.Run("Share via url-encoded form body", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost,
			"/v1/note/share",
			map[string][]string{"noteId": {fmt.Sprintf("%d", note.ID)}},
		)
		assert.Equal(t, http.StatusOK, resp.Code,
			"POST /v1/note/share with application/x-www-form-urlencoded should succeed, got: %s", resp.Body.String())
	})

	t.Run("Share still works via query parameter", func(t *testing.T) {
		// Unshare first
		tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)

		url := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code,
			"Query parameter path should still work")
	})
}
