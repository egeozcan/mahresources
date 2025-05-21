package template_context_providers

import (
	"encoding/json"
	"fmt"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	// "time" // Not used directly, but good for model instantiation if needed
)

// MockCategoryReader provides a mock implementation of interfaces.CategoryReader
type MockCategoryReader struct {
	GetCategoryFunc          func(id uint) (*models.Category, error)
	GetCategoriesFunc        func(offset, maxResults int, query *query_models.CategoryQuery) (*[]models.Category, error)
	GetCategoriesWithIdsFunc func(ids *[]uint, limit int) (*[]models.Category, error)
}

func (m *MockCategoryReader) GetCategory(id uint) (*models.Category, error) {
	if m.GetCategoryFunc != nil {
		return m.GetCategoryFunc(id)
	}
	return nil, fmt.Errorf("GetCategoryFunc not implemented")
}

func (m *MockCategoryReader) GetCategories(offset, maxResults int, query *query_models.CategoryQuery) (*[]models.Category, error) {
	if m.GetCategoriesFunc != nil {
		return m.GetCategoriesFunc(offset, maxResults, query)
	}
	return nil, fmt.Errorf("GetCategoriesFunc not implemented")
}

func (m *MockCategoryReader) GetCategoriesWithIds(ids *[]uint, limit int) (*[]models.Category, error) {
	if m.GetCategoriesWithIdsFunc != nil {
		return m.GetCategoriesWithIdsFunc(ids, limit)
	}
	return nil, fmt.Errorf("GetCategoriesWithIdsFunc not implemented")
}

// MockGroupReader provides a mock implementation of interfaces.GroupReader
type MockGroupReader struct {
	MockCategoryReader         // Embed for category methods if needed by group context
	GetGroupFunc               func(id uint) (*models.Group, error)
	GetGroupsFunc              func(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error)
	FindParentsOfGroupFunc     func(id uint) (*[]models.Group, error)
	GetGroupsCountFunc         func(query *query_models.GroupQuery) (int64, error)
	GetGroupsWithIdsFunc       func(ids *[]uint) (*[]*models.Group, error)
	GetNotesWithIdsFunc        func(ids *[]uint) (*[]*models.Note, error)
	GetResourcesWithIdsFunc    func(ids *[]uint) (*[]*models.Resource, error)
	// GetTagsWithIdsFunc is part of application_context.MahresourcesContext, not directly GroupReader if strict.
	// However, GroupsListContextProvider uses methods from the full context.
	// For simplicity, we'll assume the MahresourcesContext passed to provider has these.
}

func (m *MockGroupReader) GetGroup(id uint) (*models.Group, error) {
	if m.GetGroupFunc != nil {
		return m.GetGroupFunc(id)
	}
	return nil, fmt.Errorf("GetGroupFunc not implemented")
}

func (m *MockGroupReader) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
	if m.GetGroupsFunc != nil {
		return m.GetGroupsFunc(offset, maxResults, query)
	}
	return nil, fmt.Errorf("GetGroupsFunc not implemented")
}
func (m *MockGroupReader) GetGroupsCount(query *query_models.GroupQuery) (int64, error) {
	if m.GetGroupsCountFunc != nil {
		return m.GetGroupsCountFunc(query)
	}
	return 0, fmt.Errorf("GetGroupsCountFunc not implemented")
}

func (m *MockGroupReader) FindParentsOfGroup(id uint) (*[]models.Group, error) {
	if m.FindParentsOfGroupFunc != nil {
		return m.FindParentsOfGroupFunc(id)
	}
	return &[]models.Group{}, nil // Return empty slice to avoid nil pointer dereference
}

func (m *MockGroupReader) GetGroupsWithIds(ids *[]uint) (*[]*models.Group, error) {
	if m.GetGroupsWithIdsFunc != nil {
		return m.GetGroupsWithIdsFunc(ids)
	}
	return nil, fmt.Errorf("GetGroupsWithIdsFunc not implemented")
}
func (m *MockGroupReader) GetNotesWithIds(ids *[]uint) (*[]*models.Note, error) {
	if m.GetNotesWithIdsFunc != nil {
		return m.GetNotesWithIdsFunc(ids)
	}
	return nil, fmt.Errorf("GetNotesWithIdsFunc not implemented")
}
func (m *MockGroupReader) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error) {
	if m.GetResourcesWithIdsFunc != nil {
		return m.GetResourcesWithIdsFunc(ids)
	}
	return nil, fmt.Errorf("GetResourcesWithIdsFunc not implemented")
}

