package openapi

import (
	"reflect"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

// SchemaGenerator converts Go types to OpenAPI schemas.
type SchemaGenerator struct {
	// Schemas stores all generated schemas by name
	Schemas map[string]*openapi3.SchemaRef

	// Keep track of types being processed to handle circular refs
	processing map[reflect.Type]bool

	// Map of type to schema name for partial schemas
	partialSchemas map[string]string

	// PartialFields maps type names to the fields that should be included in partial schemas.
	// If a type is not in this map, the default fields (ID, Name, Description) are used.
	PartialFields map[string][]string

	// DefaultPartialFields are included in partial schemas when no specific config exists.
	DefaultPartialFields []string
}

// NewSchemaGenerator creates a new schema generator.
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		Schemas:              make(map[string]*openapi3.SchemaRef),
		processing:           make(map[reflect.Type]bool),
		partialSchemas:       make(map[string]string),
		DefaultPartialFields: []string{"ID", "Name"},
		PartialFields: map[string][]string{
			// Group partial includes Category to show the group's category
			"Group": {"ID", "Name", "Category"},
			// Category partial is minimal
			"Category": {"ID", "Name"},
			// Note partial is minimal
			"Note": {"ID", "Name"},
			// Resource partial is minimal
			"Resource": {"ID", "Name"},
			// Tag doesn't need special handling, uses defaults
			// GroupRelationType partial is minimal
			"GroupRelationType": {"ID", "Name"},
		},
	}
}

// GenerateSchema generates an OpenAPI schema for a Go type.
// It returns a schema reference and adds any component schemas to the generator.
func (g *SchemaGenerator) GenerateSchema(t reflect.Type) *openapi3.SchemaRef {
	return g.generateSchemaInternal(t, false, 0)
}

// GenerateQueryParams generates OpenAPI parameters from a query struct type.
func (g *SchemaGenerator) GenerateQueryParams(t reflect.Type) openapi3.Parameters {
	if t == nil {
		return nil
	}

	// Dereference pointer types
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	var params openapi3.Parameters

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs
		if field.Anonymous {
			embeddedParams := g.GenerateQueryParams(field.Type)
			params = append(params, embeddedParams...)
			continue
		}

		name := getFieldName(field)
		schema := g.generateFieldSchema(field.Type)

		param := &openapi3.Parameter{
			Name:   name,
			In:     "query",
			Schema: schema,
		}

		// Arrays in query params need special handling
		if field.Type.Kind() == reflect.Slice {
			param.Style = "form"
			param.Explode = openapi3.Ptr(true)
		}

		params = append(params, &openapi3.ParameterRef{Value: param})
	}

	return params
}

func (g *SchemaGenerator) generateSchemaInternal(t reflect.Type, asPartial bool, depth int) *openapi3.SchemaRef {
	if t == nil {
		return nil
	}

	// Prevent infinite recursion
	if depth > 10 {
		return &openapi3.SchemaRef{Value: openapi3.NewObjectSchema()}
	}

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		inner := g.generateSchemaInternal(t.Elem(), asPartial, depth)
		if inner != nil && inner.Value != nil {
			inner.Value.Nullable = true
		}
		return inner
	}

	// Handle custom types from models/types package BEFORE basic types
	// This must come before slice check because types.JSON has underlying type []byte
	typePath := t.PkgPath()
	if strings.HasSuffix(typePath, "models/types") {
		switch t.Name() {
		case "JSON":
			schema := openapi3.NewObjectSchema()
			schema.AdditionalProperties = openapi3.AdditionalProperties{Has: boolPtr(true)}
			schema.Description = "Arbitrary JSON data"
			return openapi3.NewSchemaRef("", schema)
		case "URL":
			schema := openapi3.NewStringSchema()
			schema.Format = "uri"
			return openapi3.NewSchemaRef("", schema)
		}
	}

	// Handle time.Time before basic types check
	if t == reflect.TypeOf(time.Time{}) {
		schema := openapi3.NewStringSchema()
		schema.Format = "date-time"
		return openapi3.NewSchemaRef("", schema)
	}

	// Handle basic types
	switch t.Kind() {
	case reflect.String:
		return openapi3.NewSchemaRef("", openapi3.NewStringSchema())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())
	case reflect.Float32, reflect.Float64:
		return openapi3.NewSchemaRef("", openapi3.NewFloat64Schema())
	case reflect.Bool:
		return openapi3.NewSchemaRef("", openapi3.NewBoolSchema())
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte - binary data
			schema := openapi3.NewStringSchema()
			schema.Format = "binary"
			return openapi3.NewSchemaRef("", schema)
		}
		itemSchema := g.generateSchemaInternal(t.Elem(), true, depth+1)
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = itemSchema
		return openapi3.NewSchemaRef("", arraySchema)
	case reflect.Map:
		schema := openapi3.NewObjectSchema()
		schema.AdditionalProperties = openapi3.AdditionalProperties{
			Has:    boolPtr(true),
			Schema: g.generateSchemaInternal(t.Elem(), false, depth+1),
		}
		return openapi3.NewSchemaRef("", schema)
	}

	// Handle struct types
	if t.Kind() == reflect.Struct {
		return g.generateStructSchema(t, asPartial, depth)
	}

	// Default to object
	return openapi3.NewSchemaRef("", openapi3.NewObjectSchema())
}

