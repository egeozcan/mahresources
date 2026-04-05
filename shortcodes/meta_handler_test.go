package shortcodes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMetaBasic(t *testing.T) {
	meta := map[string]any{"cooking": map[string]any{"time": 30}}
	metaJSON, _ := json.Marshal(meta)
	schema := `{"type":"object","properties":{"cooking":{"type":"object","properties":{"time":{"type":"integer","title":"Cooking Time"}}}}}`

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   42,
		Meta:       metaJSON,
		MetaSchema: schema,
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "cooking.time"},
	}, ctx)

	assert.Contains(t, result, `data-path="cooking.time"`)
	assert.Contains(t, result, `data-entity-type="group"`)
	assert.Contains(t, result, `data-entity-id="42"`)
	assert.Contains(t, result, `data-value="30"`)
	assert.Contains(t, result, `data-editable="false"`)
	assert.Contains(t, result, `data-hide-empty="false"`)
	// Schema slice is HTML-escaped in the attribute
	assert.Contains(t, result, `data-schema="`)
	assert.Contains(t, result, `integer`)
	assert.Contains(t, result, `Cooking Time`)
}

func TestRenderMetaEditable(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   7,
		Meta:       metaJSON,
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "name", "editable": "true"},
	}, ctx)

	assert.Contains(t, result, `data-editable="true"`)
	assert.Contains(t, result, `data-value="&#34;test&#34;"`)
}

func TestRenderMetaMissingPath(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{},
	}, ctx)

	assert.Equal(t, "", result)
}

func TestRenderMetaEmptyValue(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "nonexistent"},
	}, ctx)

	assert.Contains(t, result, `data-path="nonexistent"`)
	assert.Contains(t, result, `data-value=""`)
}

func TestRenderMetaHideEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "a", "hide-empty": "true"},
	}, ctx)

	assert.Contains(t, result, `data-hide-empty="true"`)
}

func TestRenderMetaObjectValue(t *testing.T) {
	meta := map[string]any{"loc": map[string]any{"lat": 1.5, "lng": 2.5}}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "loc"},
	}, ctx)

	assert.Contains(t, result, `data-path="loc"`)
	// Value is HTML-escaped JSON: {"lat":1.5,"lng":2.5} → {&#34;lat&#34;:1.5,&#34;lng&#34;:2.5}
	assert.Contains(t, result, `lat`)
}

func TestExtractSchemaSlice(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"string","title":"B Field"}}}}}`
	slice := extractSchemaSlice(schema, "a.b", nil)
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "B Field", parsed["title"])
}

func TestExtractSchemaSliceWithRef(t *testing.T) {
	schema := `{"type":"object","properties":{"home":{"$ref":"#/$defs/Address"}},"$defs":{"Address":{"type":"object","properties":{"zip":{"type":"string","title":"ZIP Code"}}}}}`
	slice := extractSchemaSlice(schema, "home.zip", nil)
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "ZIP Code", parsed["title"])
}

func TestExtractSchemaSliceWithRefAtLeaf(t *testing.T) {
	schema := `{"type":"object","properties":{"addr":{"$ref":"#/$defs/Addr"}},"$defs":{"Addr":{"type":"object","title":"Address","properties":{"city":{"type":"string"}}}}}`
	slice := extractSchemaSlice(schema, "addr", nil)
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "object", parsed["type"])
	assert.Equal(t, "Address", parsed["title"])
}

func TestExtractSchemaSliceWithAllOf(t *testing.T) {
	schema := `{"type":"object","properties":{"item":{"allOf":[{"type":"object","properties":{"name":{"type":"string","title":"Name"}}},{"properties":{"price":{"type":"number","title":"Price"}}}]}}}`
	// item.name should resolve through the allOf merge
	slice := extractSchemaSlice(schema, "item.name", nil)
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "string", parsed["type"])

	// item.price from second allOf branch
	slice2 := extractSchemaSlice(schema, "item.price", nil)
	require.NotEmpty(t, slice2)
	var parsed2 map[string]any
	err = json.Unmarshal([]byte(slice2), &parsed2)
	require.NoError(t, err)
	assert.Equal(t, "number", parsed2["type"])
}

func TestExtractSchemaSliceNestedRefThenAllOf(t *testing.T) {
	// $ref target itself uses allOf
	schema := `{
		"type":"object",
		"properties":{"item":{"$ref":"#/$defs/Item"}},
		"$defs":{
			"Item":{
				"allOf":[
					{"type":"object","properties":{"name":{"type":"string","title":"Name"}}},
					{"properties":{"price":{"type":"number","title":"Price"}}}
				]
			}
		}
	}`
	slice := extractSchemaSlice(schema, "item.name", nil)
	require.NotEmpty(t, slice)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "Name", parsed["title"])

	slice2 := extractSchemaSlice(schema, "item.price", nil)
	require.NotEmpty(t, slice2)
	var parsed2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice2), &parsed2))
	assert.Equal(t, "number", parsed2["type"])
}

func TestExtractSchemaSliceOneOf(t *testing.T) {
	// Property defined through oneOf — extract from whichever branch has it
	schema := `{
		"type":"object",
		"properties":{
			"contact":{
				"oneOf":[
					{"type":"object","properties":{"email":{"type":"string","title":"Email"}}},
					{"type":"object","properties":{"phone":{"type":"string","title":"Phone"}}}
				]
			}
		}
	}`
	slice := extractSchemaSlice(schema, "contact.email", nil)
	require.NotEmpty(t, slice, "should find email through oneOf")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "Email", parsed["title"])

	slice2 := extractSchemaSlice(schema, "contact.phone", nil)
	require.NotEmpty(t, slice2, "should find phone through oneOf")
}

func TestExtractSchemaSliceAnyOf(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{
			"data":{
				"anyOf":[
					{"type":"object","properties":{"width":{"type":"integer"}}},
					{"type":"object","properties":{"label":{"type":"string"}}}
				]
			}
		}
	}`
	slice := extractSchemaSlice(schema, "data.width", nil)
	require.NotEmpty(t, slice, "should find width through anyOf")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "integer", parsed["type"])
}

