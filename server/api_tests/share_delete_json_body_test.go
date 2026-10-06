package api_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shareTokenOf(t *testing.T, tc *TestContext, id uint) string {
	t.Helper()
	var n models.Note
	require.NoError(t, tc.DB.First(&n, id).Error)
	if n.ShareToken == nil {
		return ""
	}
	return *n.ShareToken
}

// rawJSONRequest sends body verbatim with a JSON content type, for payloads
// encoding/json would not produce.
func rawJSONRequest(tc *TestContext, method, reqUrl, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, reqUrl, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

func TestUnshareNote_JSONBody(t *testing.T) {
	for _, key := range []string{"noteId", "id"} {
		t.Run(key, func(t *testing.T) {
			tc := setupShareEnabledTestEnv(t)
			note := tc.CreateDummyNote("Unshare Via JSON Body")
			shareNote(t, tc, note.ID)
			require.NotEmpty(t, shareTokenOf(t, tc, note.ID))

			resp := tc.MakeRequest(http.MethodDelete, "/v1/note/share", map[string]any{key: note.ID})

			assert.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
			assert.Empty(t, shareTokenOf(t, tc, note.ID), "token must be cleared")
		})
	}
}

// A JSON ID that is not a whole positive number must be refused, never
// truncated or wrapped onto a different note.
func TestUnshareNote_JSONBody_InvalidIDRefused(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	note := tc.CreateDummyNote("Must Stay Shared")
	shareNote(t, tc, note.ID)

	bodies := []string{
		fmt.Sprintf(`{"noteId": %d.5}`, note.ID),
		`{"noteId": -1}`,
		`{"noteId": 1e30}`,
		`{"noteId": "abc"}`,
		`{"noteId": null}`,
		`{}`,
		`not json`,
	}
	for _, b := range bodies {
		t.Run(b, func(t *testing.T) {
			resp := rawJSONRequest(tc, http.MethodDelete, "/v1/note/share", b)
			assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
			assert.NotEmpty(t, shareTokenOf(t, tc, note.ID), "an invalid body must not unshare anything")
		})
	}
}

// The query string keeps winning over a body, as before.
func TestUnshareNote_QueryStillWins(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	a := tc.CreateDummyNote("A")
	b := tc.CreateDummyNote("B")
	shareNote(t, tc, a.ID)
	shareNote(t, tc, b.ID)

	resp := tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/note/share?noteId=%d", a.ID), map[string]any{"noteId": b.ID})
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Empty(t, shareTokenOf(t, tc, a.ID))
	assert.NotEmpty(t, shareTokenOf(t, tc, b.ID))
}
