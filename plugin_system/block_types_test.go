package plugin_system

import (
	"encoding/json"
	"strings"
	"testing"

	"mahresources/models/block_types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginBlockType_Type(t *testing.T) {
	bt := &PluginBlockType{
		TypeName: "plugin:myplugin:custom-block",
	}
	assert.Equal(t, "plugin:myplugin:custom-block", bt.Type())
}

func TestPluginBlockType_Defaults(t *testing.T) {
	defContent := json.RawMessage(`{"text":"hello"}`)
	defState := json.RawMessage(`{"collapsed":false}`)

	bt := &PluginBlockType{
		DefContent: defContent,
		DefState:   defState,
	}

	assert.JSONEq(t, `{"text":"hello"}`, string(bt.DefaultContent()))
	assert.JSONEq(t, `{"collapsed":false}`, string(bt.DefaultState()))
}

func TestPluginBlockType_Defaults_Nil(t *testing.T) {
	bt := &PluginBlockType{}

	assert.Nil(t, bt.DefaultContent())
	assert.Nil(t, bt.DefaultState())
}

func TestPluginBlockType_ValidateContent_NoSchema(t *testing.T) {
	bt := &PluginBlockType{
		// No contentSchema set — should accept anything
	}

	err := bt.ValidateContent(json.RawMessage(`{"anything":"goes"}`))
	assert.NoError(t, err)

	err = bt.ValidateContent(json.RawMessage(`"just a string"`))
	assert.NoError(t, err)

	err = bt.ValidateContent(json.RawMessage(`42`))
	assert.NoError(t, err)
}

func TestPluginBlockType_ValidateContent_WithSchema(t *testing.T) {
	contentSchemaJSON := `{
		"type": "object",
		"properties": {
			"text": {"type": "string"},
			"level": {"type": "integer"}
		},
		"required": ["text"]
	}`

	bt, err := NewPluginBlockType(PluginBlockTypeConfig{
		PluginName:    "testplugin",
		TypeName:      "plugin:testplugin:heading",
		Label:         "Test Heading",
		ContentSchema: contentSchemaJSON,
	})
	require.NoError(t, err)

	t.Run("valid content", func(t *testing.T) {
		err := bt.ValidateContent(json.RawMessage(`{"text":"hello","level":2}`))
		assert.NoError(t, err)
	})

	t.Run("missing required field", func(t *testing.T) {
		err := bt.ValidateContent(json.RawMessage(`{"level":2}`))
		assert.Error(t, err)
	})

	t.Run("wrong type for field", func(t *testing.T) {
		err := bt.ValidateContent(json.RawMessage(`{"text":123}`))
		assert.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		err := bt.ValidateContent(json.RawMessage(`not json`))
		assert.Error(t, err)
	})
}

func TestPluginBlockType_ValidateState_NoSchema(t *testing.T) {
	bt := &PluginBlockType{
		// No stateSchema set — should accept anything
	}

	err := bt.ValidateState(json.RawMessage(`{"any":"thing"}`))
	assert.NoError(t, err)
}

func TestPluginBlockType_ValidateState_WithSchema(t *testing.T) {
	stateSchemaJSON := `{
		"type": "object",
		"properties": {
			"collapsed": {"type": "boolean"}
		},
		"required": ["collapsed"]
	}`

	bt, err := NewPluginBlockType(PluginBlockTypeConfig{
		PluginName:  "testplugin",
		TypeName:    "plugin:testplugin:myblock",
		Label:       "Test Block",
		StateSchema: stateSchemaJSON,
	})
	require.NoError(t, err)

	t.Run("valid state", func(t *testing.T) {
		err := bt.ValidateState(json.RawMessage(`{"collapsed":true}`))
		assert.NoError(t, err)
	})

	t.Run("missing required field", func(t *testing.T) {
		err := bt.ValidateState(json.RawMessage(`{}`))
		assert.Error(t, err)
	})

	t.Run("wrong type for field", func(t *testing.T) {
		err := bt.ValidateState(json.RawMessage(`{"collapsed":"yes"}`))
		assert.Error(t, err)
	})
}

