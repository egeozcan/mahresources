package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationTypeEditNameEndpointExists verifies that /v1/relationType/editName
// exists and works. Previously only /v1/relation/editName existed, which
// targeted GroupRelation instead of GroupRelationType.
func TestRelationTypeEditNameEndpointExists(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a Category for the relation type
	catPayload := query_models.CategoryCreator{Name: "RelTypeEditCat"}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", catPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var category models.Category
	json.Unmarshal(resp.Body.Bytes(), &category)

	// Create a relation type
	rtPayload := struct {
		Name         string
		FromCategory uint
		ToCategory   uint
	}{
		Name:         "OriginalRelTypeName",
		FromCategory: category.ID,
		ToCategory:   category.ID,
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/relationType", rtPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var relType models.GroupRelationType
	json.Unmarshal(resp.Body.Bytes(), &relType)
	require.Greater(t, relType.ID, uint(0))

	// Try to edit the name via /v1/relationType/editName
	formData := url.Values{}
	formData.Set("name", "RenamedRelType")

	resp = tc.MakeFormRequest(http.MethodPost, fmt.Sprintf("/v1/relationType/editName?id=%d", relType.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code,
		"relationType/editName should exist and succeed, got: %s", resp.Body.String())
}
