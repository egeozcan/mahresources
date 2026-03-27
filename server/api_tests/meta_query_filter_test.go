package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetaQueryFilterGroups verifies that MetaQuery URL parameters are parsed
// and applied when listing groups. gorilla/schema's RegisterConverter does NOT
// work for slice fields like MetaQuery []ColumnMeta, so the application must
// manually parse these values from the query string.
func TestMetaQueryFilterGroups(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create two groups with different metadata
	g1 := &models.Group{Name: "Alpha Group", Description: "first", Meta: []byte(`{"color":"red","count":5}`)}
	g2 := &models.Group{Name: "Beta Group", Description: "second", Meta: []byte(`{"color":"blue","count":10}`)}
	tc.DB.Create(g1)
	tc.DB.Create(g2)

	t.Run("MetaQuery filters groups via JSON API (query string)", func(t *testing.T) {
		// Filter for color=red using the MetaQuery format key:value
		reqURL := "/v1/groups?MetaQuery=" + url.QueryEscape("color:red")
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		err := json.Unmarshal(resp.Body.Bytes(), &groups)
		require.NoError(t, err)
		assert.Len(t, groups, 1, "MetaQuery filter should return only the group with color=red")
		if len(groups) == 1 {
			assert.Equal(t, g1.ID, groups[0].ID)
		}
	})

	t.Run("MetaQuery with explicit operator", func(t *testing.T) {
		// Filter for count > 7
		reqURL := "/v1/groups?MetaQuery=" + url.QueryEscape("count:GT:7")
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		err := json.Unmarshal(resp.Body.Bytes(), &groups)
		require.NoError(t, err)
		assert.Len(t, groups, 1, "MetaQuery filter count:GT:7 should return only the group with count=10")
		if len(groups) == 1 {
			assert.Equal(t, g2.ID, groups[0].ID)
		}
	})

	t.Run("Multiple MetaQuery params", func(t *testing.T) {
		// Filter for color=blue AND count=10
		reqURL := fmt.Sprintf("/v1/groups?MetaQuery=%s&MetaQuery=%s",
			url.QueryEscape("color:blue"),
			url.QueryEscape("count:EQ:10"))
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		err := json.Unmarshal(resp.Body.Bytes(), &groups)
		require.NoError(t, err)
		assert.Len(t, groups, 1, "Multiple MetaQuery filters should intersect")
		if len(groups) == 1 {
			assert.Equal(t, g2.ID, groups[0].ID)
		}
	})
}

// TestMetaQueryFilterNotes verifies MetaQuery works for notes.
func TestMetaQueryFilterNotes(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	n1 := &models.Note{Name: "Note One", Description: "first", Meta: []byte(`{"priority":"high"}`)}
	n2 := &models.Note{Name: "Note Two", Description: "second", Meta: []byte(`{"priority":"low"}`)}
	tc.DB.Create(n1)
	tc.DB.Create(n2)

	t.Run("MetaQuery filters notes", func(t *testing.T) {
		reqURL := "/v1/notes?MetaQuery=" + url.QueryEscape("priority:high")
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var notes []models.Note
		err := json.Unmarshal(resp.Body.Bytes(), &notes)
		require.NoError(t, err)
		assert.Len(t, notes, 1, "MetaQuery filter should return only the note with priority=high")
		if len(notes) == 1 {
			assert.Equal(t, n1.ID, notes[0].ID)
		}
	})
}

// TestMetaQueryFilterResources verifies MetaQuery works for resources.
func TestMetaQueryFilterResources(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	r1 := &models.Resource{Name: "Resource One", Meta: []byte(`{"format":"pdf"}`)}
	r2 := &models.Resource{Name: "Resource Two", Meta: []byte(`{"format":"png"}`)}
	tc.DB.Create(r1)
	tc.DB.Create(r2)

	t.Run("MetaQuery filters resources", func(t *testing.T) {
		reqURL := "/v1/resources?MetaQuery=" + url.QueryEscape("format:pdf")
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var resources []models.Resource
		err := json.Unmarshal(resp.Body.Bytes(), &resources)
		require.NoError(t, err)
		assert.Len(t, resources, 1, "MetaQuery filter should return only the resource with format=pdf")
		if len(resources) == 1 {
			assert.Equal(t, r1.ID, resources[0].ID)
		}
	})
}

// TestMetaQueryFilterViaFormEncoding verifies MetaQuery works when submitted
// as form-encoded data (as template pages would use).
func TestMetaQueryFilterViaFormEncoding(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	g1 := &models.Group{Name: "Form Group A", Description: "first", Meta: []byte(`{"status":"active"}`)}
	g2 := &models.Group{Name: "Form Group B", Description: "second", Meta: []byte(`{"status":"archived"}`)}
	tc.DB.Create(g1)
	tc.DB.Create(g2)

	t.Run("MetaQuery works via query string on GET", func(t *testing.T) {
		// Use EQ operator to get exact match, avoiding LIKE substring matching
		reqURL := "/v1/groups?MetaQuery=" + url.QueryEscape("status:EQ:active")
		resp := tc.MakeRequest(http.MethodGet, reqURL, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		err := json.Unmarshal(resp.Body.Bytes(), &groups)
		require.NoError(t, err)
		assert.Len(t, groups, 1, "MetaQuery should filter via query string")
		if len(groups) == 1 {
			assert.Equal(t, g1.ID, groups[0].ID)
		}
	})
}
