package shortcodes

import (
	"context"
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

func TestRenderMetaForceReadOnly(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	// ForceReadOnly (set by the share renderer) must override editable="true",
	// so the public share page never emits an edit affordance.
	ctx := MetaShortcodeContext{
		EntityType:    "note",
		EntityID:      7,
		Meta:          metaJSON,
		ForceReadOnly: true,
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "name", "editable": "true"},
	}, ctx)

	assert.Contains(t, result, `data-editable="false"`)
	assert.NotContains(t, result, `data-editable="true"`)
}

func TestRenderMetaDefaultAttr(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "missing", "default": "n/a"},
	}, ctx)

	assert.Contains(t, result, `data-default="n/a"`)
}

func TestRenderMetaDefaultAttrOmittedWhenUnset(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "x"},
	}, ctx)
	assert.Contains(t, result, `data-default=""`)
}

func TestRenderMetaDefaultAttrEscaped(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "x", "default": `<b>"y"</b>`},
	}, ctx)
	assert.NotContains(t, result, "<b>")
	assert.Contains(t, result, "&lt;b&gt;")
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
	sliceA := extractSchemaSlice(schema, "aField", metaObject(`{"kind":"a","aField":"x"}`))
	require.NotEmpty(t, sliceA, "aField should resolve when kind=a")
	var parsedA map[string]any
	require.NoError(t, json.Unmarshal([]byte(sliceA), &parsedA))
	assert.Equal(t, "A Field", parsedA["title"])

	// When kind=b, bField should resolve
	sliceB := extractSchemaSlice(schema, "bField", metaObject(`{"kind":"b","bField":"y"}`))
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
	sliceOther := extractSchemaSlice(schema, "otherField", metaObject(`{"kind":"1"}`))
	require.NotEmpty(t, sliceOther, "otherField should resolve when kind is string '1' (else branch)")

	sliceNum := extractSchemaSlice(schema, "numField", metaObject(`{"kind":"1"}`))
	assert.Empty(t, sliceNum, "numField should NOT resolve when kind is string '1'")

	// kind is the number 1 — should take then branch
	sliceNum2 := extractSchemaSlice(schema, "numField", metaObject(`{"kind":1}`))
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
		extractSchemaSlice(schema, "extra", metaObject(`{"config":{"mode":"advanced"}}`))
	})
	assert.NotPanics(t, func() {
		extractSchemaSlice(schema, "basic", metaObject(`{"config":{"mode":"simple"}}`))
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
	slicePass := extractSchemaSlice(schema, "pass", metaObject(`{"score":80}`))
	sliceRetry := extractSchemaSlice(schema, "retry", metaObject(`{"score":80}`))
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

	slice := extractSchemaSlice(schema, "status", metaObject(`{"status":"draft"}`))
	require.NotEmpty(t, slice)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(slice), &parsed))
	assert.Equal(t, "Draft Status", parsed["title"])

	slice2 := extractSchemaSlice(schema, "status", metaObject(`{"status":"published"}`))
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
	sliceA := extractSchemaSlice(schema, "aField", metaObject(`{"kind":"a"}`))
	sliceB := extractSchemaSlice(schema, "bField", metaObject(`{"kind":"a"}`))
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

	slice := extractSchemaSlice(schema, "thenField", metaObject(`{}`))
	assert.NotEmpty(t, slice, "thenField should resolve for {} (vacuous match)")

	sliceElse := extractSchemaSlice(schema, "elseField", metaObject(`{}`))
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

	sliceX := extractSchemaSlice(schema, "x", metaObject(`{"code":"A"}`))
	sliceY := extractSchemaSlice(schema, "y", metaObject(`{"code":"A"}`))
	assert.NotEmpty(t, sliceX, "x should be discoverable via fallback merge")
	assert.NotEmpty(t, sliceY, "y should also be discoverable via fallback merge")
}

