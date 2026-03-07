package plugin_system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBlockView(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "render-test", `
plugin = { name = "render-test", version = "1.0", description = "test" }

function init()
    mah.block_type({
        type = "simple",
        label = "Simple",
        render_view = function(ctx)
            return "<div>View: " .. ctx.block.id .. " note:" .. ctx.note.id .. "</div>"
        end,
        render_edit = function(ctx)
            return "<div>Edit: " .. ctx.block.id .. "</div>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("render-test"))

	blockCtx := BlockRenderContext{
		Block: BlockRenderData{
			ID:       42,
			Content:  map[string]any{"text": "hello"},
			State:    map[string]any{},
			Position: "n",
		},
		Note: NoteRenderData{
			ID:         10,
			Name:       "Test Note",
			NoteTypeID: 1,
		},
		Settings: map[string]any{},
	}

	// Test view rendering
	html, err := pm.RenderBlock("render-test", "plugin:render-test:simple", "view", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "View: 42")
	assert.Contains(t, html, "note:10")

	// Test edit rendering
	html, err = pm.RenderBlock("render-test", "plugin:render-test:simple", "edit", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "Edit: 42")
}

func TestRenderBlock_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "render-mode", `
plugin = { name = "render-mode", version = "1.0", description = "test" }
function init()
    mah.block_type({
        type = "simple",
        label = "Simple",
        render_view = function(ctx) return "view" end,
        render_edit = function(ctx) return "edit" end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("render-mode"))

	_, err = pm.RenderBlock("render-mode", "plugin:render-mode:simple", "invalid", BlockRenderContext{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid render mode")
}

func TestRenderBlock_PluginNotFound(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	_, err = pm.RenderBlock("nonexistent", "plugin:nonexistent:x", "view", BlockRenderContext{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenderBlock_WrongPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "plugin-a", `
plugin = { name = "plugin-a", version = "1.0", description = "test" }
function init()
    mah.block_type({
        type = "myblock",
        label = "My Block",
        render_view = function(ctx) return "a" end,
        render_edit = function(ctx) return "a" end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("plugin-a"))

	_, err = pm.RenderBlock("plugin-b", "plugin:plugin-a:myblock", "view", BlockRenderContext{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong")
}
