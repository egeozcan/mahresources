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
	assert.Contains(t, result, `data-value='30'`)
	assert.Contains(t, result, `data-editable="false"`)
	assert.Contains(t, result, `data-hide-empty="false"`)
	assert.Contains(t, result, `"type":"integer"`)
	assert.Contains(t, result, `"title":"Cooking Time"`)
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
	assert.Contains(t, result, `data-value='"test"'`)
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
	assert.Contains(t, result, `data-value=''`)
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
	assert.Contains(t, result, `"lat"`)
}

func TestExtractSchemaSlice(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"string","title":"B Field"}}}}}`
	slice := extractSchemaSlice(schema, "a.b")
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "B Field", parsed["title"])
}

func TestExtractSchemaSliceNotFound(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"string"}}}`
	slice := extractSchemaSlice(schema, "b.c")
	assert.Equal(t, "", slice)
}

func TestExtractSchemaSliceEmptySchema(t *testing.T) {
	slice := extractSchemaSlice("", "a.b")
	assert.Equal(t, "", slice)
}
