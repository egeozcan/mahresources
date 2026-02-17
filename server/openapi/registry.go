package openapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// Registry stores route information and generates OpenAPI specs.
type Registry struct {
	routes    []RouteInfo
	generator *SchemaGenerator

	// API metadata
	Title       string
	Version     string
	Description string
	ServerURL   string
}

// NewRegistry creates a new OpenAPI registry.
func NewRegistry() *Registry {
	return &Registry{
		routes:    make([]RouteInfo, 0),
		generator: NewSchemaGenerator(),
		Title:     "mahresources",
		Version:   "1.0",
		ServerURL: "/v1",
	}
}

// Register adds a route to the registry.
func (r *Registry) Register(info RouteInfo) {
	r.routes = append(r.routes, info)

	// Pre-generate schemas for types
	if info.QueryType != nil {
		r.generator.GenerateSchema(info.QueryType)
	}
	if info.RequestType != nil {
		r.generator.GenerateSchema(info.RequestType)
	}
	if info.ResponseType != nil {
		r.generator.GenerateSchema(info.ResponseType)
	}
}

// GenerateSpec generates the complete OpenAPI 3.0 specification.
func (r *Registry) GenerateSpec() *openapi3.T {
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   r.Title,
			Version: r.Version,
		},
		Servers: openapi3.Servers{
			&openapi3.Server{URL: r.ServerURL, Description: "API Version 1"},
		},
		Paths: &openapi3.Paths{
			Extensions: make(map[string]interface{}),
		},
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}

	// Collect unique tags
	tagSet := make(map[string]bool)
	for _, route := range r.routes {
		for _, tag := range route.Tags {
			tagSet[tag] = true
		}
	}

	// Add tags with descriptions
	tagDescriptions := map[string]string{
		"notes":      "Operations related to notes",
		"groups":     "Operations related to groups",
		"resources":  "Operations related to resources",
		"tags":       "Operations related to tags",
		"categories": "Operations related to categories",
		"queries":    "Operations related to queries",
		"relations":  "Operations related to relations",
		"search":     "Search operations",
	}

	var tags []string
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	for _, tag := range tags {
		desc := tagDescriptions[tag]
		if desc == "" {
			desc = "Operations related to " + tag
		}
		spec.Tags = append(spec.Tags, &openapi3.Tag{Name: tag, Description: desc})
	}

	// Add component schemas
	for name, schema := range r.generator.Schemas {
		spec.Components.Schemas[name] = schema
	}

	// Group routes by path
	pathOps := make(map[string]map[string]*openapi3.Operation)
	for _, route := range r.routes {
		if _, exists := pathOps[route.Path]; !exists {
			pathOps[route.Path] = make(map[string]*openapi3.Operation)
		}
		pathOps[route.Path][route.Method] = r.generateOperation(route)
	}

	// Add paths
	for path, ops := range pathOps {
		pathItem := &openapi3.PathItem{}

		for method, op := range ops {
			switch method {
			case http.MethodGet:
				pathItem.Get = op
			case http.MethodPost:
				pathItem.Post = op
			case http.MethodPut:
				pathItem.Put = op
			case http.MethodDelete:
				pathItem.Delete = op
			case http.MethodPatch:
				pathItem.Patch = op
			}
		}

		spec.Paths.Set(path, pathItem)
	}

	return spec
}

func (r *Registry) generateOperation(route RouteInfo) *openapi3.Operation {
	op := &openapi3.Operation{
		OperationID: route.OperationID,
		Summary:     route.Summary,
		Description: route.Description,
		Tags:        route.Tags,
		Responses:   &openapi3.Responses{},
	}

	// Add query parameters
	if route.QueryType != nil {
		params := r.generator.GenerateQueryParams(route.QueryType)
		op.Parameters = append(op.Parameters, params...)
	}

	// Add ID query parameter if specified
	if route.IDQueryParam != "" {
		idParam := &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:     route.IDQueryParam,
				In:       "query",
				Required: route.IDRequired,
				Schema:   openapi3.NewSchemaRef("", openapi3.NewIntegerSchema()),
			},
		}
		op.Parameters = append(op.Parameters, idParam)
	}

	// Add extra query parameters
	for _, param := range route.ExtraQueryParams {
		p := &openapi3.Parameter{
			Name:        param.Name,
			In:          "query",
			Required:    param.Required,
			Description: param.Description,
		}

		switch param.Type {
		case "string":
			p.Schema = openapi3.NewSchemaRef("", openapi3.NewStringSchema())
		case "integer":
			p.Schema = openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())
		case "boolean":
			p.Schema = openapi3.NewSchemaRef("", openapi3.NewBoolSchema())
		case "array":
			arrSchema := openapi3.NewArraySchema()
			switch param.ItemType {
			case "integer":
				arrSchema.Items = openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())
			default:
				arrSchema.Items = openapi3.NewSchemaRef("", openapi3.NewStringSchema())
			}
			p.Schema = openapi3.NewSchemaRef("", arrSchema)
			p.Style = "form"
			p.Explode = openapi3.Ptr(true)
		}

		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: p})
	}

	// Add pagination parameter for paginated endpoints
	if route.Paginated {
		pageParam := &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        "page",
				In:          "query",
				Description: "Page number for pagination",
				Schema:      openapi3.NewSchemaRef("", openapi3.NewIntegerSchema().WithDefault(1)),
			},
		}
		op.Parameters = append(op.Parameters, pageParam)
	}

	// Add request body
	if route.RequestType != nil || route.HasFileUpload {
		op.RequestBody = r.generateRequestBody(route)
	}

	// Add responses
	op.Responses.Set("200", r.generateSuccessResponse(route))

	// Add error responses
	for code, desc := range route.ErrorResponses {
		op.Responses.Set(statusCodeToString(code), &openapi3.ResponseRef{
			Value: &openapi3.Response{Description: strPtr(desc)},
		})
	}

	return op
}