func TestExtractSchemaSliceRefInsideAllOf(t *testing.T) {
	// allOf with a $ref inside one branch
	schema := `{
		"type":"object",
		"properties":{
			"full":{
				"allOf":[
					{"$ref":"#/$defs/Base"},
					{"properties":{"extra":{"type":"boolean","title":"Extra"}}}
				]
			}
		},
		"$defs":{"Base":{"type":"object","properties":{"id":{"type":"integer","title":"ID"}}}}
	}`
	slice := extractSchemaSlice(schema, "full.id", nil)
	require.NotEmpty(t, slice, "should resolve $ref inside allOf")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "integer", parsed["type"])
	assert.Equal(t, "ID", parsed["title"])

	slice2 := extractSchemaSlice(schema, "full.extra", nil)
	require.NotEmpty(t, slice2, "should find extra alongside $ref branch")
}

func TestExtractSchemaSliceAllOfPlusOneOf(t *testing.T) {
	// Node carries both allOf and oneOf — both must be resolved.
	schema := `{
		"type":"object",
		"properties":{
			"item":{
				"type":"object",
				"allOf":[{"properties":{"id":{"type":"integer","title":"ID"}}}],
				"oneOf":[
					{"properties":{"color":{"type":"string","title":"Color"}}},
					{"properties":{"size":{"type":"integer","title":"Size"}}}
				]
			}
		}
	}`
	sliceID := extractSchemaSlice(schema, "item.id", nil)
	require.NotEmpty(t, sliceID, "id from allOf must resolve")

	sliceColor := extractSchemaSlice(schema, "item.color", nil)
	require.NotEmpty(t, sliceColor, "color from oneOf must also resolve")

	sliceSize := extractSchemaSlice(schema, "item.size", nil)
	require.NotEmpty(t, sliceSize, "size from oneOf must also resolve")
}

func TestExtractSchemaSliceOverlappingBranches(t *testing.T) {
	// Two allOf branches both define "address" with different child properties.
	// Both children must be reachable after merge.
	schema := `{
		"type":"object",
		"properties":{
			"contact":{
				"allOf":[
					{"type":"object","properties":{"address":{"type":"object","properties":{"street":{"type":"string","title":"Street"}}}}},
					{"properties":{"address":{"properties":{"zip":{"type":"string","title":"ZIP"}}}}}
				]
			}
		}
	}`
	slice := extractSchemaSlice(schema, "contact.address.street", nil)
	require.NotEmpty(t, slice, "street must survive merge with zip branch")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "Street", parsed["title"])

	slice2 := extractSchemaSlice(schema, "contact.address.zip", nil)
	require.NotEmpty(t, slice2, "zip must survive merge with street branch")
	var parsed2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice2), &parsed2))
	assert.Equal(t, "ZIP", parsed2["title"])
}

