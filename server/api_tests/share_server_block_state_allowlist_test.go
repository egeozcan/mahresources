package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/server"
)

// setupShareServer returns the share server's http.Handler for testing.
func setupShareServer(t *testing.T, tc *TestContext) http.Handler {
	t.Helper()
	srv := server.NewShareServer(tc.AppCtx)
	return srv.Handler()
}

// shareNote calls POST /v1/note/share and returns the shareToken.
// Panics if the response is not 200 or no token is returned.
func shareNote(t *testing.T, tc *TestContext, noteID uint) string {
	t.Helper()
	url := fmt.Sprintf("/v1/note/share?noteId=%d", noteID)
	rr := tc.MakeRequest(http.MethodPost, url, nil)
	require.Equal(t, http.StatusOK, rr.Code, "shareNote setup failed: HTTP %d body=%s", rr.Code, rr.Body.String())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	token, _ := body["shareToken"].(string)
	require.NotEmpty(t, token, "expected shareToken in response, got: %s", rr.Body.String())
	return token
}

// TestShareBlockState_RejectsNonTodoBlocks verifies BH-031:
// a gallery block state write via share token must return 403.
func TestShareBlockState_RejectsNonTodoBlocks(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	// Create a note and share it.
	note := tc.CreateDummyNote("bh031-note")
	token := shareNote(t, tc, note.ID)

	// Add a gallery block to the note.
	galleryContent, _ := json.Marshal(map[string]interface{}{"resourceIds": []int{}})
	gallery := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "gallery",
		Position: "a",
		Content:  galleryContent,
		State:    []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(gallery).Error)

	// Attempt to write state to the gallery block via the share server.
	stateBody, _ := json.Marshal(map[string]interface{}{"layout": "list", "injected": true})
	url := fmt.Sprintf("/s/%s/block/%d/state", token, gallery.ID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(stateBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"BH-031: gallery block state write via share token must be rejected with 403, got %d body=%s",
		rr.Code, rr.Body.String())
}

// TestShareBlockState_AllowsTodoBlocks verifies that todos blocks
// are still writable via the share token after the allowlist is added.
func TestShareBlockState_AllowsTodoBlocks(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	note := tc.CreateDummyNote("bh031-note-todo")
	token := shareNote(t, tc, note.ID)

	todosContent, _ := json.Marshal(map[string]interface{}{
		"items": []map[string]interface{}{{"id": "t1", "label": "Task", "checked": false}},
	})
	todos := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "todos",
		Position: "a",
		Content:  todosContent,
		State:    []byte(`{"items":[{"id":"t1","checked":false}]}`),
	}
	require.NoError(t, tc.DB.Create(todos).Error)

	newState := []byte(`{"items":[{"id":"t1","checked":true}]}`)
	url := fmt.Sprintf("/s/%s/block/%d/state", token, todos.ID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(newState))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		"todos block state write must still succeed after allowlist, got %d body=%s",
		rr.Code, rr.Body.String())
}

// TestShareBlockState_RejectsTextBlocks verifies that text blocks are rejected (403).
func TestShareBlockState_RejectsTextBlocks(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	note := tc.CreateDummyNote("bh031-note-text")
	token := shareNote(t, tc, note.ID)

	textContent, _ := json.Marshal(map[string]interface{}{"text": "hello"})
	textBlock := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "text",
		Position: "a",
		Content:  textContent,
		State:    []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(textBlock).Error)

	stateBody := []byte(`{"injected": "evil payload"}`)
	url := fmt.Sprintf("/s/%s/block/%d/state", token, textBlock.ID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(stateBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"BH-031: text block state write via share token must be rejected with 403, got %d body=%s",
		rr.Code, rr.Body.String())
}
