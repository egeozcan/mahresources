package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBulkDeleteTags_EmptyIDs_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// POST /v1/tags/delete with empty body should return 400, not 500
	resp := tc.MakeRequest(http.MethodPost, "/v1/tags/delete", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete tags with no IDs should return 400 Bad Request")
}

func TestBulkDeleteNotes_EmptyIDs_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/notes/delete", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete notes with no IDs should return 400 Bad Request")
}

func TestBulkDeleteGroups_EmptyIDs_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/groups/delete", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete groups with no IDs should return 400 Bad Request")
}

func TestBulkDeleteResources_EmptyIDs_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/resources/delete", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete resources with no IDs should return 400 Bad Request")
}

func TestBulkDeleteTags_EmptyIDArray_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Explicitly passing an empty ID array should also return 400
	resp := tc.MakeRequest(http.MethodPost, "/v1/tags/delete", map[string]any{
		"ID": []uint{},
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete tags with empty ID array should return 400 Bad Request")
}

func TestBulkDeleteNotes_EmptyIDArray_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/notes/delete", map[string]any{
		"ID": []uint{},
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete notes with empty ID array should return 400 Bad Request")
}

func TestBulkDeleteGroups_EmptyIDArray_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/groups/delete", map[string]any{
		"ID": []uint{},
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete groups with empty ID array should return 400 Bad Request")
}

func TestBulkDeleteResources_EmptyIDArray_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/resources/delete", map[string]any{
		"ID": []uint{},
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"bulk delete resources with empty ID array should return 400 Bad Request")
}
