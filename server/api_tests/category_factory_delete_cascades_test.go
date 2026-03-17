package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// TestCategoryDeleteViaAPI_DanglingFK demonstrates that the /v1/category/delete
// API endpoint leaves groups with a dangling category_id foreign key.
//
// The route /v1/category/delete (server/routes.go:312) is wired to
// categoryFactory.DeleteHandler(), which calls CRUDWriter.Delete
// (application_context/generic_crud.go:134-138):
//
//	func (w *CRUDWriter[T, C]) Delete(id uint) error {
//	    var entity T
//	    return w.db.Select(clause.Associations).Delete(&entity, id).Error
//	}
//
// For Category, Select(clause.Associations) includes the Groups has_many
// association (constraint OnDelete:SET NULL). GORM's Select+Delete does NOT
// honor the SET NULL constraint; it simply deletes the category row without
// nullifying groups' category_id.
//
// The correct implementation (DeleteCategory in category_context.go:155)
// explicitly runs UPDATE groups SET category_id = NULL first, but the HTTP
// route never calls it — it uses the generic factory instead.
//
// Impact: after deleting a category via the API, all groups that belonged to it
// retain a category_id pointing to a non-existent row. This causes:
//   - Template rendering errors when the UI tries to display the category name
//   - Incorrect query results when filtering by category
//   - Data integrity violations if foreign key checks are enabled
func TestCategoryDeleteViaAPI_DanglingFK(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a category
	cat := &models.Category{Name: "API Deletable Category"}
	require.NoError(t, tc.DB.Create(cat).Error)
	catID := cat.ID

	// Step 2: Create a group in that category
	group := &models.Group{
		Name:       "Important Group That Should Survive",
		CategoryId: &catID,
	}
	require.NoError(t, tc.DB.Create(group).Error)
	groupID := group.ID

	// Verify the group exists and has the category
	var check models.Group
	require.NoError(t, tc.DB.First(&check, groupID).Error)
	require.NotNil(t, check.CategoryId)
	assert.Equal(t, catID, *check.CategoryId)

	// Step 3: Delete the category via the HTTP API
	resp := tc.MakeRequest(http.MethodPost, "/v1/category/delete", map[string]interface{}{
		"ID": catID,
	})
	require.Equal(t, http.StatusOK, resp.Code,
		"DELETE should succeed; body: %s", resp.Body.String())

	// Step 4: Verify the category is actually deleted
	var deletedCat models.Category
	err := tc.DB.First(&deletedCat, catID).Error
	require.Error(t, err, "category should be deleted")

	// Step 5: The group should still exist
	var afterDelete models.Group
	err = tc.DB.First(&afterDelete, groupID).Error
	require.NoError(t, err, "group should still exist after category deletion")

	// Step 6: The group's CategoryId should be NULL
	// BUG: The generic CRUDWriter.Delete path does not clear the FK.
	// The group is left with a dangling category_id pointing to the deleted
	// category row. The correct DeleteCategory method nullifies the FK first,
	// but the API route uses the factory's generic Delete instead.
	assert.Nil(t, afterDelete.CategoryId,
		"BUG: group's CategoryId should be NULL after its category is deleted via "+
			"the API, but it still points to the deleted category (dangling FK). "+
			"The route /v1/category/delete uses CRUDWriter.Delete (generic_crud.go) "+
			"instead of the correct DeleteCategory (category_context.go) which "+
			"explicitly nullifies the FK before deleting.")
}
