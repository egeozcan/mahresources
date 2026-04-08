package plugin_system

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortcodeDocParsing(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "doc-test", `
		plugin = { name = "doc-test", version = "1.0" }
		function init()
			mah.shortcode({
				name = "badge",
				label = "Status Badge",
				render = function(ctx) return "<span>badge</span>" end,
				description = "Display a colored badge.",
				attrs = {
					{ name = "path", type = "string", required = true, description = "Dot-path to meta field" },
					{ name = "colors", type = "CSV", default = "#gray", description = "Hex colors" },
				},
				examples = {
					{ title = "Basic", code = '[plugin:doc-test:badge path="status"]', notes = "Shows raw value." },
					{ title = "With colors", code = '[plugin:doc-test:badge path="status" colors="#22c55e"]' },
				},
				notes = { "Gray badge for unmatched values.", "Supports dot-path navigation." },
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("doc-test"))

	sc := pm.GetPluginShortcode("plugin:doc-test:badge")
	require.NotNil(t, sc)

	assert.Equal(t, "Display a colored badge.", sc.Description)

	require.Len(t, sc.Attrs, 2)
	assert.Equal(t, "path", sc.Attrs[0].Name)
	assert.Equal(t, "string", sc.Attrs[0].Type)
	assert.True(t, sc.Attrs[0].Required)
	assert.Equal(t, "", sc.Attrs[0].Default)
	assert.Equal(t, "Dot-path to meta field", sc.Attrs[0].Description)

	assert.Equal(t, "colors", sc.Attrs[1].Name)
	assert.Equal(t, "CSV", sc.Attrs[1].Type)
	assert.False(t, sc.Attrs[1].Required)
	assert.Equal(t, "#gray", sc.Attrs[1].Default)

	require.Len(t, sc.Examples, 2)
	assert.Equal(t, "Basic", sc.Examples[0].Title)
	assert.Contains(t, sc.Examples[0].Code, "path=\"status\"")
	assert.Equal(t, "Shows raw value.", sc.Examples[0].Notes)
	assert.Equal(t, "With colors", sc.Examples[1].Title)
	assert.Empty(t, sc.Examples[1].Notes)

	require.Len(t, sc.Notes, 2)
	assert.Equal(t, "Gray badge for unmatched values.", sc.Notes[0])
}

func TestShortcodeDocParsingOptional(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nodoc", `
		plugin = { name = "nodoc", version = "1.0" }
		function init()
			mah.shortcode({
				name = "plain",
				label = "Plain",
				render = function(ctx) return "ok" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("nodoc"))

	sc := pm.GetPluginShortcode("plugin:nodoc:plain")
	require.NotNil(t, sc)
	assert.Empty(t, sc.Description)
	assert.Nil(t, sc.Attrs)
	assert.Nil(t, sc.Examples)
	assert.Nil(t, sc.Notes)
}

func TestHasDocsPage(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "with-docs", `
		plugin = { name = "with-docs", version = "1.0" }
		function init()
			mah.shortcode({
				name = "foo",
				label = "Foo",
				render = function(ctx) return "foo" end,
				description = "A foo component.",
			})
			mah.shortcode({
				name = "bar",
				label = "Bar",
				render = function(ctx) return "bar" end,
				description = "A bar component.",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("with-docs"))

	assert.True(t, pm.HasPage("with-docs", "docs"))
	assert.True(t, pm.HasPage("with-docs", "docs/foo"))
	assert.True(t, pm.HasPage("with-docs", "docs/bar"))
	assert.False(t, pm.HasPage("with-docs", "docs/unknown"))
	assert.False(t, pm.HasPage("with-docs", "docs/"))
	assert.False(t, pm.HasPage("unknown-plugin", "docs"))
}

func TestHasDocsPageUndocumented(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "no-docs", `
		plugin = { name = "no-docs", version = "1.0" }
		function init()
			mah.shortcode({
				name = "plain",
				label = "Plain",
				render = function(ctx) return "ok" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("no-docs"))

	assert.False(t, pm.HasPage("no-docs", "docs"))
	assert.False(t, pm.HasPage("no-docs", "docs/plain"))
}

func TestHandleDocsIndex(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "indexed", `
		plugin = { name = "indexed", version = "1.0" }
		function init()
			mah.shortcode({
				name = "alpha",
				label = "Alpha Widget",
				render = function(ctx) return "a" end,
				description = "The alpha component.",
				attrs = {
					{ name = "size", type = "number", description = "Size in pixels" },
				},
				examples = {
					{ title = "Basic", code = '[plugin:indexed:alpha size="10"]' },
				},
			})
			mah.shortcode({
				name = "beta",
				label = "Beta Widget",
				render = function(ctx) return "b" end,
				description = "The beta component.",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("indexed"))

	html, err := pm.HandleDocsPage("indexed", "docs")
	require.NoError(t, err)

	assert.Contains(t, html, "indexed Documentation")
	assert.Contains(t, html, "2 items")
	assert.Contains(t, html, "Alpha Widget")
	assert.Contains(t, html, "Beta Widget")
	assert.Contains(t, html, "/plugins/indexed/docs/alpha")
	assert.Contains(t, html, "/plugins/indexed/docs/beta")
	assert.Contains(t, html, "1 attributes")
	assert.Contains(t, html, "1 examples")
}

func TestHandleDocsDetail(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "detailed", `
		plugin = { name = "detailed", version = "1.0" }
		function init()
			mah.shortcode({
				name = "widget",
				label = "Test Widget",
				render = function(ctx) return "w" end,
				description = "A test widget for docs.",
				attrs = {
					{ name = "path", type = "string", required = true, description = "Meta field path" },
					{ name = "max", type = "number", default = "100", description = "Maximum value" },
				},
				examples = {
					{ title = "Simple usage", code = '[plugin:detailed:widget path="score"]', notes = "Uses default max of 100." },
				},
				notes = { "Supports nested dot paths.", "Returns empty string if path not found." },
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("detailed"))

	html, err := pm.HandleDocsPage("detailed", "docs/widget")
	require.NoError(t, err)

	// Breadcrumb
	assert.Contains(t, html, `detailed Docs</a>`)
	// Header
	assert.Contains(t, html, "Test Widget")
	assert.Contains(t, html, "A test widget for docs.")
	// Syntax snippet
	assert.Contains(t, html, `[plugin:detailed:widget path="…"]`)
	// Attributes table
	assert.Contains(t, html, ">path<")
	assert.Contains(t, html, ">string<")
	assert.Contains(t, html, ">max<")
	assert.Contains(t, html, ">number<")
	assert.Contains(t, html, "100")
	// Required indicator
	assert.True(t, strings.Contains(html, "Required"))
	// Examples
	assert.Contains(t, html, "Simple usage")
	assert.Contains(t, html, `[plugin:detailed:widget path=&#34;score&#34;]`)
	assert.Contains(t, html, "Uses default max of 100.")
	// Notes
	assert.Contains(t, html, "Supports nested dot paths.")
	assert.Contains(t, html, "Returns empty string if path not found.")
}

func TestDocsCleanupOnDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cleanup-docs", `
		plugin = { name = "cleanup-docs", version = "1.0" }
		function init()
			mah.shortcode({
				name = "temp",
				label = "Temp",
				render = function(ctx) return "t" end,
				description = "Temporary.",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("cleanup-docs"))
	assert.True(t, pm.HasPage("cleanup-docs", "docs"))
	assert.True(t, pm.HasPage("cleanup-docs", "docs/temp"))

	require.NoError(t, pm.DisablePlugin("cleanup-docs"))
	assert.False(t, pm.HasPage("cleanup-docs", "docs"))
	assert.False(t, pm.HasPage("cleanup-docs", "docs/temp"))
}

func TestPluginHasDocs(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "has-docs", `
		plugin = { name = "has-docs", version = "1.0" }
		function init()
			mah.shortcode({
				name = "widget",
				label = "Widget",
				render = function(ctx) return "w" end,
				description = "A widget.",
			})
		end
	`)
	writePlugin(t, dir, "no-docs", `
		plugin = { name = "no-docs", version = "1.0" }
		function init()
			mah.shortcode({
				name = "plain",
				label = "Plain",
				render = function(ctx) return "ok" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("has-docs"))
	require.NoError(t, pm.EnablePlugin("no-docs"))

	assert.True(t, pm.PluginHasDocs("has-docs"))
	assert.False(t, pm.PluginHasDocs("no-docs"))
	assert.False(t, pm.PluginHasDocs("nonexistent"))
}

func TestDocsPrevNextNavigation(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nav-test", `
		plugin = { name = "nav-test", version = "1.0" }
		function init()
			mah.shortcode({
				name = "first",
				label = "First",
				render = function(ctx) return "1" end,
				description = "First component.",
			})
			mah.shortcode({
				name = "middle",
				label = "Middle",
				render = function(ctx) return "2" end,
				description = "Middle component.",
			})
			mah.shortcode({
				name = "last",
				label = "Last",
				render = function(ctx) return "3" end,
				description = "Last component.",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("nav-test"))

	// First page: no prev, has next
	html, err := pm.HandleDocsPage("nav-test", "docs/first")
	require.NoError(t, err)
	assert.Contains(t, html, "Middle")
	assert.NotContains(t, html, "&larr;")

	// Middle page: has both
	html, err = pm.HandleDocsPage("nav-test", "docs/middle")
	require.NoError(t, err)
	assert.Contains(t, html, "First")
	assert.Contains(t, html, "Last")

	// Last page: has prev, no next
	html, err = pm.HandleDocsPage("nav-test", "docs/last")
	require.NoError(t, err)
	assert.Contains(t, html, "Middle")
	assert.NotContains(t, html, "&rarr;")
}

func TestGeneralDocRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "gen-doc", `
		plugin = { name = "gen-doc", version = "1.0" }
		function init()
			mah.doc({
				name = "colorize",
				label = "Colorize Action",
				description = "Colorize a black and white image using AI.",
				category = "Action",
				attrs = {
					{ name = "model", type = "select", default = "ddcolor", description = "AI model" },
				},
				examples = {
					{ title = "Usage", code = "Click the Colorize button on any image resource" },
				},
				notes = { "Supported: PNG, JPEG, WebP" },
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("gen-doc"))

	assert.True(t, pm.PluginHasDocs("gen-doc"))
	assert.True(t, pm.HasDocsPage("gen-doc", "docs"))
	assert.True(t, pm.HasDocsPage("gen-doc", "docs/colorize"))
	assert.False(t, pm.HasDocsPage("gen-doc", "docs/unknown"))
}

func TestGeneralDocDetailPage(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "gen-detail", `
		plugin = { name = "gen-detail", version = "1.0" }
		function init()
			mah.doc({
				name = "upscale",
				label = "Upscale Action",
				description = "Increase image resolution.",
				category = "Action",
				attrs = {
					{ name = "model", type = "select", default = "clarity", description = "Upscale model to use" },
				},
				notes = { "Results are added as a new version" },
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("gen-detail"))

	html, err := pm.HandleDocsPage("gen-detail", "docs/upscale")
	require.NoError(t, err)

	assert.Contains(t, html, "Upscale Action")
	assert.Contains(t, html, "Increase image resolution.")
	assert.Contains(t, html, "Action")              // category badge
	assert.Contains(t, html, "Parameters")           // non-shortcode uses "Parameters" not "Attributes"
	assert.Contains(t, html, "model")
	assert.Contains(t, html, "Results are added as a new version")
	assert.NotContains(t, html, "[plugin:")          // no shortcode syntax snippet
}

func TestMixedDocsIndex(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "mixed", `
		plugin = { name = "mixed", version = "1.0" }
		function init()
			mah.shortcode({
				name = "badge",
				label = "Status Badge",
				render = function(ctx) return "<span>badge</span>" end,
				description = "Display a colored badge.",
			})
			mah.doc({
				name = "colorize",
				label = "Colorize",
				description = "Colorize images using AI.",
				category = "Action",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("mixed"))

	html, err := pm.HandleDocsPage("mixed", "docs")
	require.NoError(t, err)

	assert.Contains(t, html, "2 items")
	assert.Contains(t, html, "Status Badge")
	assert.Contains(t, html, "Colorize")
	assert.Contains(t, html, "Shortcode Reference") // shortcodes get quick ref
	assert.Contains(t, html, "[plugin:mixed:badge]")
	assert.Contains(t, html, "/plugins/mixed/docs/badge")
	assert.Contains(t, html, "/plugins/mixed/docs/colorize")
}

func TestDocDuplicateNameRejected(t *testing.T) {
	dir := t.TempDir()
	// Duplicate doc name
	writePlugin(t, dir, "dup-doc", `
		plugin = { name = "dup-doc", version = "1.0" }
		function init()
			mah.doc({ name = "feat", label = "Feature", description = "First." })
			mah.doc({ name = "feat", label = "Feature 2", description = "Second." })
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	err = pm.EnablePlugin("dup-doc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate doc entry")
}

func TestDocNameConflictsWithShortcode(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "conflict", `
		plugin = { name = "conflict", version = "1.0" }
		function init()
			mah.shortcode({
				name = "badge",
				label = "Badge Shortcode",
				render = function(ctx) return "b" end,
				description = "A badge.",
			})
			mah.doc({ name = "badge", label = "Badge Doc", description = "Conflict." })
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	err = pm.EnablePlugin("conflict")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts with shortcode")
}

func TestDocsCleanupOnDisableGeneral(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cleanup-gen", `
		plugin = { name = "cleanup-gen", version = "1.0" }
		function init()
			mah.doc({ name = "feat", label = "Feature", description = "A feature.", category = "Action" })
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("cleanup-gen"))

	assert.True(t, pm.PluginHasDocs("cleanup-gen"))
	assert.True(t, pm.HasDocsPage("cleanup-gen", "docs"))
	assert.True(t, pm.HasDocsPage("cleanup-gen", "docs/feat"))

	require.NoError(t, pm.DisablePlugin("cleanup-gen"))

	assert.False(t, pm.PluginHasDocs("cleanup-gen"))
	assert.False(t, pm.HasDocsPage("cleanup-gen", "docs"))
	assert.False(t, pm.HasDocsPage("cleanup-gen", "docs/feat"))
}
