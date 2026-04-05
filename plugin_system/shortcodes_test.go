package plugin_system

import (
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
		"sc-render",
		"plugin:sc-render:stars",
		"group", 1,
		json.RawMessage(`{"rating": 4}`),
		map[string]string{"max": "3"},
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
		"sc-ctx",
		"plugin:sc-ctx:info",
		"resource", 42,
		json.RawMessage(`{}`),
		map[string]string{},
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

	_, err = pm.RenderShortcode("sc-badret", "plugin:sc-badret:nilret", "group", 1, json.RawMessage(`{}`), nil)
	assert.Error(t, err, "nil return should be an error")

	_, err = pm.RenderShortcode("sc-badret", "plugin:sc-badret:numret", "group", 1, json.RawMessage(`{}`), nil)
	assert.Error(t, err, "number return should be an error")

	_, err = pm.RenderShortcode("sc-badret", "plugin:sc-badret:boolret", "group", 1, json.RawMessage(`{}`), nil)
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
