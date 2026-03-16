package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelationTypeEditPreservesCategoryIds(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories for the relation type
	catA := &models.Category{Name: "From Category"}
	tc.DB.Create(catA)
	catB := &models.Category{Name: "To Category"}
	tc.DB.Create(catB)

	// Create a relation type with both categories
	relType := &models.GroupRelationType{
		Name:           "Test Relation",
		Description:    "Original description",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	tc.DB.Create(relType)

	// Send a partial JSON edit that only changes the name
	// (simulates CLI: mr relation-type edit ID --name "Renamed")
	partialBody := map[string]any{
		"Id":   relType.ID,
		"Name": "Renamed Relation",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/relationType/edit", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result models.GroupRelationType
	json.Unmarshal(resp.Body.Bytes(), &result)

	// Verify the name was updated
	var check models.GroupRelationType
	tc.DB.First(&check, relType.ID)
	assert.Equal(t, "Renamed Relation", check.Name)

	// The category IDs should be preserved, not cleared to nil
	if assert.NotNil(t, check.FromCategoryId,
		"Editing only name should not clear FromCategoryId — relation type becomes unusable") {
		assert.Equal(t, catA.ID, *check.FromCategoryId)
	}
	if assert.NotNil(t, check.ToCategoryId,
		"Editing only name should not clear ToCategoryId — relation type becomes unusable") {
		assert.Equal(t, catB.ID, *check.ToCategoryId)
	}
}