func TestExtractSchemaSliceConditionTypeCheck(t *testing.T) {
	// if: {properties: {code: {type: "string"}}} should NOT match {code: 42}
	schema := `{
		"type":"object",
		"if":{"properties":{"code":{"type":"string"}}},
		"then":{"properties":{"strField":{"type":"string","title":"Str"}}},
		"else":{"properties":{"numField":{"type":"string","title":"Num"}}}
	}`

	// code is a number — should take else branch
	sliceNum := extractSchemaSlice(schema, "numField", metaObject(`{"code":42}`))
	assert.NotEmpty(t, sliceNum, "numField should resolve when code is number (else branch)")

	sliceStr := extractSchemaSlice(schema, "strField", metaObject(`{"code":42}`))
	assert.Empty(t, sliceStr, "strField should NOT resolve when code is number")

	// code is a string — should take then branch
	sliceStr2 := extractSchemaSlice(schema, "strField", metaObject(`{"code":"hello"}`))
	assert.NotEmpty(t, sliceStr2, "strField should resolve when code is string (then branch)")
}

func TestExtractSchemaSliceConditionTopLevelType(t *testing.T) {
	// Top-level if: {type: "string"} — should evaluate, not fall back
	schema := `{
		"if":{"type":"object"},
		"then":{"type":"object","properties":{"objField":{"type":"string","title":"Obj"}}},
		"else":{"type":"object","properties":{"otherField":{"type":"string","title":"Other"}}}
	}`

	// Value is an object — should take then branch
	slice := extractSchemaSlice(schema, "objField", metaObject(`{"objField":"x"}`))
	assert.NotEmpty(t, slice, "objField should resolve when value is object (then branch)")
}

func TestExtractSchemaSliceConditionIntegerVsNumber(t *testing.T) {
	schema := `{
		"type":"object",
		"if":{"properties":{"val":{"type":"integer"}}},
		"then":{"properties":{"intField":{"type":"string","title":"Int"}}},
		"else":{"properties":{"floatField":{"type":"string","title":"Float"}}}
	}`

	// 3.14 is not an integer — should take else branch
	slice := extractSchemaSlice(schema, "floatField", metaObject(`{"val":3.14}`))
	assert.NotEmpty(t, slice, "floatField should resolve when val is 3.14 (else)")

	sliceInt := extractSchemaSlice(schema, "intField", metaObject(`{"val":3.14}`))
	assert.Empty(t, sliceInt, "intField should NOT resolve when val is 3.14")

	// 5 is an integer — should take then branch
	sliceInt2 := extractSchemaSlice(schema, "intField", metaObject(`{"val":5}`))
	assert.NotEmpty(t, sliceInt2, "intField should resolve when val is 5 (then)")
}

func TestExtractSchemaSliceConditionTypeArray(t *testing.T) {
	schema := `{
		"type":"object",
		"if":{"properties":{"code":{"type":["string","null"]}}},
		"then":{"properties":{"thenF":{"type":"string","title":"Then"}}},
		"else":{"properties":{"elseF":{"type":"string","title":"Else"}}}
	}`

	// 42 is not string or null — should take else
	sliceElse := extractSchemaSlice(schema, "elseF", metaObject(`{"code":42}`))
	assert.NotEmpty(t, sliceElse, "elseF should resolve when code is number")
	sliceThen := extractSchemaSlice(schema, "thenF", metaObject(`{"code":42}`))
	assert.Empty(t, sliceThen, "thenF should NOT resolve when code is number")

	// "hello" is string — should take then
	sliceThen2 := extractSchemaSlice(schema, "thenF", metaObject(`{"code":"hello"}`))
	assert.NotEmpty(t, sliceThen2, "thenF should resolve when code is string")

	// null is null — should take then
	sliceThen3 := extractSchemaSlice(schema, "thenF", metaObject(`{"code":null}`))
	assert.NotEmpty(t, sliceThen3, "thenF should resolve when code is null")
}

func TestExtractSchemaSliceConditionalBranchWithRef(t *testing.T) {
	// then-branch introduces a $ref that must be resolved before returning
	schema := `{
		"type":"object",
		"properties":{
			"mode":{"type":"string"},
			"detail":{
				"type":"string",
				"if":{"const":"advanced"},
				"then":{"$ref":"#/$defs/AdvDetail"},
				"else":{"title":"Basic Detail"}
			}
		},
		"$defs":{"AdvDetail":{"title":"Advanced Detail","description":"full detail"}}
	}`

	slice := extractSchemaSlice(schema, "detail", metaObject(`{"mode":"x","detail":"advanced"}`))
	require.NotEmpty(t, slice)
	assert.NotContains(t, slice, "$ref", "branch $ref must be resolved")
	assert.Contains(t, slice, "Advanced Detail")
}