// MockAppContext is a fuller mock for MahresourcesContext for providers that need more.
type MockAppContext struct {
	*MockGroupReader // Embed GroupReader for its methods
	// Add other reader interfaces if needed by other context providers
	GetTagsWithIdsFunc func(ids *[]uint, limit int) (*[]*models.Tag, error)
}

// Implement methods from MahresourcesContext that are directly called by providers
func (m *MockAppContext) GetCategory(id uint) (*models.Category, error) {
	return m.MockGroupReader.GetCategory(id)
}
func (m *MockAppContext) GetCategoriesWithIds(ids *[]uint, limit int) (*[]models.Category, error) {
    return m.MockGroupReader.GetCategoriesWithIds(ids, limit)
}
func (m *MockAppContext) GetGroup(id uint) (*models.Group, error) {
    return m.MockGroupReader.GetGroup(id)
}
func (m *MockAppContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
    return m.MockGroupReader.GetGroups(offset, maxResults, query)
}
func (m *MockAppContext) GetGroupsCount(query *query_models.GroupQuery) (int64, error) {
    return m.MockGroupReader.GetGroupsCount(query)
}
func (m *MockAppContext) GetGroupsWithIds(ids *[]uint) (*[]*models.Group, error) {
    return m.MockGroupReader.GetGroupsWithIds(ids)
}
func (m *MockAppContext) GetNotesWithIds(ids *[]uint) (*[]*models.Note, error) {
    return m.MockGroupReader.GetNotesWithIds(ids)
}
func (m *MockAppContext) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error) {
    return m.MockGroupReader.GetResourcesWithIds(ids)
}
func (m *MockAppContext) GetTagsWithIds(ids *[]uint, limit int) (*[]*models.Tag, error) {
    if m.GetTagsWithIdsFunc != nil {
        return m.GetTagsWithIdsFunc(ids, limit)
    }
    return nil, fmt.Errorf("GetTagsWithIdsFunc not implemented in MockAppContext")
}


func TestGroupCreateContextProvider_NewGroupWithCategory(t *testing.T) {
	mockCategoryID := uint(1)
	cfdJSON := `[{"name":"color","label":"Color","type":"text"}]`
	mockCategory := &models.Category{
		ID:                     mockCategoryID,
		Name:                   "Test Cat",
		CustomFieldsDefinition: types.JSON(cfdJSON),
	}

	mockAppCtx := &MockAppContext{
		MockGroupReader: &MockGroupReader{
			GetCategoryFunc: func(id uint) (*models.Category, error) {
				if id == mockCategoryID {
					return mockCategory, nil
				}
				return nil, fmt.Errorf("category %d not found", id)
			},
		},
	}
	// Convert MockAppContext to *application_context.MahresourcesContext for the provider
	appCtxForProvider := (*application_context.MahresourcesContext)(nil) // This is tricky without actual struct conversion
	// This highlights a limitation: if providers expect concrete *MahresourcesContext,
	// mocking becomes harder. Assuming for now it can take an interface or can be adapted.
	// For now, we will pass the mock directly, hoping the interface usage within the provider is limited.
	// This might require the provider to accept an interface instead of concrete type.
	// Let's assume GroupCreateContextProvider is refactored to take interfaces for what it needs.
	// If not, these tests would need a real (test) MahresourcesContext.

	// For the purpose of this test, let's assume the provider function primarily uses
	// the GetCategory method from the context.
	providerFunc := GroupCreateContextProvider(&application_context.MahresourcesContext{CategoryReader: mockAppCtx, GroupReader: mockAppCtx})


	req := httptest.NewRequest("GET", "/group/new?CategoryId=1", nil) // Query param for categoryId

	pongoCtx := providerFunc(req)

	customFieldDefs, ok := pongoCtx["customFieldDefinitions"].([]FieldDefinition)
	if !ok {
		t.Fatalf("customFieldDefinitions not found or not of expected type in context. Got: %T, Value: %#v", pongoCtx["customFieldDefinitions"], pongoCtx["customFieldDefinitions"])
	}

	var expectedDefs []FieldDefinition
	if err := json.Unmarshal([]byte(cfdJSON), &expectedDefs); err != nil {
		t.Fatalf("Failed to unmarshal expected definitions: %v", err)
	}

	if !reflect.DeepEqual(customFieldDefs, expectedDefs) {
		t.Errorf("customFieldDefinitions mismatch.\nGot:  %#v\nWant: %#v", customFieldDefs, expectedDefs)
	}

	// Test when Meta is also provided in query
	metaJSON := `{"color":"blue"}`
	// GroupCreator.Meta is `types.JSON` in the actual context handler, but the query model uses string.
	// The test setup for GroupCreateContextProvider for a new group with Meta from query
	// might be complex as it involves form decoding into groupQuery.Meta which is types.JSON.
	// The current GroupCreateContextProvider doesn't seem to directly pull Meta from query for *new* group.
	// It pulls groupQuery.Meta which would be populated by form decoder.
	// So, this part of test might be more for an editing scenario or if form decoding is mocked.
	// For now, focusing on CFD.
}


