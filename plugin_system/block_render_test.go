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

func TestRenderBlock_HtmlEscapeHelper(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "escape-test", `
plugin = { name = "escape-test", version = "1.0", description = "test" }
function init()
    mah.block_type({
        type = "safe",
        label = "Safe",
        render_view = function(ctx)
            return "<div>" .. mah.html_escape(ctx.block.content.text) .. "</div>"
        end,
        render_edit = function(ctx)
            return "<input value=\"" .. mah.html_escape(ctx.block.content.text) .. "\">"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("escape-test"))

	blockCtx := BlockRenderContext{
		Block: BlockRenderData{
			ID:      1,
			Content: map[string]any{"text": `<script>alert("xss")</script>`},
			State:   map[string]any{},
		},
		Note:     NoteRenderData{ID: 1},
		Settings: map[string]any{},
	}

	html, err := pm.RenderBlock("escape-test", "plugin:escape-test:safe", "view", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "&lt;script&gt;")
	assert.NotContains(t, html, "<script>")

	html, err = pm.RenderBlock("escape-test", "plugin:escape-test:safe", "edit", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "&quot;xss&quot;")
}

func TestRenderBlock_NilVMLock(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nil-lock", `
plugin = { name = "nil-lock", version = "1.0", description = "test" }
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
	require.NoError(t, pm.EnablePlugin("nil-lock"))

	// Manually remove the VM lock to simulate the race condition
	pbt := pm.GetPluginBlockType("plugin:nil-lock:simple")
	require.NotNil(t, pbt)

	pm.mu.Lock()
	delete(pm.vmLocks, pbt.State)
	pm.mu.Unlock()

	_, err = pm.RenderBlock("nil-lock", "plugin:nil-lock:simple", "view", BlockRenderContext{
		Block:    BlockRenderData{ID: 1, Content: map[string]any{}, State: map[string]any{}},
		Note:     NoteRenderData{ID: 1},
		Settings: map[string]any{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no longer available")
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