func TestNewPluginBlockType_InvalidSchema(t *testing.T) {
	t.Run("invalid content schema", func(t *testing.T) {
		_, err := NewPluginBlockType(PluginBlockTypeConfig{
			PluginName:    "testplugin",
			TypeName:      "plugin:testplugin:bad",
			Label:         "Bad",
			ContentSchema: `{"type": "not-a-real-type"}`,
		})
		assert.Error(t, err)
	})

	t.Run("invalid state schema", func(t *testing.T) {
		_, err := NewPluginBlockType(PluginBlockTypeConfig{
			PluginName:  "testplugin",
			TypeName:    "plugin:testplugin:bad",
			Label:       "Bad",
			StateSchema: `{"type": "not-a-real-type"}`,
		})
		assert.Error(t, err)
	})

	t.Run("malformed JSON in content schema", func(t *testing.T) {
		_, err := NewPluginBlockType(PluginBlockTypeConfig{
			PluginName:    "testplugin",
			TypeName:      "plugin:testplugin:bad",
			Label:         "Bad",
			ContentSchema: `not valid json`,
		})
		assert.Error(t, err)
	})

	t.Run("empty schema strings are treated as no schema", func(t *testing.T) {
		bt, err := NewPluginBlockType(PluginBlockTypeConfig{
			PluginName:    "testplugin",
			TypeName:      "plugin:testplugin:noschema",
			Label:         "No Schema",
			ContentSchema: "",
			StateSchema:   "",
		})
		require.NoError(t, err)
		assert.NoError(t, bt.ValidateContent(json.RawMessage(`{"anything":"goes"}`)))
		assert.NoError(t, bt.ValidateState(json.RawMessage(`{"anything":"goes"}`)))
	})
}