func TestExtractSchemaSliceNestedRefInCompositionBranch(t *testing.T) {
	// oneOf branch contains allOf with a nested $ref
	schema := `{
		"type":"object",
		"properties":{
			"item":{
				"oneOf":[
					{"allOf":[{"$ref":"#/$defs/Base"},{"properties":{"extra":{"type":"boolean"}}}]},
					{"const":"none","title":"None"}
				]
			}
		},
		"$defs":{"Base":{"const":"full","title":"Full Item"}}
	}`

	slice := extractSchemaSlice(schema, "item", nil)
	require.NotEmpty(t, slice)
	assert.NotContains(t, slice, "$ref", "nested $ref inside allOf inside oneOf must be resolved")
	assert.Contains(t, slice, "Full Item")
}

func TestExtractSchemaSliceRefInProperties(t *testing.T) {
	// Leaf schema has properties with $ref — must be inlined
	schema := `{
		"type":"object",
		"properties":{
			"address":{
				"type":"object",
				"properties":{
					"street":{"$ref":"#/$defs/StreetType"},
					"city":{"type":"string","title":"City"}
				}
			}
		},
		"$defs":{"StreetType":{"type":"string","title":"Street Name","maxLength":100}}
	}`

	slice := extractSchemaSlice(schema, "address", nil)
	require.NotEmpty(t, slice)
	assert.NotContains(t, slice, "$ref", "properties.$ref must be inlined")
	assert.Contains(t, slice, "Street Name")
	assert.Contains(t, slice, "City")
}

func TestExtractSchemaSliceRefInChildConditional(t *testing.T) {
	// A leaf property's child has if/then with $ref in the then-branch
	schema := `{
		"type":"object",
		"properties":{
			"address":{
				"type":"object",
				"properties":{
					"zip":{
						"type":"string",
						"if":{"const":"12345"},
						"then":{"$ref":"#/$defs/SpecialZip"},
						"else":{"title":"Normal ZIP"}
					}
				}
			}
		},
		"$defs":{"SpecialZip":{"title":"Special ZIP","pattern":"^\\d{5}-\\d{4}$"}}
	}`

	slice := extractSchemaSlice(schema, "address", nil)
	require.NotEmpty(t, slice)
	assert.NotContains(t, slice, "$ref", "$ref inside child if/then must be inlined")
	assert.Contains(t, slice, "Special ZIP")
}

