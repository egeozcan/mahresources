package application_context

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"reflect"
	"testing"
	// "gorm.io/driver/sqlite" // Will need for in-memory DB
	// "gorm.io/gorm"
)

// Assume 'context' is available globally as per resource_context_test.go
// or we have a helper like getTestContext() for a fresh, isolated context.
// For simplicity in this step, I'll assume the global 'context' can be used,
// acknowledging this makes them more like integration tests.

func TestMahresourcesContext_CreateCategory_WithCustomFields(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized (likely due to .env issues in test runner)")
		return
	}

	creator := &query_models.CategoryCreator{
		Name:                   "Test Category CFD",
		Description:            "Category with custom fields definition.",
		CustomFieldsDefinition: `[{"name":"color","label":"Color","type":"text"},{"name":"rating","label":"Rating","type":"number"}]`,
	}

	createdCategory, err := context.CreateCategory(creator)
	if err != nil {
		t.Fatalf("CreateCategory() error = %v", err)
	}
	if createdCategory == nil || createdCategory.ID == 0 {
		t.Fatalf("CreateCategory() returned nil or zero ID category")
	}
	defer context.DeleteCategory(createdCategory.ID) // Cleanup

	// Fetch the category to verify
	fetchedCategory, err := context.GetCategory(createdCategory.ID)
	if err != nil {
		t.Fatalf("GetCategory() error = %v", err)
	}

	// Verify CustomFieldsDefinition
	// We need to compare the JSON content, not the raw string, as formatting might change.
	var expectedDef, actualDef []map[string]interface{}
	if err := json.Unmarshal([]byte(creator.CustomFieldsDefinition), &expectedDef); err != nil {
		t.Fatalf("Error unmarshalling expected CustomFieldsDefinition: %v", err)
	}
	if err := json.Unmarshal(fetchedCategory.CustomFieldsDefinition, &actualDef); err != nil {
		t.Fatalf("Error unmarshalling actual CustomFieldsDefinition: %v, raw: %s", err, string(fetchedCategory.CustomFieldsDefinition))
	}

	if !reflect.DeepEqual(expectedDef, actualDef) {
		t.Errorf("CreateCategory() CustomFieldsDefinition = %v, want %v", actualDef, expectedDef)
	}
	if fetchedCategory.Name != creator.Name {
		t.Errorf("CreateCategory() Name = %s, want %s", fetchedCategory.Name, creator.Name)
	}
}

func TestMahresourcesContext_UpdateCategory_WithCustomFields(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized")
		return
	}

	// 1. Create a category first
	initialCreator := &query_models.CategoryCreator{
		Name:                   "Test Category CFD Update Initial",
		CustomFieldsDefinition: `[{"name":"initial_field","type":"text"}]`,
	}
	createdCategory, err := context.CreateCategory(initialCreator)
	if err != nil {
		t.Fatalf("CreateCategory() for update test error = %v", err)
	}
	defer context.DeleteCategory(createdCategory.ID)

	// 2. Update the category
	updater := &query_models.CategoryEditor{
		ID: createdCategory.ID,
		CategoryCreator: query_models.CategoryCreator{
			Name:                   "Test Category CFD Updated",
			Description:            "Updated description.",
			CustomFieldsDefinition: `[{"name":"updated_field","label":"Updated Field","type":"number"}]`,
		},
	}
	updatedCategory, err := context.UpdateCategory(updater)
	if err != nil {
		t.Fatalf("UpdateCategory() error = %v", err)
	}

	// 3. Fetch and verify
	fetchedCategory, err := context.GetCategory(updatedCategory.ID)
	if err != nil {
		t.Fatalf("GetCategory() after update error = %v", err)
	}

	var expectedDef, actualDef []map[string]interface{}
	if err := json.Unmarshal([]byte(updater.CustomFieldsDefinition), &expectedDef); err != nil {
		t.Fatalf("Error unmarshalling expected CustomFieldsDefinition for update: %v", err)
	}
	if err := json.Unmarshal(fetchedCategory.CustomFieldsDefinition, &actualDef); err != nil {
		t.Fatalf("Error unmarshalling actual CustomFieldsDefinition after update: %v, raw: %s", err, string(fetchedCategory.CustomFieldsDefinition))
	}

	if !reflect.DeepEqual(expectedDef, actualDef) {
		t.Errorf("UpdateCategory() CustomFieldsDefinition = %v, want %v", actualDef, expectedDef)
	}
	if fetchedCategory.Name != updater.Name {
		t.Errorf("UpdateCategory() Name = %s, want %s", fetchedCategory.Name, updater.Name)
	}
	if fetchedCategory.Description != updater.Description {
		t.Errorf("UpdateCategory() Description = %s, want %s", fetchedCategory.Description, updater.Description)
	}
}

