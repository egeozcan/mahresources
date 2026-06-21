package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCategoryCustomCSS_CreatePreserveClear exercises the full CustomCSS lifecycle for group
// categories: it persists on create, survives a partial update that omits it (handler_factory
// fieldWasSent preservation), and can be explicitly cleared with an empty string.
func TestCategoryCustomCSS_CreatePreserveClear(t *testing.T) {
	tc := SetupTestEnv(t)
	const css = ".group-card{outline:2px solid red}"

	resp := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name":      "CSS Cat",
		"CustomCSS": css,
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, css, created.CustomCSS, "CustomCSS should persist on create")

	// Partial update that does NOT send CustomCSS must preserve it.
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"ID":          created.ID,
		"Name":        "CSS Cat",
		"Description": "added later",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterPartial models.Category
	tc.DB.First(&afterPartial, created.ID)
	assert.Equal(t, css, afterPartial.CustomCSS, "CustomCSS must be preserved when not sent in a partial update")

	// Explicit empty string clears it.
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"ID":        created.ID,
		"Name":      "CSS Cat",
		"CustomCSS": "",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterClear models.Category
	tc.DB.First(&afterClear, created.ID)
	assert.Equal(t, "", afterClear.CustomCSS, "CustomCSS should clear when an explicit empty string is sent")
}

// TestResourceCategoryCustomCSS_CreatePreserveClear mirrors the lifecycle for resource categories.
func TestResourceCategoryCustomCSS_CreatePreserveClear(t *testing.T) {
	tc := SetupTestEnv(t)
	const css = ".resource-card{filter:grayscale(1)}"

	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{
		"Name":      "CSS RCat",
		"CustomCSS": css,
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, css, created.CustomCSS, "CustomCSS should persist on create")

	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{
		"ID":          created.ID,
		"Name":        "CSS RCat",
		"Description": "added later",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterPartial models.ResourceCategory
	tc.DB.First(&afterPartial, created.ID)
	assert.Equal(t, css, afterPartial.CustomCSS, "CustomCSS must be preserved when not sent in a partial update")

	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{
		"ID":        created.ID,
		"Name":      "CSS RCat",
		"CustomCSS": "",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterClear models.ResourceCategory
	tc.DB.First(&afterClear, created.ID)
	assert.Equal(t, "", afterClear.CustomCSS, "CustomCSS should clear when an explicit empty string is sent")
}

// TestNoteTypeCustomCSS_CreatePreserveClear mirrors the lifecycle for note types, which use the
// separate note_api_handlers.go preservation path (raw-map check for JSON updates).
func TestNoteTypeCustomCSS_CreatePreserveClear(t *testing.T) {
	tc := SetupTestEnv(t)
	const css = ".note-card{font-style:italic}"

	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", map[string]any{
		"Name":      "CSS NType",
		"CustomCSS": css,
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, css, created.CustomCSS, "CustomCSS should persist on create")

	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", map[string]any{
		"ID":          created.ID,
		"Name":        "CSS NType",
		"Description": "added later",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterPartial models.NoteType
	tc.DB.First(&afterPartial, created.ID)
	assert.Equal(t, css, afterPartial.CustomCSS, "CustomCSS must be preserved when not sent in a partial update")

	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", map[string]any{
		"ID":        created.ID,
		"Name":      "CSS NType",
		"CustomCSS": "",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var afterClear models.NoteType
	tc.DB.First(&afterClear, created.ID)
	assert.Equal(t, "", afterClear.CustomCSS, "CustomCSS should clear when an explicit empty string is sent")
}