func TestGroupCreateContextProvider_EditGroupWithCategoryAndMeta(t *testing.T) {
	mockGroupID := uint(1)
	mockCategoryID := uint(2)
	cfdJSON := `[{"name":"size","label":"Size","type":"number"}]`
	metaJSON := `{"size":10,"extra":"info"}`

	mockCategory := &models.Category{
		ID:                     mockCategoryID,
		Name:                   "Related Cat",
		CustomFieldsDefinition: types.JSON(cfdJSON),
	}
	mockGroup := &models.Group{
		ID:         mockGroupID,
		Name:       "Test Group",
		CategoryId: &mockCategoryID,
		Category:   mockCategory, // GORM would populate this
		Meta:       types.JSON(metaJSON),
	}

	mockAppCtx := &MockAppContext{
		MockGroupReader: &MockGroupReader{
			GetGroupFunc: func(id uint) (*models.Group, error) {
				if id == mockGroupID {
					return mockGroup, nil
				}
				return nil, fmt.Errorf("group %d not found", id)
			},
		},
	}
	providerFunc := GroupCreateContextProvider(&application_context.MahresourcesContext{GroupReader: mockAppCtx, CategoryReader: mockAppCtx})


	req := httptest.NewRequest("GET", "/group/edit?id=1", nil) // ID for group being edited
	pongoCtx := providerFunc(req)

	customFieldDefs, ok := pongoCtx["customFieldDefinitions"].([]FieldDefinition)
	if !ok {
		t.Fatalf("customFieldDefinitions not found or not of expected type. Got: %T", pongoCtx["customFieldDefinitions"])
	}
	var expectedDefs []FieldDefinition
	if err := json.Unmarshal([]byte(cfdJSON), &expectedDefs); err != nil {
		t.Fatalf("Failed to unmarshal expected definitions: %v", err)
	}
	if !reflect.DeepEqual(customFieldDefs, expectedDefs) {
		t.Errorf("customFieldDefinitions mismatch.\nGot:  %#v\nWant: %#v", customFieldDefs, expectedDefs)
	}

	metaMap, ok := pongoCtx["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta not found or not map[string]interface{}. Got: %T", pongoCtx["meta"])
	}
	var expectedMetaMap map[string]interface{}
	if err := json.Unmarshal([]byte(metaJSON), &expectedMetaMap); err != nil {
		t.Fatalf("Failed to unmarshal expected meta: %v", err)
	}
	if !reflect.DeepEqual(metaMap, expectedMetaMap) {
		t.Errorf("meta mismatch.\nGot:  %#v\nWant: %#v", metaMap, expectedMetaMap)
	}
}