func TestExtractSchemaSliceIfThenElse(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{
			"kind":{"type":"string","enum":["a","b"]}
		},
		"if":{"properties":{"kind":{"const":"a"}}},
		"then":{"properties":{"aField":{"type":"string","title":"A Field"}}},
		"else":{"properties":{"bField":{"type":"string","title":"B Field"}}}
	}`

	// When kind=a, aField should resolve
	sliceA := extractSchemaSlice(schema, "aField", json.RawMessage(`{"kind":"a","aField":"x"}`))
	require.NotEmpty(t, sliceA, "aField should resolve when kind=a")
	var parsedA map[string]any
	require.NoError(t, json.Unmarshal([]byte(sliceA), &parsedA))
	assert.Equal(t, "A Field", parsedA["title"])

	// When kind=b, bField should resolve
	sliceB := extractSchemaSlice(schema, "bField", json.RawMessage(`{"kind":"b","bField":"y"}`))
	require.NotEmpty(t, sliceB, "bField should resolve when kind=b")
	var parsedB map[string]any
	require.NoError(t, json.Unmarshal([]byte(sliceB), &parsedB))
	assert.Equal(t, "B Field", parsedB["title"])
}

func TestExtractSchemaSliceIfThenElseTypeSafe(t *testing.T) {
	// const: 1 (number) should NOT match kind: "1" (string)
	schema := `{
		"type":"object",
		"properties":{"kind":{"type":"integer"}},
		"if":{"properties":{"kind":{"const":1}}},
		"then":{"properties":{"numField":{"type":"string","title":"Num Field"}}},
		"else":{"properties":{"otherField":{"type":"string","title":"Other Field"}}}
	}`

	// kind is the string "1", not the number 1 — should take else branch
	sliceOther := extractSchemaSlice(schema, "otherField", json.RawMessage(`{"kind":"1"}`))
	require.NotEmpty(t, sliceOther, "otherField should resolve when kind is string '1' (else branch)")

	sliceNum := extractSchemaSlice(schema, "numField", json.RawMessage(`{"kind":"1"}`))
	assert.Empty(t, sliceNum, "numField should NOT resolve when kind is string '1'")

	// kind is the number 1 — should take then branch
	sliceNum2 := extractSchemaSlice(schema, "numField", json.RawMessage(`{"kind":1}`))
	require.NotEmpty(t, sliceNum2, "numField should resolve when kind is number 1 (then branch)")
}

func TestExtractSchemaSliceIfThenElseObjectConst(t *testing.T) {
	// Object-valued const should not panic
	schema := `{
		"type":"object",
		"properties":{"config":{"type":"object"}},
		"if":{"properties":{"config":{"const":{"mode":"advanced"}}}},
		"then":{"properties":{"extra":{"type":"string","title":"Extra"}}},
		"else":{"properties":{"basic":{"type":"string","title":"Basic"}}}
	}`

	// Should not panic regardless of match result
	assert.NotPanics(t, func() {
		extractSchemaSlice(schema, "extra", json.RawMessage(`{"config":{"mode":"advanced"}}`))
	})
	assert.NotPanics(t, func() {
		extractSchemaSlice(schema, "basic", json.RawMessage(`{"config":{"mode":"simple"}}`))
	})
}

func TestExtractSchemaSliceUnsupportedConditionMergesBoth(t *testing.T) {
	// Conditions the Go evaluator can't handle should merge both branches
	// so the field is at least discoverable (graceful degradation).
	schema := `{
		"type":"object",
		"properties":{"score":{"type":"integer"}},
		"if":{"properties":{"score":{"minimum":50}}},
		"then":{"properties":{"pass":{"type":"boolean","title":"Pass"}}},
		"else":{"properties":{"retry":{"type":"boolean","title":"Retry"}}}
	}`

	// "minimum" is not supported by evaluateSimpleCondition — both branches
	// should be merged so both fields are discoverable.
	slicePass := extractSchemaSlice(schema, "pass", json.RawMessage(`{"score":80}`))
	sliceRetry := extractSchemaSlice(schema, "retry", json.RawMessage(`{"score":80}`))
	assert.NotEmpty(t, slicePass, "pass should be discoverable via fallback merge")
	assert.NotEmpty(t, sliceRetry, "retry should also be discoverable via fallback merge")
}

func TestExtractSchemaSliceLeafConditional(t *testing.T) {
	// A leaf property has if/then/else based on its own value
	schema := `{
		"type":"object",
		"properties":{
			"status":{
				"type":"string",
				"if":{"const":"draft"},
				"then":{"title":"Draft Status","description":"This is a draft"},
				"else":{"title":"Published Status","description":"This is published"}
			}
		}
	}`

	slice := extractSchemaSlice(schema, "status", json.RawMessage(`{"status":"draft"}`))
	require.NotEmpty(t, slice)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "Draft Status", parsed["title"])

	slice2 := extractSchemaSlice(schema, "status", json.RawMessage(`{"status":"published"}`))
	require.NotEmpty(t, slice2)
	var parsed2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice2), &parsed2))
	assert.Equal(t, "Published Status", parsed2["title"])
}

func TestExtractSchemaSliceConditionWithMixedKeywords(t *testing.T) {
	// const + required in same condition — should fall back to merge-both
	// because required is unsupported
	schema := `{
		"type":"object",
		"if":{"required":["flag"],"properties":{"kind":{"const":"a"}}},
		"then":{"properties":{"aField":{"type":"string","title":"A"}}},
		"else":{"properties":{"bField":{"type":"string","title":"B"}}}
	}`

	// Even though kind=a matches, required:["flag"] is unsupported —
	// should merge both branches, not trust the const-only match
	sliceA := extractSchemaSlice(schema, "aField", json.RawMessage(`{"kind":"a"}`))
	sliceB := extractSchemaSlice(schema, "bField", json.RawMessage(`{"kind":"a"}`))
	assert.NotEmpty(t, sliceA, "aField should be discoverable via fallback merge")
	assert.NotEmpty(t, sliceB, "bField should also be discoverable via fallback merge")
}

func TestExtractSchemaSliceConditionVacuousMatch(t *testing.T) {
	// Missing properties should match vacuously (per JSON Schema spec),
	// so {} should take the then-branch when if only constrains "kind".
	schema := `{
		"type":"object",
		"if":{"properties":{"kind":{"const":"a"}}},
		"then":{"properties":{"thenField":{"type":"string","title":"Then"}}},
		"else":{"properties":{"elseField":{"type":"string","title":"Else"}}}
	}`

	slice := extractSchemaSlice(schema, "thenField", json.RawMessage(`{}`))
	assert.NotEmpty(t, slice, "thenField should resolve for {} (vacuous match)")

	sliceElse := extractSchemaSlice(schema, "elseField", json.RawMessage(`{}`))
	assert.Empty(t, sliceElse, "elseField should NOT resolve for {} (vacuous match picks then)")
}

func TestExtractSchemaSlicePropertyLevelUnsupportedKeyword(t *testing.T) {
	// const + minLength on the same property — should fall back to merge-both
	schema := `{
		"type":"object",
		"if":{"properties":{"code":{"const":"A","minLength":2}}},
		"then":{"properties":{"x":{"type":"string","title":"X"}}},
		"else":{"properties":{"y":{"type":"string","title":"Y"}}}
	}`

	sliceX := extractSchemaSlice(schema, "x", json.RawMessage(`{"code":"A"}`))
	sliceY := extractSchemaSlice(schema, "y", json.RawMessage(`{"code":"A"}`))
	assert.NotEmpty(t, sliceX, "x should be discoverable via fallback merge")
	assert.NotEmpty(t, sliceY, "y should also be discoverable via fallback merge")
}

func TestExtractSchemaSliceNotFound(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"string"}}}`
	slice := extractSchemaSlice(schema, "b.c", nil)
	assert.Equal(t, "", slice)
}

func TestExtractSchemaSliceEmptySchema(t *testing.T) {
	slice := extractSchemaSlice("", "a.b", nil)
	assert.Equal(t, "", slice)
}
