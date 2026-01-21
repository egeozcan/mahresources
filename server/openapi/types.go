package openapi

import (
	"net/http"
	"reflect"
)

// ContentType represents supported content types for requests/responses.
type ContentType string

const (
	ContentTypeJSON      ContentType = "application/json"
	ContentTypeForm      ContentType = "application/x-www-form-urlencoded"
	ContentTypeMultipart ContentType = "multipart/form-data"
)

// RouteInfo contains all metadata needed to generate OpenAPI documentation for a route.
type RouteInfo struct {
	// HTTP method (GET, POST, etc.)
	Method string

	// URL path (e.g., "/v1/tags")
	Path string

	// Unique identifier for this operation (e.g., "listTags")
	OperationID string

	// Short summary of the operation
	Summary string

	// Longer description (optional)
	Description string

	// Tags for grouping operations
	Tags []string

	// Query parameter type - struct whose fields become query params (for GET requests)
	QueryType reflect.Type

	// Request body type - struct for request body (for POST/PUT requests)
	RequestType reflect.Type

	// Response type - struct or slice for success response body
	ResponseType reflect.Type

	// Supported content types for request body
	RequestContentTypes []ContentType

	// Supported content types for response
	ResponseContentTypes []ContentType

	// Whether this endpoint supports file uploads
	HasFileUpload bool

	// Name of the file field in multipart form (if HasFileUpload is true)
	FileFieldName string

	// Whether the file upload accepts multiple files
	MultipleFiles bool

	// Query parameter for entity ID (if route uses ?id=X pattern)
	IDQueryParam string

	// Whether ID is required in query params
	IDRequired bool

	// Additional query parameters not derived from QueryType
	ExtraQueryParams []QueryParam

	// HTTP status codes and their response types
	ErrorResponses map[int]string

	// Whether this endpoint supports pagination (adds page parameter)
	Paginated bool
}

// QueryParam represents a single query parameter.
type QueryParam struct {
	Name        string
	Description string
	Type        string // "string", "integer", "boolean", "array"
	ItemType    string // For arrays, the type of items
	Required    bool
	Default     interface{}
}

// NewRoute creates a RouteInfo with sensible defaults.
func NewRoute(method, path, operationID, summary string, tags ...string) RouteInfo {
	return RouteInfo{
		Method:               method,
		Path:                 path,
		OperationID:          operationID,
		Summary:              summary,
		Tags:                 tags,
		RequestContentTypes:  []ContentType{ContentTypeJSON, ContentTypeForm},
		ResponseContentTypes: []ContentType{ContentTypeJSON},
		ErrorResponses: map[int]string{
			http.StatusBadRequest:          "Invalid input",
			http.StatusNotFound:            "Not found",
			http.StatusInternalServerError: "Internal server error",
		},
	}
}

// WithQuery sets the query parameter type.
func (r RouteInfo) WithQuery(queryType interface{}) RouteInfo {
	r.QueryType = reflect.TypeOf(queryType)
	return r
}

// WithRequest sets the request body type.
func (r RouteInfo) WithRequest(requestType interface{}) RouteInfo {
	r.RequestType = reflect.TypeOf(requestType)
	return r
}

// WithResponse sets the response type.
func (r RouteInfo) WithResponse(responseType interface{}) RouteInfo {
	r.ResponseType = reflect.TypeOf(responseType)
	return r
}

// WithFileUpload marks this route as supporting file uploads.
func (r RouteInfo) WithFileUpload(fieldName string, multiple bool) RouteInfo {
	r.HasFileUpload = true
	r.FileFieldName = fieldName
	r.MultipleFiles = multiple
	r.RequestContentTypes = []ContentType{ContentTypeMultipart}
	return r
}

// WithIDParam adds an ID query parameter.
func (r RouteInfo) WithIDParam(paramName string, required bool) RouteInfo {
	r.IDQueryParam = paramName
	r.IDRequired = required
	return r
}

// WithDescription adds a longer description.
func (r RouteInfo) WithDescription(description string) RouteInfo {
	r.Description = description
	return r
}

// WithPagination marks this route as supporting pagination.
func (r RouteInfo) WithPagination() RouteInfo {
	r.Paginated = true
	return r
}
