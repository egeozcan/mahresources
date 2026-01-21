package openapi

import (
	"net/http"
	"reflect"
	"testing"
)

// Test types for registry tests
type TestEntity struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TestQuery struct {
	Name   string `json:"name"`
	Limit  int    `json:"limit"`
}

type TestCreator struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	if r.Title != "mahresources" {
		t.Errorf("expected title 'mahresources', got %s", r.Title)
	}
	if r.Version != "1.0" {
		t.Errorf("expected version '1.0', got %s", r.Version)
	}
	if r.ServerURL != "/v1" {
		t.Errorf("expected serverURL '/v1', got %s", r.ServerURL)
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()

	route := RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/test",
		OperationID: "getTest",
		Summary:     "Get test",
		Tags:        []string{"test"},
	}

	r.Register(route)

	if len(r.routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(r.routes))
	}
}

func TestGenerateSpec(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/tests",
		OperationID:  "listTests",
		Summary:      "List tests",
		Tags:         []string{"tests"},
		QueryType:    reflect.TypeOf(TestQuery{}),
		ResponseType: reflect.SliceOf(reflect.TypeOf(TestEntity{})),
		Paginated:    true,
	})

	r.Register(RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/test",
		OperationID:  "getTest",
		Summary:      "Get a test",
		Tags:         []string{"tests"},
		IDQueryParam: "id",
		IDRequired:   true,
		ResponseType: reflect.TypeOf(TestEntity{}),
	})

	r.Register(RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/test",
		OperationID:         "createTest",
		Summary:             "Create a test",
		Tags:                []string{"tests"},
		RequestType:         reflect.TypeOf(TestCreator{}),
		RequestContentTypes: []ContentType{ContentTypeJSON, ContentTypeForm},
		ResponseType:        reflect.TypeOf(TestEntity{}),
	})

	spec := r.GenerateSpec()

	// Check OpenAPI version
	if spec.OpenAPI != "3.0.0" {
		t.Errorf("expected OpenAPI version 3.0.0, got %s", spec.OpenAPI)
	}

	// Check info
	if spec.Info.Title != "mahresources" {
		t.Errorf("expected title 'mahresources', got %s", spec.Info.Title)
	}

	// Check paths
	if spec.Paths.Len() != 2 { // /v1/tests and /v1/test
		t.Errorf("expected 2 paths, got %d", spec.Paths.Len())
	}

	// Check tags
	if len(spec.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(spec.Tags))
	}
	if spec.Tags[0].Name != "tests" {
		t.Errorf("expected tag 'tests', got %s", spec.Tags[0].Name)
	}

	// Check schemas were generated
	if len(spec.Components.Schemas) == 0 {
		t.Error("expected some schemas in components")
	}
	if spec.Components.Schemas["TestEntity"] == nil {
		t.Error("expected TestEntity schema")
	}
}

func TestGenerateSpec_Pagination(t *testing.T) {
	r := NewRegistry()

	// Route with pagination
	r.Register(RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/items",
		OperationID: "listItems",
		Summary:     "List items",
		Tags:        []string{"items"},
		Paginated:   true,
	})

	// Route without pagination
	r.Register(RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/item",
		OperationID: "getItem",
		Summary:     "Get item",
		Tags:        []string{"items"},
	})

	spec := r.GenerateSpec()

	// Check paginated endpoint has page parameter
	listPath := spec.Paths.Find("/v1/items")
	if listPath == nil || listPath.Get == nil {
		t.Fatal("expected GET /v1/items")
	}

	hasPageParam := false
	for _, param := range listPath.Get.Parameters {
		if param.Value.Name == "page" {
			hasPageParam = true
			break
		}
	}
	if !hasPageParam {
		t.Error("expected 'page' parameter on paginated endpoint")
	}

	// Check non-paginated endpoint does NOT have page parameter
	getPath := spec.Paths.Find("/v1/item")
	if getPath == nil || getPath.Get == nil {
		t.Fatal("expected GET /v1/item")
	}

	for _, param := range getPath.Get.Parameters {
		if param.Value.Name == "page" {
			t.Error("expected no 'page' parameter on non-paginated endpoint")
			break
		}
	}
}

func TestGenerateSpec_FileUpload(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:        http.MethodPost,
		Path:          "/v1/upload",
		OperationID:   "uploadFile",
		Summary:       "Upload a file",
		Tags:          []string{"uploads"},
		HasFileUpload: true,
		FileFieldName: "file",
		MultipleFiles: false,
	})

	spec := r.GenerateSpec()

	uploadPath := spec.Paths.Find("/v1/upload")
	if uploadPath == nil || uploadPath.Post == nil {
		t.Fatal("expected POST /v1/upload")
	}

	if uploadPath.Post.RequestBody == nil || uploadPath.Post.RequestBody.Value == nil {
		t.Fatal("expected request body")
	}

	content := uploadPath.Post.RequestBody.Value.Content
	if content["multipart/form-data"] == nil {
		t.Error("expected multipart/form-data content type for file upload")
	}
}

