package plugin_system

import (
	"context"
	"encoding/json"
	"testing"

	"mahresources/shortcodes"

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

func TestDocsPreviewBlockShortcodeNoNestedExpansion(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-preview", `
		plugin = { name = "sc-preview", version = "1.0" }
		function init()
			mah.shortcode({
				name = "echo",
				label = "Echo",
				description = "Echoes inner content",
				render = function(ctx)
					return ctx.inner_content or ""
				end,
				examples = {
					{
						title = "With nested shortcode",
						code = '[plugin:sc-preview:echo]has [meta path="x"] inside[/plugin:sc-preview:echo]',
						example_data = { x = "val" },
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-preview"))

	// Docs preview: nested shortcodes should render as literal text, NOT expanded
	pm.mu.RLock()
	items := pm.collectDocItems("sc-preview")
	pm.mu.RUnlock()

	require.Len(t, items, 1)
	require.Len(t, items[0].Examples, 1)

	preview := renderExamplePreview(context.Background(), pm, "sc-preview", "plugin:sc-preview:echo", items[0].Examples[0])
	assert.Contains(t, preview, `[meta path="x"]`)
	assert.NotContains(t, preview, "<meta-shortcode")
}

// mah.html_escape is the one escaping helper the platform offers, and a plugin's
// output is re-processed as shortcode source, so it has to neutralise the
// shortcode brackets along with the HTML metacharacters. Every bundled plugin
// carries its own copy of the helper, and bundled_plugin_output_test.go holds
// those; this holds the platform's own, which is what a third-party plugin
// following the documented pattern relies on.
func TestHtmlEscapeNeutralisesShortcodeSyntax(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-escape", `
		plugin = { name = "sc-escape", version = "1.0" }
		function init()
			mah.shortcode({
				name = "echo",
				label = "Echo",
				render = function(ctx)
					return "<span>" .. mah.html_escape(tostring(ctx.value.status)) .. "</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-escape"))

	// The meta value is written by whoever may edit the entity, which under
	// -auth includes the `user` role; the entity field it reaches for is what
	// that account would be reading back out on somebody else's page.
	const injected = "INJECTED-FROM-A-VALUE-A-USER-TYPED"
	ctx := shortcodes.MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"[property path=\"Name\" raw=\"true\"]"}`),
		Entity:     &testResourceEntity{Name: injected},
	}

	out := shortcodes.Process(context.Background(),
		`[plugin:sc-escape:echo]`, ctx, bundledRenderer(pm), nil)

	assert.NotContains(t, out, injected, "the escaped value ran as a shortcode: %s", out)
	// Still printed, only inert. The entities read as the characters
	// themselves, so the page shows what was typed.
	assert.Contains(t, out, "&#91;property", "the value stopped being printed: %s", out)
}
