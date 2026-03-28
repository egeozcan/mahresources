package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationTypeDescriptionCanBeCleared verifies that a user can clear the
// Description field of a RelationType by sending an explicit empty string.
// BUG: EditRelationType guards with `if query.Description != ""` so once set,
// Description can never be cleared.
func TestRelationTypeDescriptionCanBeCleared(t *testing.T) {
	tc := SetupTestEnv(t)

	catA := &models.Category{Name: "RT Clear From"}
	catB := &models.Category{Name: "RT Clear To"}
	tc.DB.Create(catA)
	tc.DB.Create(catB)

	// Create a relation type with a non-empty description
	relType := &models.GroupRelationType{
		Name:           "Clearable RT",
		Description:    "Will be cleared",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	tc.DB.Create(relType)

	// Send a JSON edit that explicitly clears Description
	updateBody := map[string]any{
		"Id":          relType.ID,
		"Name":        "Clearable RT",
		"Description": "",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/relationType/edit", updateBody)
	require.Equal(t, http.StatusOK, resp.Code, "editing relation type should succeed")

	var check models.GroupRelationType
	tc.DB.First(&check, relType.ID)
	assert.Equal(t, "", check.Description,
		"Description should be cleared to empty string after explicit update with empty value")
}

// TestRelationDescriptionCanBeCleared verifies that a user can clear the
// Description field of a GroupRelation by sending an explicit empty string.
// BUG: EditRelation guards with `if query.Description != ""` so once set,
// Description can never be cleared.
func TestRelationDescriptionCanBeCleared(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "Rel Clear Cat"}
	tc.DB.Create(cat)

	relType := &models.GroupRelationType{
		Name:           "Rel Clear Type",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	groupA := &models.Group{Name: "Clear From", CategoryId: &cat.ID}
	groupB := &models.Group{Name: "Clear To", CategoryId: &cat.ID}
	tc.DB.Create(groupA)
	tc.DB.Create(groupB)

	// Create a relation with description
	createResp := tc.MakeRequest(http.MethodPost, "/v1/relation", map[string]any{
		"FromGroupId":         groupA.ID,
		"ToGroupId":           groupB.ID,
		"GroupRelationTypeId": relType.ID,
		"Name":                "Relation With Desc",
		"Description":         "Should be clearable",
	})
	require.Equal(t, http.StatusOK, createResp.Code)
	var created models.GroupRelation
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	// Verify description was set
	var before models.GroupRelation
	tc.DB.First(&before, created.ID)
	require.Equal(t, "Should be clearable", before.Description)

	// Now edit to clear Description
	editResp := tc.MakeRequest(http.MethodPost, "/v1/relation", map[string]any{
		"Id":          created.ID,
		"Name":        "Relation With Desc",
		"Description": "",
	})
	require.Equal(t, http.StatusOK, editResp.Code)

	var after models.GroupRelation
	tc.DB.First(&after, created.ID)
	assert.Equal(t, "", after.Description,
		"Description should be cleared to empty string after explicit update with empty value")
}

// TestRelationTypeFormPartialUpdatePreservesDescription ensures that a
// form-encoded partial update (e.g. only Name sent) does not wipe Description.
func TestRelationTypeFormPartialUpdatePreservesDescription(t *testing.T) {
	tc := SetupTestEnv(t)

	catA := &models.Category{Name: "RT Form From"}
	catB := &models.Category{Name: "RT Form To"}
	tc.DB.Create(catA)
	tc.DB.Create(catB)

	relType := &models.GroupRelationType{
		Name:           "Form Preserve RT",
		Description:    "Keep this description",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	tc.DB.Create(relType)

	// Send a form-encoded edit with only Name (no Description field)
	formData := url.Values{}
	formData.Set("Id", strconv.Itoa(int(relType.ID)))
	formData.Set("Name", "Renamed RT")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/relationType/edit", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	var check models.GroupRelationType
	tc.DB.First(&check, relType.ID)
	assert.Equal(t, "Renamed RT", check.Name)
	assert.Equal(t, "Keep this description", check.Description,
		"Form-encoded partial update with only Name should not wipe Description")
}

// TestRelationFormPartialUpdatePreservesDescription ensures that a
// form-encoded partial update (e.g. only Name sent) does not wipe Description.
func TestRelationFormPartialUpdatePreservesDescription(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "Rel Form Cat"}
	tc.DB.Create(cat)

	relType := &models.GroupRelationType{
		Name:           "Rel Form Type",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	groupA := &models.Group{Name: "Form From", CategoryId: &cat.ID}
	groupB := &models.Group{Name: "Form To", CategoryId: &cat.ID}
	tc.DB.Create(groupA)
	tc.DB.Create(groupB)

	// Create a relation with a description
	rel := &models.GroupRelation{
		FromGroupId:    &groupA.ID,
		ToGroupId:      &groupB.ID,
		RelationTypeId: &relType.ID,
		Name:           "Form Rel",
		Description:    "Preserve this",
	}
	tc.DB.Create(rel)

	// Form-encoded edit with only Name (no Description field)
	formData := url.Values{}
	formData.Set("Id", strconv.Itoa(int(rel.ID)))
	formData.Set("Name", "Renamed Rel")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/relation", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	var check models.GroupRelation
	tc.DB.First(&check, rel.ID)
	assert.Equal(t, "Renamed Rel", check.Name)
	assert.Equal(t, "Preserve this", check.Description,
		"Form-encoded partial update with only Name should not wipe Description")
}

// TestNoteTypeFormPartialUpdatePreservesFields verifies that a form-encoded
// partial update of a NoteType (e.g. only Name sent) does not wipe other
// fields like Description, CustomHeader, etc.
// BUG: The form-encoded path in GetAddNoteTypeHandler does NOT have formHasField
// protection, so partial form submits wipe all unsent fields.
func TestNoteTypeFormPartialUpdatePreservesFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note type via JSON with all fields populated
	createBody := map[string]any{
		"Name":          "Form Partial NT",
		"Description":   "Keep this desc",
		"CustomHeader":  "<h1>Keep header</h1>",
		"CustomSidebar": "<nav>Keep sidebar</nav>",
		"CustomSummary": "<p>Keep summary</p>",
		"CustomAvatar":  "<img src='keep.png'>",
	}
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, createResp.Code)

	var created models.NoteType
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	require.Equal(t, "Keep this desc", created.Description)

	// Step 2: Send a form-encoded update with ONLY Name
	formData := url.Values{}
	formData.Set("ID", strconv.Itoa(int(created.ID)))
	formData.Set("Name", "Renamed NT")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note/noteType", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	// Step 3: Verify all other fields are preserved
	var check models.NoteType
	tc.DB.First(&check, created.ID)
	assert.Equal(t, "Renamed NT", check.Name)
	assert.Equal(t, "Keep this desc", check.Description,
		"Form-encoded partial update should preserve Description")
	assert.Equal(t, "<h1>Keep header</h1>", check.CustomHeader,
		"Form-encoded partial update should preserve CustomHeader")
	assert.Equal(t, "<nav>Keep sidebar</nav>", check.CustomSidebar,
		"Form-encoded partial update should preserve CustomSidebar")
	assert.Equal(t, "<p>Keep summary</p>", check.CustomSummary,
		"Form-encoded partial update should preserve CustomSummary")
	assert.Equal(t, "<img src='keep.png'>", check.CustomAvatar,
		"Form-encoded partial update should preserve CustomAvatar")
}