func (g *SchemaGenerator) generateStructSchema(t reflect.Type, asPartial bool, depth int) *openapi3.SchemaRef {
	schemaName := t.Name()
	if schemaName == "" {
		// Anonymous struct - generate inline
		return g.generateInlineStructSchema(t, depth)
	}

	// For partial schemas (used in arrays to avoid deep nesting and circular refs)
	// depth > 0 means this is a nested type (e.g., array item or nested struct field)
	if asPartial && depth > 0 {
		partialName := schemaName + "Partial"
		if _, exists := g.Schemas[partialName]; !exists {
			g.generatePartialSchema(t, partialName)
		}
		return openapi3.NewSchemaRef("#/components/schemas/"+partialName, nil)
	}

	// Check if we're already processing this type (circular reference)
	if g.processing[t] {
		return openapi3.NewSchemaRef("#/components/schemas/"+schemaName, nil)
	}

	// Check if schema already exists
	if _, exists := g.Schemas[schemaName]; exists {
		return openapi3.NewSchemaRef("#/components/schemas/"+schemaName, nil)
	}

	// Mark as processing
	g.processing[t] = true
	defer delete(g.processing, t)

	// Generate the schema
	schema := g.generateInlineStructSchema(t, depth)

	// Store in components
	g.Schemas[schemaName] = schema

	return openapi3.NewSchemaRef("#/components/schemas/"+schemaName, nil)
}

func (g *SchemaGenerator) generateInlineStructSchema(t reflect.Type, depth int) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()

	// Collect all fields including from embedded structs
	g.collectStructFields(t, schema, depth)

	return openapi3.NewSchemaRef("", schema)
}

// collectStructFields recursively collects fields from a struct type,
// including fields from embedded (anonymous) structs.
func (g *SchemaGenerator) collectStructFields(t reflect.Type, schema *openapi3.Schema, depth int) {
	// Handle pointer types
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs - recursively collect their fields
		if field.Anonymous {
			g.collectStructFields(field.Type, schema, depth+1)
			continue
		}

		// Skip fields with json:"-" tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := getFieldName(field)
		fieldSchema := g.generateFieldSchema(field.Type)

		// Check for readOnly in gorm tags or timestamp fields
		// Use field.Name (struct field name) not JSON name for timestamp detection
		gormTag := field.Tag.Get("gorm")
		if strings.Contains(gormTag, "primarykey") ||
			field.Name == "CreatedAt" ||
			field.Name == "UpdatedAt" ||
			field.Name == "DeletedAt" {
			if fieldSchema.Value != nil {
				fieldSchema.Value.ReadOnly = true
			}
		}

		schema.Properties[name] = fieldSchema
	}
}

func (g *SchemaGenerator) generatePartialSchema(t reflect.Type, partialName string) {
	schema := openapi3.NewObjectSchema()

	// Determine which fields to include in this partial schema
	typeName := t.Name()
	fieldsToInclude := g.DefaultPartialFields
	if customFields, ok := g.PartialFields[typeName]; ok {
		fieldsToInclude = customFields
	}

	// Build a set for quick lookup
	includeSet := make(map[string]bool)
	for _, f := range fieldsToInclude {
		includeSet[f] = true
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if !field.IsExported() {
			continue
		}

		// Use struct field name for config lookup, JSON name for schema property
		structFieldName := field.Name
		jsonName := getFieldName(field)

		// Only include configured fields in partial schemas
		if !includeSet[structFieldName] {
			continue
		}

		// For struct fields, try to use their partial schema if it exists or will exist
		fieldType := field.Type
		for fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct && fieldType.Name() != "" && fieldType.Name() != "Time" {
			// Reference the partial schema for this nested type
			nestedPartialName := fieldType.Name() + "Partial"
			// Ensure the partial schema exists
			if _, exists := g.Schemas[nestedPartialName]; !exists {
				g.generatePartialSchema(fieldType, nestedPartialName)
			}
			schema.Properties[jsonName] = openapi3.NewSchemaRef("#/components/schemas/"+nestedPartialName, nil)
		} else {
			fieldSchema := g.generateFieldSchema(field.Type)
			schema.Properties[jsonName] = fieldSchema
		}
	}

	g.Schemas[partialName] = openapi3.NewSchemaRef("", schema)
}

func (g *SchemaGenerator) generateFieldSchema(t reflect.Type) *openapi3.SchemaRef {
	return g.generateSchemaInternal(t, true, 0)
}

// getFieldName extracts the JSON field name from struct tags or falls back to field name.
func getFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get("json")
	if jsonTag != "" && jsonTag != "-" {
		parts := strings.Split(jsonTag, ",")
		if parts[0] != "" {
			return parts[0]
		}
	}
	return field.Name
}

func boolPtr(b bool) *bool {
	return &b
}
