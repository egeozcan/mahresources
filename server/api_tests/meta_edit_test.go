package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/types"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditMeta_GroupSimpleField(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{
		Name:        "Cooking Group",
		Description: "A group about cooking",
		Meta:        types.JSON(`{"cooking":{"difficulty":"easy"}}`),
	}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"cooking.time"}, "value": {"30"}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, true, body["ok"])
	assert.Equal(t, float64(group.ID), body["id"])

	meta, ok := body["meta"].(map[string]any)
	require.True(t, ok, "meta should be an object")

	cooking, ok := meta["cooking"].(map[string]any)
	require.True(t, ok, "meta.cooking should be an object")
	assert.Equal(t, float64(30), cooking["time"])
	assert.Equal(t, "easy", cooking["difficulty"])
}

func TestEditMeta_DeepPathCreation(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{
		Name:        "Empty Meta Group",
		Description: "Group with empty meta",
		Meta:        types.JSON(`{}`),
	}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"a.b.c.d"}, "value": {`"deep_value"`}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))

	meta := body["meta"].(map[string]any)
	a := meta["a"].(map[string]any)
	b := a["b"].(map[string]any)
	c := b["c"].(map[string]any)
	assert.Equal(t, "deep_value", c["d"])
}

func TestEditMeta_PreservesExistingNestedFields(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{
		Name:        "Cooking Group",
		Description: "Existing nested fields",
		Meta:        types.JSON(`{"cooking":{"difficulty":"easy","servings":4}}`),
	}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"cooking.time"}, "value": {"30"}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))

	meta := body["meta"].(map[string]any)
	cooking := meta["cooking"].(map[string]any)
	assert.Equal(t, float64(30), cooking["time"])
	assert.Equal(t, "easy", cooking["difficulty"])
	assert.Equal(t, float64(4), cooking["servings"])
}

func TestEditMeta_MissingID(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost,
		"/v1/group/editMeta",
		url.Values{"path": {"foo"}, "value": {"1"}},
	)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestEditMeta_MissingPath(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("test")

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"value": {"1"}},
	)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestEditMeta_MissingValue(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("test")

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"foo"}},
	)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestEditMeta_ResourceEditMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	resource := &models.Resource{
		Name:     "Test Resource",
		Hash:     "testhash123",
		HashType: "SHA1",
		Location: "/test/resource.txt",
		Meta:     types.JSON(`{"existing":"value"}`),
	}
	tc.DB.Create(resource)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/resource/editMeta?id=%d", resource.ID),
		url.Values{"path": {"new_field"}, "value": {`"hello"`}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, true, body["ok"])

	meta := body["meta"].(map[string]any)
	assert.Equal(t, "value", meta["existing"])
	assert.Equal(t, "hello", meta["new_field"])
}

func TestEditMeta_NoteEditMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	note := &models.Note{
		Name:        "Test Note",
		Description: "A test note",
		Meta:        types.JSON(`{"status":"draft"}`),
	}
	tc.DB.Create(note)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/note/editMeta?id=%d", note.ID),
		url.Values{"path": {"priority"}, "value": {"5"}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, true, body["ok"])

	meta := body["meta"].(map[string]any)
	assert.Equal(t, "draft", meta["status"])
	assert.Equal(t, float64(5), meta["priority"])
}

func TestEditMeta_ResponseIncludesFullMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{
		Name:        "Full Meta Group",
		Description: "Group for meta response test",
		Meta:        types.JSON(`{"alpha":"a","beta":"b","gamma":"g"}`),
	}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"delta"}, "value": {`"d"`}},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))

	meta := body["meta"].(map[string]any)
	assert.Equal(t, "a", meta["alpha"])
	assert.Equal(t, "b", meta["beta"])
	assert.Equal(t, "g", meta["gamma"])
	assert.Equal(t, "d", meta["delta"])
}

func TestEditMeta_NonExistentEntity(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	resp := tc.MakeFormRequest(http.MethodPost,
		"/v1/group/editMeta?id=99999",
		url.Values{"path": {"foo"}, "value": {"1"}},
	)

	assert.NotEqual(t, http.StatusOK, resp.Code)
}

func TestEditMeta_InvalidJSON(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("test")

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"foo"}, "value": {`{invalid json`}},
	)

	assert.NotEqual(t, http.StatusOK, resp.Code)
}
