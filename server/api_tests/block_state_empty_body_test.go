package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockStatePatch_EmptyBodyTextBlock(t *testing.T) {
	tc := SetupTestEnv(t)

	note := tc.CreateDummyNote("Block State Empty Body Test")

	// Use a text block: TextBlockType.ValidateState always returns nil,
	// so nil state will bypass validation and hit the NOT NULL constraint.
	block := tc.CreateDummyBlock(note.ID, "text", `{"text": "hello"}`, "n")

	// PATCH with empty JSON body {} — state field is missing, so body.State is nil
	stateURL := fmt.Sprintf("/v1/note/block/state?id=%d", block.ID)
	resp := tc.MakeRequest(http.MethodPatch, stateURL, map[string]any{})

	// Should return 400 with a user-friendly error, NOT a 500 or SQL constraint error
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"PATCH block state with empty body should return 400, not crash with constraint error")

	var errResp map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &errResp)
	if err == nil {
		if msg, ok := errResp["error"]; ok {
			assert.NotContains(t, msg, "NOT NULL constraint",
				"error message should not expose raw SQL constraint errors")
		}
	}
}

func TestBlockStatePatch_NullStateTextBlock(t *testing.T) {
	tc := SetupTestEnv(t)

	note := tc.CreateDummyNote("Block State Null Test")

	// Use a text block
	block := tc.CreateDummyBlock(note.ID, "text", `{"text": "hello"}`, "n")

	// PATCH with explicit null state: {"state": null}
	stateURL := fmt.Sprintf("/v1/note/block/state?id=%d", block.ID)
	resp := tc.MakeRequest(http.MethodPatch, stateURL, map[string]any{
		"state": nil,
	})

	// Should return 400: null is not valid state
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"PATCH block state with null state should return 400")
}

func TestBlockStatePatch_EmptyBodyTodosBlock(t *testing.T) {
	tc := SetupTestEnv(t)

	note := tc.CreateDummyNote("Block State Todos Empty Body Test")

	// Use a todos block — ValidateState does json.Unmarshal which fails on nil
	block := tc.CreateDummyBlock(note.ID, "todos",
		`{"items": [{"id": "x1", "label": "Task"}]}`, "n")

	// PATCH with empty JSON body {} — state field is missing
	stateURL := fmt.Sprintf("/v1/note/block/state?id=%d", block.ID)
	resp := tc.MakeRequest(http.MethodPatch, stateURL, map[string]any{})

	// Should return 400
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"PATCH block state with empty body should return 400")

	// The error should be user-friendly, not "unexpected end of JSON input"
	var errResp map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &errResp)
	if err == nil {
		if msg, ok := errResp["error"].(string); ok {
			assert.Contains(t, msg, "state",
				"error message should mention the missing 'state' field")
		}
	}
}