func (r *Registry) generateRequestBody(route RouteInfo) *openapi3.RequestBodyRef {
	content := openapi3.Content{}

	if route.HasFileUpload {
		// Multipart form with file
		schema := openapi3.NewObjectSchema()

		// Add file field
		fileSchema := openapi3.NewStringSchema()
		fileSchema.Format = "binary"
		if route.MultipleFiles {
			arrSchema := openapi3.NewArraySchema()
			arrSchema.Items = openapi3.NewSchemaRef("", fileSchema)
			schema.Properties[route.FileFieldName] = openapi3.NewSchemaRef("", arrSchema)
		} else {
			schema.Properties[route.FileFieldName] = openapi3.NewSchemaRef("", fileSchema)
		}

		// Add other fields from RequestType
		if route.RequestType != nil {
			reqSchema := r.generator.GenerateSchema(route.RequestType)
			if reqSchema != nil && reqSchema.Value != nil {
				for name, prop := range reqSchema.Value.Properties {
					schema.Properties[name] = prop
				}
			}
		}

		content[string(ContentTypeMultipart)] = &openapi3.MediaType{
			Schema: openapi3.NewSchemaRef("", schema),
		}
	} else if route.RequestType != nil {
		schema := r.generator.GenerateSchema(route.RequestType)

		for _, ct := range route.RequestContentTypes {
			content[string(ct)] = &openapi3.MediaType{Schema: schema}
		}
	}

	return &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Required: true,
			Content:  content,
		},
	}
}

func (r *Registry) generateSuccessResponse(route RouteInfo) *openapi3.ResponseRef {
	resp := &openapi3.Response{
		Description: strPtr("Successful response"),
	}

	if route.ResponseType != nil {
		schema := r.generator.GenerateSchema(route.ResponseType)
		resp.Content = openapi3.Content{
			string(ContentTypeJSON): &openapi3.MediaType{Schema: schema},
		}
	}

	return &openapi3.ResponseRef{Value: resp}
}

func strPtr(s string) *string {
	return &s
}

func statusCodeToString(code int) string {
	switch code {
	case 200:
		return "200"
	case 201:
		return "201"
	case 204:
		return "204"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 500:
		return "500"
	default:
		return "default"
	}
}

// MarshalYAML generates YAML output from the spec.
func (r *Registry) MarshalYAML() ([]byte, error) {
	spec := r.GenerateSpec()
	return yaml.Marshal(spec)
}

// MarshalJSON generates JSON output from the spec.
func (r *Registry) MarshalJSON() ([]byte, error) {
	spec := r.GenerateSpec()
	return json.MarshalIndent(spec, "", "  ")
}

// RegisterCRUDRoutes is a helper to register standard CRUD routes for an entity.
func (r *Registry) RegisterCRUDRoutes(
	entityName string,
	entityNamePlural string,
	tag string,
	entityType reflect.Type,
	queryType reflect.Type,
	creatorType reflect.Type,
) {
	// List endpoint
	r.Register(RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/" + entityNamePlural,
		OperationID:          "list" + capitalize(entityNamePlural),
		Summary:              "List " + entityNamePlural,
		Tags:                 []string{tag},
		QueryType:            queryType,
		ResponseType:         reflect.SliceOf(entityType),
		ResponseContentTypes: []ContentType{ContentTypeJSON},
		Paginated:            true,
	})

	// Get endpoint
	r.Register(RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/" + entityName,
		OperationID:          "get" + capitalize(entityName),
		Summary:              "Get a specific " + entityName,
		Tags:                 []string{tag},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         entityType,
		ResponseContentTypes: []ContentType{ContentTypeJSON},
	})

	// Create endpoint
	r.Register(RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/" + entityName,
		OperationID:          "createOrUpdate" + capitalize(entityName),
		Summary:              "Create or update a " + entityName,
		Tags:                 []string{tag},
		RequestType:          creatorType,
		RequestContentTypes:  []ContentType{ContentTypeJSON, ContentTypeForm},
		ResponseType:         entityType,
		ResponseContentTypes: []ContentType{ContentTypeJSON},
	})

	// Delete endpoint
	r.Register(RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/" + entityName + "/delete",
		OperationID:         "delete" + capitalize(entityName),
		Summary:             "Delete a " + entityName,
		Tags:                []string{tag},
		IDQueryParam:        "Id",
		IDRequired:          true,
		RequestContentTypes: []ContentType{ContentTypeJSON, ContentTypeForm},
	})

	// EditName endpoint
	r.Register(NewRoute(
		http.MethodPost,
		"/v1/"+entityName+"/editName",
		"edit"+capitalize(entityName)+"Name",
		"Edit a "+entityName+"'s name",
		tag,
	).WithIDParam("id", true))

	// EditDescription endpoint
	r.Register(NewRoute(
		http.MethodPost,
		"/v1/"+entityName+"/editDescription",
		"edit"+capitalize(entityName)+"Description",
		"Edit a "+entityName+"'s description",
		tag,
	).WithIDParam("id", true))
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