func TestExtractSchemaSliceRefInsideOneOfBranch(t *testing.T) {
	// A leaf with oneOf where a branch uses $ref — the $ref must be resolved
	// in the leaf slice since the client doesn't have root $defs.
	schema := `{
		"type":"object",
		"properties":{
			"status":{
				"oneOf":[
					{"$ref":"#/$defs/Draft"},
					{"const":"published","title":"Published"}
				]
			}
		},
		"$defs":{"Draft":{"const":"draft","title":"Draft"}}
	}`

	slice := extractSchemaSlice(schema, "status", nil)
	require.NotEmpty(t, slice)
	// The resolved slice should contain the inlined Draft definition,
	// not a dangling $ref
	assert.NotContains(t, slice, "$ref")
	assert.Contains(t, slice, "Draft")
	assert.Contains(t, slice, "Published")
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

// [meta] renders a <meta-shortcode> custom element, which cannot sit inside an
// HTML attribute — an element nested in an attribute value is broken markup, and
// the element's own quotes close the attribute early. inline="true" is the way
// out: a bare value, formatted through the same helpers as [property]/[item].

func inlineMetaCtx() MetaShortcodeContext {
	metaJSON, _ := json.Marshal(map[string]any{
		"slug":  "hello-world",
		"quote": `a "quoted" & <tagged> value`,
		"blurb": "<em>emphasis</em>",
		"when":  "2026-08-22T10:30:00Z",
		"size":  float64(2048),
		"nest":  map[string]any{"deep": "found"},
	})
	return MetaShortcodeContext{EntityType: "group", EntityID: 7, Meta: metaJSON}
}

func renderInlineMeta(attrs map[string]string) string {
	return RenderMetaShortcode(Shortcode{Name: "meta", Attrs: attrs}, inlineMetaCtx())
}

func TestRenderMetaInlineRendersBareValue(t *testing.T) {
	got := renderInlineMeta(map[string]string{"path": "slug", "inline": "true"})
	assert.Equal(t, "hello-world", got)
	assert.NotContains(t, got, "meta-shortcode")
}

func TestRenderMetaInlineFollowsDotPaths(t *testing.T) {
	assert.Equal(t, "found", renderInlineMeta(map[string]string{"path": "nest.deep", "inline": "true"}))
}

// The whole point of the mode: the output has to survive being pasted into an
// attribute value, so every quote must come back escaped.
func TestRenderMetaInlineIsAttributeSafe(t *testing.T) {
	got := renderInlineMeta(map[string]string{"path": "quote", "inline": "true"})
	assert.NotContains(t, got, `"`)
	assert.NotContains(t, got, "<")
	assert.Contains(t, got, "&#34;")
	assert.Contains(t, got, "&amp;")
	assert.Contains(t, got, "&lt;")
}

// raw= keeps the meaning it already has on [property] and [item]: skip escaping.
func TestRenderMetaInlineRawSkipsEscaping(t *testing.T) {
	assert.Equal(t, "<em>emphasis</em>",
		renderInlineMeta(map[string]string{"path": "blurb", "inline": "true", "raw": "true"}))
}

func TestRenderMetaInlineFormatsLikeProperty(t *testing.T) {
	assert.Equal(t, "2026-08-22",
		renderInlineMeta(map[string]string{"path": "when", "inline": "true", "format": "date"}))
	assert.Equal(t, "Aug 22, 2026",
		renderInlineMeta(map[string]string{"path": "when", "inline": "true", "layout": "Jan 2, 2006"}))
	assert.Equal(t, "2.0 KB",
		renderInlineMeta(map[string]string{"path": "size", "inline": "true", "format": "filesize"}))
}

func TestRenderMetaInlineDefaultAndHideEmpty(t *testing.T) {
	assert.Equal(t, "n/a",
		renderInlineMeta(map[string]string{"path": "missing", "inline": "true", "default": "n/a"}))
	// hide-empty is a client-side concern for the widget; inline mode has no
	// client, so it has to be honoured here.
	assert.Equal(t, "",
		renderInlineMeta(map[string]string{"path": "missing", "inline": "true", "hide-empty": "true"}))
	assert.Equal(t, "",
		renderInlineMeta(map[string]string{"path": "missing", "inline": "true"}))
}

// An inline value is text, so there is nothing to edit. Emitting the widget
// because editable was also passed would put an edit affordance inside whatever
// attribute the author was building.
func TestRenderMetaInlineIgnoresEditable(t *testing.T) {
	got := renderInlineMeta(map[string]string{"path": "slug", "inline": "true", "editable": "true"})
	assert.Equal(t, "hello-world", got)
	assert.NotContains(t, got, "meta-shortcode")
}

func TestRenderMetaWidgetModeUnchangedWithoutInline(t *testing.T) {
	got := renderInlineMeta(map[string]string{"path": "slug"})
	assert.Contains(t, got, "<meta-shortcode")
	assert.Contains(t, got, `data-path="slug"`)
}

// The widget's client side treats a whitespace-only value as empty. Inline mode
// has no client, so it has to make the same call server-side or one Meta value
// renders three spaces in a template and the default in the widget beside it.
func TestRenderMetaInlineTreatsWhitespaceAsEmpty(t *testing.T) {
	metaJSON, _ := json.Marshal(map[string]any{"blank": "   "})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: metaJSON}
	render := func(attrs map[string]string) string {
		return RenderMetaShortcode(Shortcode{Name: "meta", Attrs: attrs}, ctx)
	}
	assert.Equal(t, "n/a", render(map[string]string{"path": "blank", "inline": "true", "default": "n/a"}))
	assert.Equal(t, "", render(map[string]string{"path": "blank", "inline": "true", "hide-empty": "true"}))
	// A value that is not blank is still emitted with its own spacing intact.
	padded, _ := json.Marshal(map[string]any{"v": " x "})
	assert.Equal(t, " x ", RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "v", "inline": "true"}},
		MetaShortcodeContext{Meta: padded}))
}