func TestMahresourcesContext_CreateCategory_EmptyOrInvalidCustomFields(t *testing.T) {
	if context == nil {
		t.Skip("Skipping test, context not initialized")
		return
	}
	testCases := []struct {
		name           string
		cfdInput       string
		expectedCFDStr string // Expected string representation after being processed by types.JSON
	}{
		{
			name:           "Empty string CFD",
			cfdInput:       "",
			expectedCFDStr: "null", // types.JSON("").Scan("") -> j becomes "null" via types.JSON.Scan
		},
		{
			name:           "Explicit 'null' CFD",
			cfdInput:       "null",
			expectedCFDStr: "null",
		},
		// Invalid JSON string will be stored as is by types.JSON("invalid json")
		// but GORM might fail if DB column type expects valid JSON.
		// The types.JSON.Value() method would also fail to marshal it.
		// For this test, we check what's stored if CreateCategory doesn't error out before DB.
		// The current implementation of types.JSON(`...`).Value() will error if the content is not valid JSON.
		// GORM uses .Value(). So, saving "invalid json" might error at DB level.
		// Let's test with a string that GORM might accept but is not a list of definitions.
		{
			name:           "Non-array JSON string for CFD",
			cfdInput:       `{"greeting":"hello"}`,
			expectedCFDStr: `{"greeting":"hello"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			creator := &query_models.CategoryCreator{
				Name:                   "Test Category " + tc.name,
				CustomFieldsDefinition: tc.cfdInput,
			}
			createdCategory, err := context.CreateCategory(creator)
			if err != nil {
				// Depending on DB (SQLite vs Postgres JSON type validation),
				// saving truly "invalid" JSON might error here.
				// For now, assuming it might pass if types.JSON.Value() allows it.
				// The current types.JSON.Value() tries to marshal, so "invalid" (non-quoted) would fail.
				// `types.JSON("invalid").Value()` -> error
				// `types.JSON(`"invalid"`).Value()` -> `"invalid"` (string)
				// `types.JSON(`{"key":"val"}`).Value()` -> `{"key":"val"}` (string)
				// This test depends on how robust the DB and GORM type handling is.
				// If cfdInput is `""`, types.JSON("") is created. Value() returns nil. DB gets NULL. Scan(nil) makes it "null".
				// If cfdInput is `"null"`, types.JSON("null") is created. Value() returns `"null"`. DB gets "null". Scan("null") makes it "null".
				if tc.cfdInput == `{"greeting":"hello"}` { // this is valid json
					t.Fatalf("CreateCategory() with '%s' error = %v", tc.cfdInput, err)
				}
				// For truly invalid JSON that Value() would reject, we should expect an error from CreateCategory
				// For now, let's assume the current types.JSON and context code passes it to DB.
				// This part of test might need refinement based on DB behavior.
			}
			if createdCategory == nil || createdCategory.ID == 0 {
				t.Fatalf("CreateCategory() with '%s' returned nil or zero ID", tc.cfdInput)
			}
			defer context.DeleteCategory(createdCategory.ID)

			fetchedCategory, err := context.GetCategory(createdCategory.ID)
			if err != nil {
				t.Fatalf("GetCategory() with '%s' error = %v", tc.cfdInput, err)
			}

			// types.JSON("") becomes "null" after a round trip if DB stores NULL for empty JSON.
			// types.JSON("null") stays "null".
			// types.JSON(`{"greeting":"hello"}`) stays `{"greeting":"hello"}`.
			if string(fetchedCategory.CustomFieldsDefinition) != tc.expectedCFDStr {
				t.Errorf("CreateCategory() with CFD '%s', got '%s', want '%s'", tc.cfdInput, string(fetchedCategory.CustomFieldsDefinition), tc.expectedCFDStr)
			}
		})
	}
}
[end of application_context/category_context_test.go]