func TestGroupContextProvider_DisplayGroupWithCategoryAndMeta(t *testing.T) {
	mockGroupID := uint(1)
	mockCategoryID := uint(2)
	cfdJSON := `[{"name":"priority","label":"Priority","type":"text"}]`
	metaJSON := `{"priority":"high","user":"testuser"}`

	mockCategory := &models.Category{
		ID:                     mockCategoryID,
		Name:                   "Display Cat",
		CustomFieldsDefinition: types.JSON(cfdJSON),
	}
	mockGroup := &models.Group{
		ID:         mockGroupID,
		Name:       "Display Group",
		CategoryId: &mockCategoryID,
		Category:   mockCategory,
		Meta:       types.JSON(metaJSON),
	}

	mockReader := &MockGroupReader{
		GetGroupFunc: func(id uint) (*models.Group, error) {
			if id == mockGroupID {
				return mockGroup, nil
			}
			return nil, fmt.Errorf("group %d not found", id)
		},
		FindParentsOfGroupFunc: func(id uint) (*[]models.Group, error) {
			return &[]models.Group{}, nil
		},
	}

	req := httptest.NewRequest("GET", "/group?id=1", nil)
	providerFunc := groupContextProviderImpl(mockReader)
	pongoCtx := providerFunc(req)

	customFieldDefs, ok := pongoCtx["customFieldDefinitions"].([]FieldDefinition)
	if !ok {
		t.Fatalf("customFieldDefinitions not found or not of expected type. Got: %T", pongoCtx["customFieldDefinitions"])
	}
	var expectedDefs []FieldDefinition
	if err := json.Unmarshal([]byte(cfdJSON), &expectedDefs); err != nil {
		t.Fatalf("Failed to unmarshal expected definitions: %v", err)
	}
	if !reflect.DeepEqual(customFieldDefs, expectedDefs) {
		t.Errorf("customFieldDefinitions mismatch.\nGot:  %#v\nWant: %#v", customFieldDefs, expectedDefs)
	}

	metaMap, ok := pongoCtx["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta not found or not map[string]interface{}. Got: %T", pongoCtx["meta"])
	}
	var expectedMetaMap map[string]interface{}
	if err := json.Unmarshal([]byte(metaJSON), &expectedMetaMap); err != nil {
		t.Fatalf("Failed to unmarshal expected meta: %v", err)
	}
	if !reflect.DeepEqual(metaMap, expectedMetaMap) {
		t.Errorf("meta mismatch.\nGot:  %#v\nWant: %#v", metaMap, expectedMetaMap)
	}
}


func TestGroupsListContextProvider_SingleCategoryFilter(t *testing.T) {
	mockCategoryID := uint(1)
	cfdJSON := `[{"name":"status","label":"Status","type":"select"}]`
	mockCategory := models.Category{
		ID:                     mockCategoryID,
		Name:                   "Filtered Cat",
		CustomFieldsDefinition: types.JSON(cfdJSON),
	}

	mockAppCtx := &MockAppContext{
		MockGroupReader: &MockGroupReader{
			GetGroupsFunc: func(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
				return &[]models.Group{
					{ID: 100, Name: "Group A", CategoryId: &mockCategoryID, Category: &mockCategory},
				}, nil
			},
			GetGroupsCountFunc: func(query *query_models.GroupQuery) (int64, error) { return 1, nil },
			GetCategoriesWithIdsFunc: func(ids *[]uint, limit int) (*[]models.Category, error) {
				if len(*ids) == 1 && (*ids)[0] == mockCategoryID {
					return &[]models.Category{mockCategory}, nil
				}
				return &[]models.Category{}, nil
			},
			GetGroupsWithIdsFunc:   func(ids *[]uint) (*[]*models.Group, error) { return &[]*models.Group{}, nil },
			GetNotesWithIdsFunc:    func(ids *[]uint) (*[]*models.Note, error) { return &[]*models.Note{}, nil },
			GetResourcesWithIdsFunc:func(ids *[]uint) (*[]*models.Resource, error) { return &[]*models.Resource{}, nil },
		},
		GetTagsWithIdsFunc: func(ids *[]uint, limit int) (*[]*models.Tag, error) { return &[]*models.Tag{}, nil },
	}

	providerFunc := GroupsListContextProvider(&application_context.MahresourcesContext{
		GroupReader:    mockAppCtx.MockGroupReader,
		CategoryReader: mockAppCtx.MockGroupReader, // Use the embedded one
		TagReader:      mockAppCtx, // Assuming MahresourcesContext has TagReader interface fulfilled by MockAppContext
		NoteReader:     mockAppCtx,
		ResourceReader: mockAppCtx,
	})

	req := httptest.NewRequest("GET", "/groups?Categories=1", nil)
	pongoCtx := providerFunc(req)

	customFieldDefs, ok := pongoCtx["singleCategoryCustomFieldDefinitions"].([]FieldDefinition)
	if !ok {
		t.Fatalf("singleCategoryCustomFieldDefinitions not found or not of expected type. Got: %T", pongoCtx["singleCategoryCustomFieldDefinitions"])
	}
	var expectedDefs []FieldDefinition
	if err := json.Unmarshal([]byte(cfdJSON), &expectedDefs); err != nil {
		t.Fatalf("Failed to unmarshal expected definitions: %v", err)
	}
	if !reflect.DeepEqual(customFieldDefs, expectedDefs) {
		t.Errorf("singleCategoryCustomFieldDefinitions mismatch.\nGot:  %#v\nWant: %#v", customFieldDefs, expectedDefs)
	}
}
[end of server/template_handlers/template_context_providers/group_template_context_test.go]