// The memo must be shared by the by-value copies of the context (so one render
// decodes once) and must never outlive the entity: a context built fresh — as
// the MRQL handler does per result item — carries its own.
func TestInlineMetaDecodeIsMemoizedPerEntity(t *testing.T) {
	a, _ := json.Marshal(map[string]any{"v": "first"})
	b, _ := json.Marshal(map[string]any{"v": "second"})

	tpl := `[meta path="v" inline="true"]|[meta path="v" inline="true"]`
	got := Process(context.Background(), tpl, MetaShortcodeContext{Meta: a}, nil, nil)
	assert.Equal(t, "first|first", got)

	// A different entity, a different context value: no bleed from the first.
	assert.Equal(t, "second|second",
		Process(context.Background(), tpl, MetaShortcodeContext{Meta: b}, nil, nil))

	// Called directly with no cache at all, it still decodes correctly.
	assert.Equal(t, "first", RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "v", "inline": "true"}},
		MetaShortcodeContext{Meta: a}))

	// A malformed blob is treated as missing, exactly like an absent path.
	assert.Equal(t, "n/a", RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "v", "inline": "true", "default": "n/a"}},
		MetaShortcodeContext{Meta: []byte("{not json")}))
}

// valueJSONAtPath fills the widget's data-value attribute, so "not found"
// has to stay distinguishable from an explicit null — one is the empty string
// and the other is the JSON text "null". Its answer for an empty path is "",
// the third of the three empty-path behaviours (see
// TestItemWithNoPathRendersTheElementItself and TestRawValueAtPathEdges);
// RenderMetaShortcode never asks, since it returns early on an empty path.
func TestValueJSONAtPathEdges(t *testing.T) {
	assert.Equal(t, `"b"`, valueJSONAt(json.RawMessage(`{"a":"b"}`), "a"))
	assert.Equal(t, `{"b":1}`, valueJSONAt(json.RawMessage(`{"a":{"b":1}}`), "a"))
	assert.Equal(t, "null", valueJSONAt(json.RawMessage(`{"a":null}`), "a"))

	assert.Equal(t, "", valueJSONAt(json.RawMessage(`{"a":"b"}`), "missing"))
	assert.Equal(t, "", valueJSONAt(json.RawMessage(`{"a":"b"}`), ""))
	assert.Equal(t, "", valueJSONAt(json.RawMessage(`{"a":"b"}`), "a.b"))
	assert.Equal(t, "", valueJSONAt(nil, "a"))
	assert.Equal(t, "", valueJSONAt(json.RawMessage(`[1,2]`), "a"))
	assert.Equal(t, "", valueJSONAt(json.RawMessage(`not json`), "a"))
}

// valueJSONAt reads the widget's data-value for a path through a context, the
// way RenderMetaShortcode does.
func valueJSONAt(meta json.RawMessage, path string) string {
	return MetaShortcodeContext{Meta: meta}.valueJSONAtPath(path)
}

// metaObject decodes a Meta blob the way the context's memo does, for the
// extractSchemaSlice cases that need a value context to evaluate if/then/else.
func metaObject(raw string) map[string]any {
	return MetaShortcodeContext{Meta: json.RawMessage(raw)}.decodeMetaObject()
}

// The memo hands one decoded tree to every reader on every by-value copy of the
// context, so the tree has to be read-only: a mutation by one reader would be
// seen by every later reader in that Process call, including the nested ones
// (a card on a list page is its own Process call with its own memo, so the blast
// radius is one entity's render, not the page). Nothing writes to it today — the
// schema resolvers copy into fresh maps and tryEvaluateCondition only reads —
// and this fails if that changes.
func TestDecodedMetaIsSharedReadOnly(t *testing.T) {
	const schema = `{
		"type": "object",
		"properties": {"aField": {"type": "string"}},
		"if": {"properties": {"kind": {"const": "a"}}},
		"then": {"properties": {"aField": {"title": "A"}}},
		"else": {"properties": {"aField": {"title": "B"}}}
	}`
	raw := json.RawMessage(`{"kind":"a","aField":"x","nested":{"b":1},"list":[{"c":2}]}`)

	ctx := MetaShortcodeContext{
		EntityType:  "group",
		EntityID:    1,
		Meta:        raw,
		MetaSchema:  schema,
		decodedMeta: &decodedMetaCache{},
	}

	// Every reader of the shared tree, in one render's worth of calls.
	_ = RenderMetaShortcode(Shortcode{Name: "meta", Attrs: map[string]string{"path": "aField"}}, ctx)
	_ = RenderMetaShortcode(Shortcode{Name: "meta", Attrs: map[string]string{"path": "nested.b", "inline": "true"}}, ctx)
	_ = ctx.rawValueAtPath("list")
	_ = ctx.valueJSONAtPath("nested")

	var want any
	require.NoError(t, json.Unmarshal(raw, &want))
	assert.Equal(t, want, ctx.decodeMeta(), "a reader mutated the shared Meta tree")
}

