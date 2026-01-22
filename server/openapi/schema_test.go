package openapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

// Test types for schema generation
type SimpleStruct struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
	Active      bool   `json:"active"`
}

type NestedStruct struct {
	ID     uint         `json:"id"`
	Name   string       `json:"name"`
	Simple SimpleStruct `json:"simple"`
}

type StructWithTags struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Internal  string    `json:"-"`
}

type StructWithSlice struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Names       []string       `json:"names"`
	Items       []SimpleStruct `json:"items"`
}

type QueryStruct struct {
	Name   string   `json:"name"`
	Tags   []string `json:"tags"`
	Limit  int      `json:"limit"`
	Active bool     `json:"active"`
}

func TestGenerateSchema_BasicTypes(t *testing.T) {
	g := NewSchemaGenerator()

	tests := []struct {
		name         string
		input        interface{}
		expectedType string
	}{
		{"string", "", "string"},
		{"int", 0, "integer"},
		{"int64", int64(0), "integer"},
		{"float64", float64(0), "number"},
		{"bool", false, "boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := g.GenerateSchema(reflect.TypeOf(tt.input))
			if schema == nil || schema.Value == nil {
				t.Fatal("expected non-nil schema")
			}
			if schema.Value.Type.Slice()[0] != tt.expectedType {
				t.Errorf("expected type %s, got %v", tt.expectedType, schema.Value.Type)
			}
		})
	}
}

func TestGenerateSchema_TimeType(t *testing.T) {
	g := NewSchemaGenerator()

	schema := g.GenerateSchema(reflect.TypeOf(time.Time{}))
	if schema == nil || schema.Value == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema.Value.Type.Slice()[0] != "string" {
		t.Errorf("expected type string for time.Time, got %v", schema.Value.Type)
	}
	if schema.Value.Format != "date-time" {
		t.Errorf("expected format date-time, got %s", schema.Value.Format)
	}
}

func TestGenerateSchema_Struct(t *testing.T) {
	g := NewSchemaGenerator()

	schema := g.GenerateSchema(reflect.TypeOf(SimpleStruct{}))

	// Should create a reference to a component schema
	if schema.Ref == "" {
		t.Error("expected schema reference for named struct")
	}
	if schema.Ref != "#/components/schemas/SimpleStruct" {
		t.Errorf("expected ref #/components/schemas/SimpleStruct, got %s", schema.Ref)
	}

	// Check that the schema was added to components
	componentSchema, exists := g.Schemas["SimpleStruct"]
	if !exists {
		t.Fatal("expected SimpleStruct schema in components")
	}

	// Verify properties
	props := componentSchema.Value.Properties
	if props["id"] == nil {
		t.Error("expected 'id' property")
	}
	if props["name"] == nil {
		t.Error("expected 'name' property")
	}
	if props["description"] == nil {
		t.Error("expected 'description' property")
	}
	if props["count"] == nil {
		t.Error("expected 'count' property")
	}
	if props["active"] == nil {
		t.Error("expected 'active' property")
	}
}

func TestGenerateSchema_ReadOnlyFields(t *testing.T) {
	g := NewSchemaGenerator()

	g.GenerateSchema(reflect.TypeOf(StructWithTags{}))

	componentSchema, exists := g.Schemas["StructWithTags"]
	if !exists {
		t.Fatal("expected StructWithTags schema in components")
	}

	props := componentSchema.Value.Properties

	// ID with gorm:"primarykey" should be readOnly
	if props["id"].Value == nil || !props["id"].Value.ReadOnly {
		t.Error("expected 'id' to be readOnly (primarykey)")
	}

	// CreatedAt should be readOnly
	if props["createdAt"].Value == nil || !props["createdAt"].Value.ReadOnly {
		t.Error("expected 'createdAt' to be readOnly")
	}

	// UpdatedAt should be readOnly
	if props["updatedAt"].Value == nil || !props["updatedAt"].Value.ReadOnly {
		t.Error("expected 'updatedAt' to be readOnly")
	}

	// Name should not be readOnly
	if props["name"].Value != nil && props["name"].Value.ReadOnly {
		t.Error("expected 'name' to not be readOnly")
	}

	// Internal field with json:"-" should not be present
	if props["Internal"] != nil {
		t.Error("expected 'Internal' field (json:\"-\") to be excluded")
	}
}

func TestGenerateSchema_Slice(t *testing.T) {
	g := NewSchemaGenerator()

	schema := g.GenerateSchema(reflect.TypeOf([]string{}))
	if schema == nil || schema.Value == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema.Value.Type.Slice()[0] != "array" {
		t.Errorf("expected type array, got %v", schema.Value.Type)
	}
	if schema.Value.Items == nil || schema.Value.Items.Value == nil {
		t.Fatal("expected items schema")
	}
	if schema.Value.Items.Value.Type.Slice()[0] != "string" {
		t.Errorf("expected items type string, got %v", schema.Value.Items.Value.Type)
	}
}