func TestNewPluginBlockType_FullConfig(t *testing.T) {
	contentSchemaJSON := `{
		"type": "object",
		"properties": {
			"url": {"type": "string"}
		},
		"required": ["url"]
	}`

	bt, err := NewPluginBlockType(PluginBlockTypeConfig{
		PluginName:    "gallery",
		TypeName:      "plugin:gallery:image",
		Label:         "Gallery Image",
		Icon:          "image-icon",
		Description:   "An image block from gallery plugin",
		ContentSchema: contentSchemaJSON,
		DefContent:    json.RawMessage(`{"url":""}`),
		DefState:      json.RawMessage(`{}`),
		Filters: BlockTypeFilter{
			NoteTypeIDs: []uint{1, 2},
			CategoryIDs: []uint{3},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "plugin:gallery:image", bt.Type())
	assert.Equal(t, "gallery", bt.PluginName)
	assert.Equal(t, "Gallery Image", bt.Label)
	assert.Equal(t, "image-icon", bt.Icon)
	assert.Equal(t, "An image block from gallery plugin", bt.Description)
	assert.JSONEq(t, `{"url":""}`, string(bt.DefaultContent()))
	assert.JSONEq(t, `{}`, string(bt.DefaultState()))
	assert.Equal(t, []uint{1, 2}, bt.Filters.NoteTypeIDs)
	assert.Equal(t, []uint{3}, bt.Filters.CategoryIDs)

	// Content validation works
	assert.NoError(t, bt.ValidateContent(json.RawMessage(`{"url":"http://example.com/img.png"}`)))
	assert.Error(t, bt.ValidateContent(json.RawMessage(`{}`)))
}

func TestPluginManager_BlockTypeLifecycle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "blocks-test", `
plugin = { name = "blocks-test", version = "1.0", description = "block type test" }

function view_render(ctx)
    return "<div>view</div>"
end

function edit_render(ctx)
    return "<div>edit</div>"
end

function init()
    mah.block_type({
        type = "custom-block",
        label = "Custom Block",
        icon = "star",
        description = "A custom block type",
        render_view = view_render,
        render_edit = edit_render,
        default_content = { text = "hello" },
        default_state = { collapsed = false },
        filters = {
            note_type_ids = { 1, 2 },
            category_ids = { 3 },
        },
    })
end
`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	// Before enable: no block types
	assert.Empty(t, pm.GetBlockTypes())
	assert.Nil(t, pm.GetPluginBlockType("plugin:blocks-test:custom-block"))

	// Enable plugin
	err = pm.EnablePlugin("blocks-test")
	require.NoError(t, err)

	// After enable: block type registered
	bts := pm.GetBlockTypes()
	require.Len(t, bts, 1)

	bt := bts[0]
	assert.Equal(t, "plugin:blocks-test:custom-block", bt.TypeName)
	assert.Equal(t, "Custom Block", bt.Label)
	assert.Equal(t, "star", bt.Icon)
	assert.Equal(t, "A custom block type", bt.Description)
	assert.Equal(t, "blocks-test", bt.PluginName)
	assert.NotNil(t, bt.RenderView)
	assert.NotNil(t, bt.RenderEdit)
	assert.NotNil(t, bt.State)
	assert.JSONEq(t, `{"text":"hello"}`, string(bt.DefaultContent()))
	assert.JSONEq(t, `{"collapsed":false}`, string(bt.DefaultState()))
	assert.Equal(t, []uint{1, 2}, bt.Filters.NoteTypeIDs)
	assert.Equal(t, []uint{3}, bt.Filters.CategoryIDs)

	// Accessor by full name
	found := pm.GetPluginBlockType("plugin:blocks-test:custom-block")
	assert.NotNil(t, found)
	assert.Equal(t, bt.TypeName, found.TypeName)

	// Global registry should have it
	globalBt := block_types.GetBlockType("plugin:blocks-test:custom-block")
	assert.NotNil(t, globalBt)

	// Disable plugin
	err = pm.DisablePlugin("blocks-test")
	require.NoError(t, err)

	// After disable: block type removed
	assert.Empty(t, pm.GetBlockTypes())
	assert.Nil(t, pm.GetPluginBlockType("plugin:blocks-test:custom-block"))

	// Global registry should no longer have it
	globalBt = block_types.GetBlockType("plugin:blocks-test:custom-block")
	assert.Nil(t, globalBt)
}

func TestPluginManager_BlockTypeDuplicateID(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dup-block", `
plugin = { name = "dup-block", version = "1.0", description = "duplicate block type test" }

function view_render(ctx) return "<div>view</div>" end
function edit_render(ctx) return "<div>edit</div>" end

function init()
    mah.block_type({
        type = "my-block",
        label = "My Block",
        render_view = view_render,
        render_edit = edit_render,
    })
    mah.block_type({
        type = "my-block",
        label = "My Block Again",
        render_view = view_render,
        render_edit = edit_render,
    })
end
`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("dup-block")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "duplicate block type"))
}

func TestPluginManager_BlockTypeMissingRequired(t *testing.T) {
	t.Run("missing label", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "no-label", `
plugin = { name = "no-label", version = "1.0", description = "test" }

function view_render(ctx) return "" end
function edit_render(ctx) return "" end

function init()
    mah.block_type({
        type = "my-block",
        render_view = view_render,
        render_edit = edit_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("no-label")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "label"))
	})

	t.Run("missing render_view", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "no-view", `
plugin = { name = "no-view", version = "1.0", description = "test" }

function edit_render(ctx) return "" end

function init()
    mah.block_type({
        type = "my-block",
        label = "My Block",
        render_edit = edit_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("no-view")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "render_view"))
	})

	t.Run("missing render_edit", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "no-edit", `
plugin = { name = "no-edit", version = "1.0", description = "test" }

function view_render(ctx) return "" end

function init()
    mah.block_type({
        type = "my-block",
        label = "My Block",
        render_view = view_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("no-edit")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "render_edit"))
	})

	t.Run("missing type", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "no-type", `
plugin = { name = "no-type", version = "1.0", description = "test" }

function view_render(ctx) return "" end
function edit_render(ctx) return "" end

function init()
    mah.block_type({
        label = "My Block",
        render_view = view_render,
        render_edit = edit_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("no-type")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "type"))
	})

	t.Run("type field is a number not string", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "num-type", `
plugin = { name = "num-type", version = "1.0", description = "test" }

function view_render(ctx) return "" end
function edit_render(ctx) return "" end

function init()
    mah.block_type({
        type = 42,
        label = "My Block",
        render_view = view_render,
        render_edit = edit_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("num-type")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "must be a string"))
	})

	t.Run("invalid type name", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "bad-type-name", `
plugin = { name = "bad-type-name", version = "1.0", description = "test" }

function view_render(ctx) return "" end
function edit_render(ctx) return "" end

function init()
    mah.block_type({
        type = "INVALID_NAME",
        label = "My Block",
        render_view = view_render,
        render_edit = edit_render,
    })
end
`)

		pm, err := NewPluginManager(dir)
		require.NoError(t, err)
		defer pm.Close()

		err = pm.EnablePlugin("bad-type-name")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "invalid type name"))
	})
}