// Every Meta reader answers from the memo rather than from its own Unmarshal.
// Priming the cache with a tree that disagrees with ctx.Meta is the only way to
// tell the two apart from outside: a reader that decoded for itself would answer
// "from-blob".
func TestEveryMetaReaderReadsThroughTheMemo(t *testing.T) {
	const schema = `{
		"type": "object",
		"properties": {"a": {"type": "string"}},
		"if": {"properties": {"a": {"const": "from-memo"}}},
		"then": {"properties": {"a": {"title": "MEMO"}}},
		"else": {"properties": {"a": {"title": "BLOB"}}}
	}`
	ctx := MetaShortcodeContext{
		EntityType:  "group",
		EntityID:    1,
		Meta:        json.RawMessage(`{"a":"from-blob","list":["blob"]}`),
		MetaSchema:  schema,
		decodedMeta: &decodedMetaCache{},
	}
	ctx.decodedMeta.once.Do(func() {
		ctx.decodedMeta.value = map[string]any{"a": "from-memo", "list": []any{"memo"}}
	})

	// [conditional]/[each]
	assert.Equal(t, "from-memo", ctx.rawValueAtPath("a"))
	// The widget as it is actually rendered, not its two helpers: its data-value
	// and its data-schema (whose if/then is evaluated against the value) both
	// have to come from the memo, and a RenderMetaShortcode that decoded
	// ctx.Meta for either of them would print "from-blob"/"BLOB" here.
	widget := RenderMetaShortcode(Shortcode{Name: "meta", Attrs: map[string]string{"path": "a"}}, ctx)
	assert.Contains(t, widget, "from-memo")
	assert.Contains(t, widget, "MEMO")
	assert.NotContains(t, widget, "from-blob")
	assert.NotContains(t, widget, "BLOB")
	// [meta inline="true"]
	assert.Equal(t, "from-memo", RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "a", "inline": "true"}}, ctx))
	// [each]
	assert.Equal(t, "memo|", RenderEachShortcode(context.Background(),
		Shortcode{Name: "each", Attrs: map[string]string{"path": "list"}, InnerContent: "[item]|", IsBlock: true},
		ctx, nil, nil, 0))
}

// encoding/json keeps decoding after a semantic error, so a blob carrying one
// number too large for a float64 comes back with that key nil and every other
// key populated. Half a Meta must not render: the failed key would print as an
// explicit null, and an editable widget bound to it would offer that null back
// for writing. All four readers answer "nothing", which is what the three
// non-inline ones did before they shared the memo.
func TestPartiallyDecodableMetaReadsAsNothing(t *testing.T) {
	raw := json.RawMessage(`{"tags":["a"],"huge":1e400}`)
	ctx := MetaShortcodeContext{
		EntityType:  "group",
		EntityID:    1,
		Meta:        raw,
		decodedMeta: &decodedMetaCache{},
	}

	assert.Nil(t, ctx.decodeMeta())
	assert.Nil(t, ctx.rawValueAtPath("tags"))
	assert.Equal(t, "", ctx.valueJSONAtPath("huge"))
	assert.Equal(t, "", ctx.valueJSONAtPath("tags"))
	assert.Equal(t, "-", RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "tags", "inline": "true", "default": "-"}}, ctx))

	widget := RenderMetaShortcode(Shortcode{Name: "meta", Attrs: map[string]string{"path": "huge"}}, ctx)
	assert.Contains(t, widget, `data-value=""`, "a key that failed to decode must not render as null")

	assert.Equal(t, "none", RenderEachShortcode(context.Background(),
		Shortcode{Name: "each", Attrs: map[string]string{"path": "tags"}, InnerContent: "[item]|[else]none", IsBlock: true},
		ctx, nil, nil, 0))
}