func TestGenerateSchema_StructWithSlice(t *testing.T) {
	g := NewSchemaGenerator()

	g.GenerateSchema(reflect.TypeOf(StructWithSlice{}))

	componentSchema, exists := g.Schemas["StructWithSlice"]
	if !exists {
		t.Fatal("expected StructWithSlice schema in components")
	}

	props := componentSchema.Value.Properties

	// Check names array
	if props["names"] == nil || props["names"].Value == nil {
		t.Fatal("expected 'names' property")
	}
	if props["names"].Value.Type.Slice()[0] != "array" {
		t.Errorf("expected 'names' to be array, got %v", props["names"].Value.Type)
	}

	// Check items array (should reference partial schema)
	if props["items"] == nil {
		t.Fatal("expected 'items' property")
	}
}

func TestGenerateQueryParams(t *testing.T) {
	g := NewSchemaGenerator()

	params := g.GenerateQueryParams(reflect.TypeOf(QueryStruct{}))

	if len(params) != 4 {
		t.Errorf("expected 4 parameters, got %d", len(params))
	}

	// Build a map of parameter names to their values
	paramNames := make(map[string]bool)
	var tagsParam *openapi3.Parameter
	for _, p := range params {
		paramNames[p.Value.Name] = true
		if p.Value.Name == "tags" {
			tagsParam = p.Value
		}
	}

	// Check name parameter
	if !paramNames["name"] {
		t.Error("expected 'name' parameter")
	}

	// Check tags parameter (array)
	if !paramNames["tags"] {
		t.Error("expected 'tags' parameter")
	} else if tagsParam != nil {
		if tagsParam.Style != "form" {
			t.Errorf("expected 'tags' parameter to have style 'form', got %s", tagsParam.Style)
		}
	}

	// Check limit parameter
	if !paramNames["limit"] {
		t.Error("expected 'limit' parameter")
	}

	// Check active parameter
	if !paramNames["active"] {
		t.Error("expected 'active' parameter")
	}
}

func TestGeneratePartialSchema(t *testing.T) {
	g := NewSchemaGenerator()

	// Directly test partial schema generation
	// This tests the configurable partial fields feature
	g.generatePartialSchema(reflect.TypeOf(StructWithSlice{}), "StructWithSlicePartial")

	partialSchema, exists := g.Schemas["StructWithSlicePartial"]
	if !exists {
		t.Fatal("expected StructWithSlicePartial schema in components")
	}

	props := partialSchema.Value.Properties

	// Partial should only include default fields: ID, Name (minimal for avoiding deep nesting)
	if props["id"] == nil {
		t.Error("expected 'id' in partial schema")
	}
	if props["name"] == nil {
		t.Error("expected 'name' in partial schema")
	}

	// Partial should NOT include other fields (Description, Names, Items)
	if props["description"] != nil {
		t.Error("expected 'description' to be excluded from partial schema (minimal defaults)")
	}
	if props["names"] != nil {
		t.Error("expected 'names' to be excluded from partial schema")
	}
	if props["items"] != nil {
		t.Error("expected 'items' to be excluded from partial schema")
	}
}

func TestGeneratePartialSchema_CustomFields(t *testing.T) {
	g := NewSchemaGenerator()

	// Configure custom partial fields for StructWithSlice
	g.PartialFields["StructWithSlice"] = []string{"ID", "Name", "Names"}

	// Directly test partial schema generation with custom config
	g.generatePartialSchema(reflect.TypeOf(StructWithSlice{}), "StructWithSlicePartial")

	partialSchema, exists := g.Schemas["StructWithSlicePartial"]
	if !exists {
		t.Fatal("expected StructWithSlicePartial schema in components")
	}

	props := partialSchema.Value.Properties

	// Should include custom fields
	if props["id"] == nil {
		t.Error("expected 'id' in partial schema")
	}
	if props["name"] == nil {
		t.Error("expected 'name' in partial schema")
	}
	if props["names"] == nil {
		t.Error("expected 'names' in partial schema (custom config)")
	}

	// Should NOT include fields not in custom config
	if props["description"] != nil {
		t.Error("expected 'description' to be excluded from partial schema (not in custom config)")
	}
	if props["items"] != nil {
		t.Error("expected 'items' to be excluded from partial schema (not in custom config)")
	}
}

func TestPointerTypes(t *testing.T) {
	g := NewSchemaGenerator()

	// Test pointer to string
	var strPtr *string
	schema := g.GenerateSchema(reflect.TypeOf(strPtr))
	if schema == nil || schema.Value == nil {
		t.Fatal("expected non-nil schema for *string")
	}
	if schema.Value.Type.Slice()[0] != "string" {
		t.Errorf("expected type string for *string, got %v", schema.Value.Type)
	}
	if !schema.Value.Nullable {
		t.Error("expected nullable for pointer type")
	}
}