func TestGenerateSpec_IDQueryParam(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/item",
		OperationID:  "getItem",
		Summary:      "Get item",
		Tags:         []string{"items"},
		IDQueryParam: "id",
		IDRequired:   true,
	})

	spec := r.GenerateSpec()

	itemPath := spec.Paths.Find("/v1/item")
	if itemPath == nil || itemPath.Get == nil {
		t.Fatal("expected GET /v1/item")
	}

	hasIDParam := false
	for _, param := range itemPath.Get.Parameters {
		if param.Value.Name == "id" {
			hasIDParam = true
			if !param.Value.Required {
				t.Error("expected 'id' parameter to be required")
			}
			break
		}
	}
	if !hasIDParam {
		t.Error("expected 'id' parameter")
	}
}

func TestGenerateSpec_ExtraQueryParams(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/search",
		OperationID: "search",
		Summary:     "Search",
		Tags:        []string{"search"},
		ExtraQueryParams: []QueryParam{
			{Name: "q", Type: "string", Required: true, Description: "Search query"},
			{Name: "limit", Type: "integer", Required: false},
			{Name: "tags", Type: "array", ItemType: "string"},
		},
	})

	spec := r.GenerateSpec()

	searchPath := spec.Paths.Find("/v1/search")
	if searchPath == nil || searchPath.Get == nil {
		t.Fatal("expected GET /v1/search")
	}

	paramMap := make(map[string]bool)
	for _, param := range searchPath.Get.Parameters {
		paramMap[param.Value.Name] = true
	}

	if !paramMap["q"] {
		t.Error("expected 'q' parameter")
	}
	if !paramMap["limit"] {
		t.Error("expected 'limit' parameter")
	}
	if !paramMap["tags"] {
		t.Error("expected 'tags' parameter")
	}
}

func TestMarshalYAML(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/test",
		OperationID: "getTest",
		Summary:     "Get test",
		Tags:        []string{"test"},
	})

	data, err := r.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty YAML output")
	}

	// Check it contains expected content
	yamlStr := string(data)
	if !contains(yamlStr, "openapi:") {
		t.Error("expected 'openapi:' in YAML output")
	}
	if !contains(yamlStr, "getTest") {
		t.Error("expected 'getTest' operation in YAML output")
	}
}

func TestMarshalJSON(t *testing.T) {
	r := NewRegistry()

	r.Register(RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/test",
		OperationID: "getTest",
		Summary:     "Get test",
		Tags:        []string{"test"},
	})

	data, err := r.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}

	// Check it contains expected content
	jsonStr := string(data)
	if !contains(jsonStr, "\"openapi\"") {
		t.Error("expected '\"openapi\"' in JSON output")
	}
	if !contains(jsonStr, "getTest") {
		t.Error("expected 'getTest' operation in JSON output")
	}
}

func TestNewRoute(t *testing.T) {
	route := NewRoute(http.MethodGet, "/v1/test", "getTest", "Get test", "test")

	if route.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", route.Method)
	}
	if route.Path != "/v1/test" {
		t.Errorf("expected path '/v1/test', got %s", route.Path)
	}
	if route.OperationID != "getTest" {
		t.Errorf("expected operationID 'getTest', got %s", route.OperationID)
	}
	if route.Summary != "Get test" {
		t.Errorf("expected summary 'Get test', got %s", route.Summary)
	}
	if len(route.Tags) != 1 || route.Tags[0] != "test" {
		t.Errorf("expected tags ['test'], got %v", route.Tags)
	}

	// Check defaults
	if len(route.RequestContentTypes) != 2 {
		t.Error("expected default request content types")
	}
	if len(route.ResponseContentTypes) != 1 {
		t.Error("expected default response content types")
	}
	if len(route.ErrorResponses) != 3 {
		t.Error("expected default error responses")
	}
}

func TestRouteBuilders(t *testing.T) {
	route := NewRoute(http.MethodPost, "/v1/test", "createTest", "Create test", "test").
		WithQuery(TestQuery{}).
		WithRequest(TestCreator{}).
		WithResponse(TestEntity{}).
		WithIDParam("id", true).
		WithDescription("Detailed description").
		WithPagination()

	if route.QueryType == nil {
		t.Error("expected QueryType to be set")
	}
	if route.RequestType == nil {
		t.Error("expected RequestType to be set")
	}
	if route.ResponseType == nil {
		t.Error("expected ResponseType to be set")
	}
	if route.IDQueryParam != "id" {
		t.Errorf("expected IDQueryParam 'id', got %s", route.IDQueryParam)
	}
	if !route.IDRequired {
		t.Error("expected IDRequired to be true")
	}
	if route.Description != "Detailed description" {
		t.Errorf("expected description, got %s", route.Description)
	}
	if !route.Paginated {
		t.Error("expected Paginated to be true")
	}
}

func TestRouteWithFileUpload(t *testing.T) {
	route := NewRoute(http.MethodPost, "/v1/upload", "uploadFile", "Upload file", "uploads").
		WithFileUpload("file", true)

	if !route.HasFileUpload {
		t.Error("expected HasFileUpload to be true")
	}
	if route.FileFieldName != "file" {
		t.Errorf("expected FileFieldName 'file', got %s", route.FileFieldName)
	}
	if !route.MultipleFiles {
		t.Error("expected MultipleFiles to be true")
	}
	if len(route.RequestContentTypes) != 1 || route.RequestContentTypes[0] != ContentTypeMultipart {
		t.Error("expected multipart content type for file upload")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
