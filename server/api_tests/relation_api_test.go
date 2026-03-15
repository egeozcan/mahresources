package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddRelationWithNilCategoryDoesNotPanic(t *testing.T) {
	tc := SetupTestEnv(t)

	category := &models.Category{Name: "Test Category"}
	tc.DB.Create(category)

	// Group WITHOUT a category (CategoryId is nil)
	groupNoCat := &models.Group{Name: "No Category Group", Description: "test"}
	tc.DB.Create(groupNoCat)

	// Group WITH a category
	groupWithCat := &models.Group{Name: "Has Category Group", Description: "test", CategoryId: &category.ID}
	tc.DB.Create(groupWithCat)

	relType := &models.GroupRelationType{
		Name:           "TestRel",
		FromCategoryId: &category.ID,
		ToCategoryId:   &category.ID,
	}
	tc.DB.Create(relType)

	// Adding a relation where the "from" group has no category should return
	// an error, not panic with a nil pointer dereference
	payload := query_models.GroupRelationshipQuery{
		FromGroupId:         groupNoCat.ID,
		ToGroupId:           groupWithCat.ID,
		GroupRelationTypeId: relType.ID,
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/relation", payload)
	// Should get an error response (400), not a 500 panic recovery
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"creating a relation with a nil-category group should fail gracefully")
}

func TestRelationEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	// Setup data: Category, 2 Groups and 1 RelationType
	category := &models.Category{Name: "General"}
	tc.DB.Create(category)

	group1 := &models.Group{Name: "Group A", Description: "Test Group", CategoryId: &category.ID}
	tc.DB.Create(group1)

	group2 := &models.Group{Name: "Group B", Description: "Test Group", CategoryId: &category.ID}
	tc.DB.Create(group2)

	relType := &models.GroupRelationType{Name: "Dependson", FromCategoryId: &category.ID, ToCategoryId: &category.ID}
	tc.DB.Create(relType)

	t.Run("Create Relation", func(t *testing.T) {
		payload := query_models.GroupRelationshipQuery{
			FromGroupId:         group1.ID,
			ToGroupId:           group2.ID,
			GroupRelationTypeId: relType.ID,
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/relation", payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var createdRel models.GroupRelation
		err := json.Unmarshal(resp.Body.Bytes(), &createdRel)
		assert.NoError(t, err)
		assert.NotZero(t, createdRel.ID)
		assert.Equal(t, group1.ID, *createdRel.FromGroupId)
		assert.Equal(t, group2.ID, *createdRel.ToGroupId)
	})

	// Get the ID of the created relation for editing tests
	var relation models.GroupRelation
	tc.DB.First(&relation, "from_group_id = ? AND to_group_id = ?", group1.ID, group2.ID)
	assert.NotZero(t, relation.ID, "Relation should exist in DB")

	t.Run("Edit Relation Name (POST)", func(t *testing.T) {
		// Verify fix: This should be a POST request as per openapi.yaml and our fix
		newName := "Updated Relation Name"
		url := fmt.Sprintf("/v1/relation/editName?id=%d", relation.ID)
		payload := map[string]string{"Name": newName}

		resp := tc.MakeRequest(http.MethodPost, url, payload)

		// If the fix wasn't applied (still GET), this POST would likely fail with 404 or 405 depending on router strictness
		// With strict router, wrong method = 405 Method Not Allowed
		assert.Equal(t, http.StatusOK, resp.Code, "Expected POST to succeed")

		// Verify update in DB
		var updatedRel models.GroupRelation
		tc.DB.First(&updatedRel, relation.ID)
		assert.Equal(t, newName, updatedRel.Name)
	})

	t.Run("Edit Relation Description (POST)", func(t *testing.T) {
		newDesc := "New Description"
		url := fmt.Sprintf("/v1/relation/editDescription?id=%d", relation.ID)
		payload := map[string]string{"Description": newDesc}

		resp := tc.MakeRequest(http.MethodPost, url, payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var updatedRel models.GroupRelation
		tc.DB.First(&updatedRel, relation.ID)
		assert.Equal(t, newDesc, updatedRel.Description)
	})

	t.Run("Delete Relation", func(t *testing.T) {
		url := fmt.Sprintf("/v1/relation/delete?Id=%d", relation.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var check models.GroupRelation
		result := tc.DB.First(&check, relation.ID)
		assert.Error(t, result.Error, "Record should be deleted")
	})
}
