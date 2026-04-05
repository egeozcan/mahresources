//go:build postgres

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

func TestPG_TagCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-tag")
	rr := tc.MakeFormRequest("POST", "/v1/tag", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create tag: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_GroupCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-group")
	rr := tc.MakeFormRequest("POST", "/v1/group", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create group: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_NoteCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-note")
	rr := tc.MakeFormRequest("POST", "/v1/note", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create note: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_NoteWithOwner(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	groupData := url.Values{}
	groupData.Set("Name", "pg-owner-group")
	tc.MakeFormRequest("POST", "/v1/group", groupData)
	noteData := url.Values{}
	noteData.Set("Name", "pg-owned-note")
	noteData.Set("OwnerId", "1")
	rr := tc.MakeFormRequest("POST", "/v1/note", noteData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create note with owner: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_EditMeta_SimpleField(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	group := &models.Group{
		Name: "PG Meta Group",
		Meta: types.JSON(`{"cooking":{"difficulty":"easy"}}`),
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
}

func TestPG_EditMeta_DeepPath(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	group := &models.Group{Name: "PG Deep Path", Meta: types.JSON(`{}`)}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"a.b.c"}, "value": {`"deep"`}},
	)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	meta := body["meta"].(map[string]any)
	a := meta["a"].(map[string]any)
	b := a["b"].(map[string]any)
	assert.Equal(t, "deep", b["c"])
}

func TestPG_EditMeta_NullValue(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	group := &models.Group{
		Name: "PG Null Test",
		Meta: types.JSON(`{"x":1,"y":2}`),
	}
	tc.DB.Create(group)

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"x"}, "value": {"null"}},
	)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	meta := body["meta"].(map[string]any)
	_, exists := meta["x"]
	assert.True(t, exists, "x should exist as null")
	assert.Nil(t, meta["x"])
	assert.Equal(t, float64(2), meta["y"])
}

func TestPG_EditMeta_OverwritesScalar(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	group := &models.Group{
		Name: "PG Scalar Overwrite",
		Meta: types.JSON(`{"cooking":"just a string"}`),
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
	cooking, ok := meta["cooking"].(map[string]any)
	require.True(t, ok, "cooking should be an object now")
	assert.Equal(t, float64(30), cooking["time"])
}

func TestPG_BulkDeleteEmpty(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	rr := tc.MakeRequest("POST", "/v1/tags/delete", map[string]any{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestPG_MRQL(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	tagData := url.Values{}
	tagData.Set("Name", "mrql-pg-tag")
	tc.MakeFormRequest("POST", "/v1/tag", tagData)
	groupData := url.Values{}
	groupData.Set("Name", "mrql-pg-group")
	tc.MakeFormRequest("POST", "/v1/group", groupData)
	rr := tc.MakeRequest("POST", "/v1/mrql", map[string]interface{}{
		"query": `type = group AND name ~ "mrql-pg"`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("MRQL query: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
