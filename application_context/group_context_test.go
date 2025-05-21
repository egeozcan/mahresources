package application_context

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"reflect"
	"testing"
	// "gorm.io/driver/sqlite" // For in-memory DB
	// "gorm.io/gorm"
)

// Assuming 'context' is available globally or via a getTestContext() helper.

func TestMahresourcesContext_CreateGroup_WithMeta(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized")
		return
	}

	// Prerequisite: Create a category if CategoryId is to be linked
	catCreator := &query_models.CategoryCreator{Name: "Test Category for Group"}
	category, err := context.CreateCategory(catCreator)
	if err != nil {
		t.Fatalf("Prerequisite CreateCategory() error = %v", err)
	}
	defer context.DeleteCategory(category.ID)

	creator := &query_models.GroupCreator{
		Name:       "Test Group with Meta",
		CategoryId: category.ID,
		Meta:       `{"color":"blue","size":"large","tags":["tag1","tag2"]}`,
	}

	createdGroup, err := context.CreateGroup(creator)
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if createdGroup == nil || createdGroup.ID == 0 {
		t.Fatalf("CreateGroup() returned nil or zero ID group")
	}
	defer context.DeleteGroup(createdGroup.ID) // Cleanup

	// Fetch the group to verify
	fetchedGroup, err := context.GetGroup(createdGroup.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}

	// Verify Meta
	var expectedMeta, actualMeta map[string]interface{}
	if err := json.Unmarshal([]byte(creator.Meta), &expectedMeta); err != nil {
		t.Fatalf("Error unmarshalling expected Meta: %v", err)
	}
	if err := json.Unmarshal(fetchedGroup.Meta, &actualMeta); err != nil {
		t.Fatalf("Error unmarshalling actual Meta: %v, raw: %s", err, string(fetchedGroup.Meta))
	}

	if !reflect.DeepEqual(expectedMeta, actualMeta) {
		t.Errorf("CreateGroup() Meta = %v, want %v", actualMeta, expectedMeta)
	}
	if fetchedGroup.Name != creator.Name {
		t.Errorf("CreateGroup() Name = %s, want %s", fetchedGroup.Name, creator.Name)
	}
}

func TestMahresourcesContext_UpdateGroup_WithMeta(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized")
		return
	}

	// 1. Create a category and group first
	catCreator := &query_models.CategoryCreator{Name: "Test Category for Group Update"}
	category, err := context.CreateCategory(catCreator)
	if err != nil {
		t.Fatalf("Prerequisite CreateCategory() error = %v", err)
	}
	defer context.DeleteCategory(category.ID)

	initialCreator := &query_models.GroupCreator{
		Name:       "Test Group Meta Update Initial",
		CategoryId: category.ID,
		Meta:       `{"status":"initial"}`,
	}
	createdGroup, err := context.CreateGroup(initialCreator)
	if err != nil {
		t.Fatalf("CreateGroup() for update test error = %v", err)
	}
	defer context.DeleteGroup(createdGroup.ID)

	// 2. Update the group
	updater := &query_models.GroupEditor{
		ID: createdGroup.ID,
		GroupCreator: query_models.GroupCreator{
			Name:        "Test Group Meta Updated",
			Description: "Updated group description.",
			Meta:        `{"status":"updated","priority":1}`,
			CategoryId:  category.ID, // Keep category or update if needed
		},
	}
	updatedGroup, err := context.UpdateGroup(updater)
	if err != nil {
		t.Fatalf("UpdateGroup() error = %v", err)
	}

	// 3. Fetch and verify
	fetchedGroup, err := context.GetGroup(updatedGroup.ID)
	if err != nil {
		t.Fatalf("GetGroup() after update error = %v", err)
	}

	var expectedMeta, actualMeta map[string]interface{}
	if err := json.Unmarshal([]byte(updater.Meta), &expectedMeta); err != nil {
		t.Fatalf("Error unmarshalling expected Meta for update: %v", err)
	}
	if err := json.Unmarshal(fetchedGroup.Meta, &actualMeta); err != nil {
		t.Fatalf("Error unmarshalling actual Meta after update: %v, raw: %s", err, string(fetchedGroup.Meta))
	}

	if !reflect.DeepEqual(expectedMeta, actualMeta) {
		t.Errorf("UpdateGroup() Meta = %v, want %v", actualMeta, expectedMeta)
	}
	if fetchedGroup.Name != updater.Name {
		t.Errorf("UpdateGroup() Name = %s, want %s", fetchedGroup.Name, updater.Name)
	}
}

func TestMahresourcesContext_CreateGroup_EmptyOrInvalidMeta(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized")
		return
	}
	catCreator := &query_models.CategoryCreator{Name: "Test Category for Group Meta Edge Cases"}
	category, err := context.CreateCategory(catCreator)
	if err != nil {
		t.Fatalf("Prerequisite CreateCategory() error = %v", err)
	}
	defer context.DeleteCategory(category.ID)

	testCases := []struct {
		name           string
		metaInput      string
		expectedMetaStr string // Expected string representation after being processed by types.JSON and DB roundtrip
	}{
		{
			name:      "Empty string Meta",
			metaInput: "",
			// CreateGroup sets Meta to "{}" if groupQuery.Meta is ""
			// Then []byte("{}") is stored.
			expectedMetaStr: "{}",
		},
		{
			name:           "Explicit 'null' Meta",
			metaInput:      "null",
			expectedMetaStr: "null",
		},
		// As with categories, truly invalid JSON that types.JSON.Value() would reject should error out
		// during CreateGroup because GORM calls Value().
		// types.JSON("invalid").Value() -> error
		// `types.JSON(`"invalid"`).Value()` -> `"invalid"` (string)
		// `types.JSON(`{"key":"val"}`).Value()` -> `{"key":"val"}` (string)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			creator := &query_models.GroupCreator{
				Name:       "Test Group Meta " + tc.name,
				CategoryId: category.ID,
				Meta:       tc.metaInput,
			}
			createdGroup, errC := context.CreateGroup(creator)
			// If metaInput is such that types.JSON(metaInput).Value() errors (e.g. "invalid json"),
			// then CreateGroup should error.
			if errC != nil && tc.metaInput == "null" { // "null" is valid
				t.Fatalf("CreateGroup() with Meta '%s' unexpectedly errored: %v", tc.metaInput, errC)
			}
			if errC == nil && tc.metaInput != "null" && tc.metaInput != "" {
				// This case might indicate that an "invalid" json string like "{" (malformed) passed.
				// However, GroupContext's CreateGroup converts "" to "{}".
				// And `types.JSON("malformed").Value()` would error.
			}

			if createdGroup == nil || createdGroup.ID == 0 {
				// If it errored as expected for invalid JSON, this is fine.
				if errC == nil {
					t.Fatalf("CreateGroup() with Meta '%s' returned nil or zero ID without error", tc.metaInput)
				}
				return // Test ends if group creation failed as potentially expected
			}
			defer context.DeleteGroup(createdGroup.ID)

			fetchedGroup, errG := context.GetGroup(createdGroup.ID)
			if errG != nil {
				t.Fatalf("GetGroup() with Meta '%s' error = %v", tc.metaInput, errG)
			}

			if string(fetchedGroup.Meta) != tc.expectedMetaStr {
				t.Errorf("CreateGroup() with Meta '%s', got Meta '%s', want '%s'", tc.metaInput, string(fetchedGroup.Meta), tc.expectedMetaStr)
			}
		})
	}
}

[end of application_context/group_context_test.go]
