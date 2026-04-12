package plugin_system

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortcodeRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-test", `
		plugin = { name = "sc-test", version = "1.0" }
		function init()
			mah.shortcode({
				name = "greeting",
				label = "Greeting",
				render = function(ctx)
					return "<span>Hello from plugin!</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-test"))

	sc := pm.GetPluginShortcode("plugin:sc-test:greeting")
	require.NotNil(t, sc)
	assert.Equal(t, "Greeting", sc.Label)
	assert.Equal(t, "sc-test", sc.PluginName)
}

func TestShortcodeRendering(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-render", `
		plugin = { name = "sc-render", version = "1.0" }
		function init()
			mah.shortcode({
				name = "stars",
				label = "Star Rating",
				render = function(ctx)
					local max = tonumber(ctx.attrs.max) or 5
					local stars = ""
					for i = 1, max do stars = stars .. "★" end
					return "<span>" .. stars .. "</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-render"))

	html, err := pm.RenderShortcode(
		context.Background(),
		"sc-render",
		"plugin:sc-render:stars",
		"group", 1,
		json.RawMessage(`{"rating": 4}`),
		map[string]string{"max": "3"},
		nil,
		"", false,
	)
	require.NoError(t, err)
	assert.Equal(t, "<span>★★★</span>", html)
}

func TestShortcodeRenderContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-ctx", `
		plugin = { name = "sc-ctx", version = "1.0" }
		function init()
			mah.shortcode({
				name = "info",
				label = "Info",
				render = function(ctx)
					return ctx.entity_type .. ":" .. tostring(ctx.entity_id)
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-ctx"))

	html, err := pm.RenderShortcode(
		context.Background(),
		"sc-ctx",
		"plugin:sc-ctx:info",
		"resource", 42,
		json.RawMessage(`{}`),
		map[string]string{},
		nil,
		"", false,
	)
	require.NoError(t, err)
	assert.Equal(t, "resource:42", html)
}

func TestShortcodeNonStringReturnErrors(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-badret", `
		plugin = { name = "sc-badret", version = "1.0" }
		function init()
			mah.shortcode({
				name = "nilret",
				label = "Nil Return",
				render = function(ctx) return nil end
			})
			mah.shortcode({
				name = "numret",
				label = "Number Return",
				render = function(ctx) return 42 end
			})
			mah.shortcode({
				name = "boolret",
				label = "Bool Return",
				render = function(ctx) return true end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("sc-badret"))

	_, err = pm.RenderShortcode(context.Background(), "sc-badret", "plugin:sc-badret:nilret", "group", 1, json.RawMessage(`{}`), nil, nil, "", false)
	assert.Error(t, err, "nil return should be an error")

	_, err = pm.RenderShortcode(context.Background(), "sc-badret", "plugin:sc-badret:numret", "group", 1, json.RawMessage(`{}`), nil, nil, "", false)
	assert.Error(t, err, "number return should be an error")

	_, err = pm.RenderShortcode(context.Background(), "sc-badret", "plugin:sc-badret:boolret", "group", 1, json.RawMessage(`{}`), nil, nil, "", false)
	assert.Error(t, err, "boolean return should be an error")
}

func TestShortcodeDuplicate(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-dup", `
		plugin = { name = "sc-dup", version = "1.0" }
		function init()
			mah.shortcode({
				name = "test",
				label = "Test",
				render = function(ctx) return "a" end
			})
			mah.shortcode({
				name = "test",
				label = "Test2",
				render = function(ctx) return "b" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("sc-dup")
	assert.Error(t, err)
}

func TestShortcodeInvalidName(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-bad", `
		plugin = { name = "sc-bad", version = "1.0" }
		function init()
			mah.shortcode({
				name = "INVALID",
				label = "Bad",
				render = function(ctx) return "" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("sc-bad")
	assert.Error(t, err)
}

func TestShortcodeCleanupOnDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-cleanup", `
		plugin = { name = "sc-cleanup", version = "1.0" }
		function init()
			mah.shortcode({
				name = "temp",
				label = "Temp",
				render = function(ctx) return "temp" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-cleanup"))
	assert.NotNil(t, pm.GetPluginShortcode("plugin:sc-cleanup:temp"))

	require.NoError(t, pm.DisablePlugin("sc-cleanup"))
	assert.Nil(t, pm.GetPluginShortcode("plugin:sc-cleanup:temp"))
}

func TestShortcodeEntityContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-entity", `
		plugin = { name = "sc-entity", version = "1.0" }
		function init()
			mah.shortcode({
				name = "entfield",
				label = "Entity Field",
				render = function(ctx)
					if ctx.entity == nil then return "no entity" end
					return tostring(ctx.entity.Name) .. ":" .. tostring(ctx.entity.FileSize)
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-entity"))

	html, err := pm.RenderShortcode(
		context.Background(),
		"sc-entity",
		"plugin:sc-entity:entfield",
		"resource", 1,
		json.RawMessage(`{"rating": 5}`),
		map[string]string{},
		&testResourceEntity{Name: "photo.jpg", FileSize: 1024},
		"", false,
	)
	require.NoError(t, err)
	assert.Equal(t, "photo.jpg:1024", html)
}

type testResourceEntity struct {
	Name        string
	FileSize    int64
	ContentType string
}

func TestRenderShortcodeBlockContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-block", `
		plugin = { name = "sc-block", version = "1.0" }
		function init()
			mah.shortcode({
				name = "wrapper",
				label = "Wrapper",
				render = function(ctx)
					if ctx.is_block then
						return "<div class=\"block\">" .. ctx.inner_content .. "</div>"
					end
					return "<span>inline</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-block"))

	// Inline mode: is_block=false, inner_content=""
	inlineResult, err := pm.RenderShortcode(
		context.Background(),
		"sc-block",
		"plugin:sc-block:wrapper",
		"group", 1,
		json.RawMessage(`{}`),
		map[string]string{},
		nil,
		"", false,
	)
	require.NoError(t, err)
	assert.Equal(t, "<span>inline</span>", inlineResult)

	// Block mode: is_block=true, inner_content populated
	blockResult, err := pm.RenderShortcode(
		context.Background(),
		"sc-block",
		"plugin:sc-block:wrapper",
		"group", 1,
		json.RawMessage(`{}`),
		map[string]string{},
		nil,
		"hello world", true,
	)
	require.NoError(t, err)
	assert.Equal(t, `<div class="block">hello world</div>`, blockResult)
}
