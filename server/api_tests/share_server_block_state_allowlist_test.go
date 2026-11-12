package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	tc := setupShareEnabledTestEnv(t) // finding 7: sharing needs a configured share server
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
	tc := setupShareEnabledTestEnv(t) // finding 7: sharing needs a configured share server
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
	tc := setupShareEnabledTestEnv(t) // finding 7: sharing needs a configured share server
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

// createSharedTodosBlock shares a fresh note and adds a todos block with items
// "a" and "b" to it. It returns the share token and the block ID.
func createSharedTodosBlock(t *testing.T, tc *TestContext, name string) (string, uint) {
	t.Helper()
	note := tc.CreateDummyNote(name)
	token := shareNote(t, tc, note.ID)

	todos := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "todos",
		Position: "a",
		Content:  []byte(`{"items":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`),
		State:    []byte(`{"checked":[]}`),
	}
	require.NoError(t, tc.DB.Create(todos).Error)
	return token, todos.ID
}

// postShareBlockState posts body to the share server's block state endpoint.
// contentType is set on the request only when non-empty.
func postShareBlockState(shareRouter http.Handler, token string, blockID uint, contentType string, body []byte) *httptest.ResponseRecorder {
	url := fmt.Sprintf("/s/%s/block/%d/state", token, blockID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)
	return rr
}

// storedBlockState re-reads a block's State from the database.
func storedBlockState(t *testing.T, tc *TestContext, blockID uint) string {
	t.Helper()
	var block models.NoteBlock
	require.NoError(t, tc.DB.First(&block, blockID).Error)
	return string(block.State)
}

// TestShareBlockState_RejectsOversizedBody verifies that a share-token state
// write larger than the 64 KiB cap is refused with 413 and nothing is stored.
// The body is sent with and without a JSON Content-Type, because the cap must
// not depend on either.
func TestShareBlockState_RejectsOversizedBody(t *testing.T) {
	for _, contentType := range []string{"", "application/json"} {
		t.Run("contentType="+contentType, func(t *testing.T) {
			tc := setupShareEnabledTestEnv(t)
			shareRouter := setupShareServer(t, tc)
			token, blockID := createSharedTodosBlock(t, tc, "oversized-body")

			junk := strings.Repeat("A", 1<<20)
			body := []byte(`{"checked":["a"],"junk":"` + junk + `"}`)
			rr := postShareBlockState(shareRouter, token, blockID, contentType, body)

			assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code,
				"oversized share state write must be rejected with 413, got %d body=%s",
				rr.Code, rr.Body.String())
			assert.JSONEq(t, `{"checked":[]}`, storedBlockState(t, tc, blockID),
				"a rejected write must not change the stored state")
		})
	}
}

// TestShareBlockState_NormalizesUnknownKeysAndForeignIDs verifies that the
// stored state is rebuilt as {"checked": [...]} holding only the block's own
// item IDs, de-duplicated, not the raw client bytes.
func TestShareBlockState_NormalizesUnknownKeysAndForeignIDs(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)
	token, blockID := createSharedTodosBlock(t, tc, "normalize-state")

	body := []byte(`{"checked":["a","not-an-item","a"],"junk":"x"}`)
	rr := postShareBlockState(shareRouter, token, blockID, "application/json", body)

	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())
	assert.JSONEq(t, `{"checked":["a"]}`, storedBlockState(t, tc, blockID))
}

// TestShareBlockState_ValidToggleStillPersists is the positive path: checking
// and then unchecking items persists, and an empty list is stored as [] not null.
func TestShareBlockState_ValidToggleStillPersists(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)
	token, blockID := createSharedTodosBlock(t, tc, "valid-toggle")

	rr := postShareBlockState(shareRouter, token, blockID, "application/json", []byte(`{"checked":["a","b"]}`))
	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())
	assert.JSONEq(t, `{"checked":["a","b"]}`, storedBlockState(t, tc, blockID))

	rr = postShareBlockState(shareRouter, token, blockID, "application/json", []byte(`{"checked":[]}`))
	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())
	assert.JSONEq(t, `{"checked":[]}`, storedBlockState(t, tc, blockID))
	assert.Contains(t, storedBlockState(t, tc, blockID), `"checked":[]`, "empty state must marshal as an array, not null")
}

// TestShareBlockState_AcceptsBodyUnderCap checks that the cap is not so tight
// that a large but legal body is refused: 60 KiB of padding is accepted and
// stored normalised.
func TestShareBlockState_AcceptsBodyUnderCap(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)
	token, blockID := createSharedTodosBlock(t, tc, "under-cap")

	padding := strings.Repeat("A", 60<<10)
	body := []byte(`{"checked":["b"],"pad":"` + padding + `"}`)
	rr := postShareBlockState(shareRouter, token, blockID, "", body)

	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())
	assert.JSONEq(t, `{"checked":["b"]}`, storedBlockState(t, tc, blockID))
}
