package plugin_system

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/models"
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

func TestBlockDocsUseTheirOwnPathAndRenderDefaults(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "block-docs", `
		plugin = { api_version = 1, name = "block-docs", version = "1.0", capabilities = { "render", "db:read" } }
		function init()
			mah.shortcode({
				name = "card", label = "Card shortcode", render = function() return "shortcode" end,
				description = "A shortcode with the same short name as the block.",
			})
			mah.block_type({
				type = "card", label = "Card block", icon = "C",
				description = "A rendered card block.",
				content_schema = {
					type = "object", required = { "message" },
					properties = { message = { type = "string", maxLength = 100 } },
				},
				default_content = { message = "Default block content" },
				default_state = { expanded = true },
				filters = { note_type_ids = { 7 }, category_ids = { 12 } },
				render_view = function(ctx)
					if not ctx.preview or not ctx.read_only or ctx.can_write then return "<article>UNSAFE_CONTEXT</article>" end
					return "<article>" .. mah.html_escape(ctx.block.content.message) .. "</article>"
				end,
				render_edit = function() return "<input>" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("block-docs"))

	assert.True(t, pm.HasPage("block-docs", "docs"))
	assert.True(t, pm.HasPage("block-docs", "docs/card"))
	assert.True(t, pm.HasPage("block-docs", "docs/blocks/card"))

	index, err := pm.HandleDocsPage(context.Background(), "block-docs", "docs")
	require.NoError(t, err)
	assert.Contains(t, index, `/plugins/block-docs/docs/card`)
	assert.Contains(t, index, `/plugins/block-docs/docs/blocks/card`)
	assert.Contains(t, index, `plugin:block-docs:card`)
	assert.Contains(t, index, `2 items`)

	pm.SetEntityQuerier(&mockQuerier{})
	writableDocsRequest := auth.WithPrincipal(context.Background(), &auth.Principal{Role: models.RoleEditor})
	html, err := pm.HandleDocsPage(writableDocsRequest, "block-docs", "docs/blocks/card")
	require.NoError(t, err)
	assert.Contains(t, html, `Card block`)
	assert.Contains(t, html, `plugin:block-docs:card`)
	assert.Contains(t, html, `Default block content`)
	assert.Contains(t, html, `&#34;expanded&#34;: true`)
	assert.Contains(t, html, `Validation`)
	assert.Contains(t, html, `&#34;maxLength&#34;: 100`)
	assert.Contains(t, html, `Note type IDs:`)
	assert.Contains(t, html, `Owning-group category IDs:`)
	assert.Contains(t, html, `<article>Default block content</article>`)
	assert.NotContains(t, html, `UNSAFE_CONTEXT`)
	assert.Contains(t, html, `interactive controls are disabled`)
}

func TestBlockDocsOmitPreviewThatRequestsHostData(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "host-data-block", `
		plugin = { api_version = 1, name = "host-data-block", version = "1.0", capabilities = { "render", "db:read" } }
		function init()
			mah.block_type({
				type = "live-data", label = "Live data", description = "Needs host data.",
				default_content = {}, default_state = {},
				render_view = function()
					local note = mah.db.get_note(1)
					local secret = mah.get_setting("api_key")
					return "<article>fallback " .. tostring(note) .. " " .. tostring(secret) .. "</article>"
				end,
				render_edit = function() return "" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	pm.SetEntityQuerier(&mockQuerier{})
	pm.SetPluginSettings("host-data-block", map[string]any{"api_key": "secret-value"})
	require.NoError(t, pm.EnablePlugin("host-data-block"))

	writableDocsRequest := auth.WithPrincipal(context.Background(), &auth.Principal{Role: models.RoleEditor})
	html, err := pm.HandleDocsPage(writableDocsRequest, "host-data-block", "docs/blocks/live-data")
	require.NoError(t, err)
	assert.Contains(t, html, `Needs host data.`)
	assert.NotContains(t, html, `>Preview</h2>`)
	assert.NotContains(t, html, `fallback`)
	assert.NotContains(t, html, `secret-value`)
}

func TestBlockDocsKeepReferenceWhenPreviewCannotRender(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "unpreviewable-block", `
		plugin = { api_version = 1, name = "unpreviewable-block", version = "1.0", capabilities = { "render" } }
		function init()
			mah.block_type({
				type = "live-data", label = "Live data", description = "Needs a persisted record.",
				default_content = {}, default_state = {},
				render_view = function() return nil end,
				render_edit = function() return "" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("unpreviewable-block"))

	html, err := pm.HandleDocsPage(context.Background(), "unpreviewable-block", "docs/blocks/live-data")
	require.NoError(t, err)
	assert.Contains(t, html, `Needs a persisted record.`)
	assert.Contains(t, html, `Defaults`)
	assert.NotContains(t, html, `>Preview</h2>`)
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

	html, err := pm.HandleDocsPage(context.Background(), "indexed", "docs")
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

	html, err := pm.HandleDocsPage(context.Background(), "detailed", "docs/widget")
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
	html, err := pm.HandleDocsPage(context.Background(), "nav-test", "docs/first")
	require.NoError(t, err)
	assert.Contains(t, html, "Middle")
	assert.NotContains(t, html, "&larr;")

	// Middle page: has both
	html, err = pm.HandleDocsPage(context.Background(), "nav-test", "docs/middle")
	require.NoError(t, err)
	assert.Contains(t, html, "First")
	assert.Contains(t, html, "Last")

	// Last page: has prev, no next
	html, err = pm.HandleDocsPage(context.Background(), "nav-test", "docs/last")
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

	html, err := pm.HandleDocsPage(context.Background(), "gen-detail", "docs/upscale")
	require.NoError(t, err)

	assert.Contains(t, html, "Upscale Action")
	assert.Contains(t, html, "Increase image resolution.")
	assert.Contains(t, html, "Action")     // category badge
	assert.Contains(t, html, "Parameters") // non-shortcode uses "Parameters" not "Attributes"
	assert.Contains(t, html, "model")
	assert.Contains(t, html, "Results are added as a new version")
	assert.NotContains(t, html, "[plugin:") // no shortcode syntax snippet
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

	html, err := pm.HandleDocsPage(context.Background(), "mixed", "docs")
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

func TestExampleDataParsing(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ex-data", `
		plugin = { name = "ex-data", version = "1.0" }
		function init()
			mah.shortcode({
				name = "badge",
				label = "Badge",
				render = function(ctx)
					local val = ctx.value and ctx.value["status"] or "unknown"
					return "<span>" .. val .. "</span>"
				end,
				description = "A test badge.",
				examples = {
					{
						title = "With data",
						code = '[plugin:ex-data:badge path="status"]',
						example_data = { status = "active", nested = { key = "val" } },
					},
					{
						title = "No data",
						code = '[plugin:ex-data:badge path="status"]',
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("ex-data"))

	sc := pm.GetPluginShortcode("plugin:ex-data:badge")
	require.NotNil(t, sc)
	require.Len(t, sc.Examples, 2)

	assert.Equal(t, map[string]any{"status": "active", "nested": map[string]any{"key": "val"}}, sc.Examples[0].ExampleData)
	assert.Nil(t, sc.Examples[1].ExampleData)
}

func TestDocsDetailWithPreview(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "preview", `
		plugin = { name = "preview", version = "1.0" }
		function init()
			mah.shortcode({
				name = "badge",
				label = "Badge",
				render = function(ctx)
					local val = ctx.value and ctx.value["status"] or "unknown"
					return "<span class='badge'>status:" .. val .. "</span>"
				end,
				description = "Badge with preview.",
				examples = {
					{
						title = "With preview",
						code = '[plugin:preview:badge path="status"]',
						example_data = { status = "active" },
					},
					{
						title = "Code only",
						code = '[plugin:preview:badge path="status"]',
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("preview"))

	html, err := pm.HandleDocsPage(context.Background(), "preview", "docs/badge")
	require.NoError(t, err)

	// Preview rendered output should appear for the first example
	assert.Contains(t, html, "status:active")
	assert.Contains(t, html, "Preview")

	// Code blocks should still be present for both examples
	assert.Contains(t, html, "With preview")
	assert.Contains(t, html, "Code only")

	// The second example (no example_data) should NOT have a preview
	// Count occurrences of "Preview" label — should be exactly 1
	assert.Equal(t, 1, strings.Count(html, ">Preview<"))
}

func TestDocsPreviewRenderError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "err-preview", `
		plugin = { name = "err-preview", version = "1.0" }
		function init()
			mah.shortcode({
				name = "boom",
				label = "Boom",
				render = function(ctx) error("intentional error") end,
				description = "A shortcode that errors.",
				examples = {
					{
						title = "Should not crash",
						code = '[plugin:err-preview:boom]',
						example_data = { anything = true },
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("err-preview"))

	html, err := pm.HandleDocsPage(context.Background(), "err-preview", "docs/boom")
	require.NoError(t, err)

	// Page should render successfully, just without preview
	assert.Contains(t, html, "Should not crash")
	assert.NotContains(t, html, ">Preview<")
}

func TestDocsPreviewEmptyExampleData(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "empty-data", `
		plugin = { name = "empty-data", version = "1.0" }
		function init()
			mah.shortcode({
				name = "widget",
				label = "Widget",
				render = function(ctx)
					return "<div class='widget'>rendered</div>"
				end,
				description = "A widget.",
				examples = {
					{
						title = "Empty data preview",
						code = '[plugin:empty-data:widget]',
						example_data = {},
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("empty-data"))

	html, err := pm.HandleDocsPage(context.Background(), "empty-data", "docs/widget")
	require.NoError(t, err)

	// Empty example_data (non-nil) should still trigger preview
	assert.Contains(t, html, ">Preview<")
	assert.Contains(t, html, "rendered")
}

func TestDocsPreviewContextHasPreviewFlag(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "flag-check", `
		plugin = { name = "flag-check", version = "1.0" }
		function init()
			mah.shortcode({
				name = "check",
				label = "Check",
				render = function(ctx)
					if ctx.preview then
						return "<span>PREVIEW_MODE</span>"
					end
					return "<span>NORMAL_MODE</span>"
				end,
				description = "Checks preview flag.",
				examples = {
					{
						title = "Flag test",
						code = '[plugin:flag-check:check]',
						example_data = {},
					},
				},
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("flag-check"))

	html, err := pm.HandleDocsPage(context.Background(), "flag-check", "docs/check")
	require.NoError(t, err)

	assert.Contains(t, html, "PREVIEW_MODE")
	assert.NotContains(t, html, "NORMAL_MODE")
}

func TestDocsPreviewSkipsMismatchedCode(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "mismatch", `
		plugin = { name = "mismatch", version = "1.0" }
		function init()
			mah.shortcode({
				name = "alpha",
				label = "Alpha",
				render = function(ctx) return "<span>alpha</span>" end,
				description = "Alpha shortcode.",
				examples = {
					{
						title = "Mismatched code",
						code = '[plugin:mismatch:beta path="x"]',
						example_data = { x = "test" },
					},
				},
			})
			mah.shortcode({
				name = "beta",
				label = "Beta",
				render = function(ctx) return "<span>beta</span>" end,
				description = "Beta shortcode.",
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("mismatch"))

	html, err := pm.HandleDocsPage(context.Background(), "mismatch", "docs/alpha")
	require.NoError(t, err)

	// Alpha's example has code referencing beta — preview should be skipped
	assert.NotContains(t, html, ">Preview<")
	// But the code block should still be shown
	assert.Contains(t, html, "Mismatched code")
}

func TestAllShortcodeDocs(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-list", `
		plugin = { name = "sc-list", version = "1.0" }
		function init()
			mah.shortcode({
				name = "widget",
				label = "Widget",
				render = function(ctx) return "<span>w</span>" end,
				description = "A widget.",
				attrs = {
					{ name = "size", type = "string", required = true, description = "Widget size" },
				},
			})
			mah.shortcode({
				name = "undocumented",
				label = "Undocumented",
				render = function(ctx) return "<span>u</span>" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	// Before enabling, no shortcodes are registered.
	assert.Empty(t, pm.AllShortcodeDocs())

	require.NoError(t, pm.EnablePlugin("sc-list"))

	docs := pm.AllShortcodeDocs()
	require.Len(t, docs, 2)

	// Sorted by full type name: undocumented < widget.
	assert.Equal(t, "plugin:sc-list:undocumented", docs[0].FullName)
	assert.Equal(t, "undocumented", docs[0].Name)

	widget := docs[1]
	assert.Equal(t, "plugin:sc-list:widget", widget.FullName)
	assert.Equal(t, "widget", widget.Name)
	assert.Equal(t, "sc-list", widget.PluginName)
	assert.Equal(t, "A widget.", widget.Description)
	require.Len(t, widget.Attrs, 1)
	assert.Equal(t, "size", widget.Attrs[0].Name)
	assert.True(t, widget.Attrs[0].Required)

	// Disabling removes the plugin's shortcodes from the catalogue.
	require.NoError(t, pm.DisablePlugin("sc-list"))
	assert.Empty(t, pm.AllShortcodeDocs())
}

func TestAuthoringSnapshotKeepsPluginShortcodesAndBlocksTogether(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "authoring", `
		plugin = { name = "authoring", version = "1.0", description = "Authoring tools." }
		function init()
			mah.shortcode({
				name = "status",
				label = "Status",
				render = function(ctx) return "<span>status</span>" end,
				description = "Shows a status.",
			})
			mah.block_type({
				type = "plan",
				label = "Plan",
				description = "A project plan.",
				content_schema = { type = "object", properties = { title = { type = "string" } } },
				default_content = { title = "Ship" },
				filters = { note_type_ids = { 9 } },
				render_view = function(ctx) return "<div>plan</div>" end,
				render_edit = function(ctx) return "<div>plan edit</div>" end,
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("authoring"))

	snapshot := pm.AuthoringSnapshot()
	require.Len(t, snapshot.Plugins, 1)
	assert.Equal(t, "authoring", snapshot.Plugins[0].Name)
	require.Len(t, snapshot.Shortcodes, 1)
	assert.Equal(t, "plugin:authoring:status", snapshot.Shortcodes[0].FullName)
	require.Len(t, snapshot.Blocks, 1)
	block := snapshot.Blocks[0]
	assert.Equal(t, "plugin:authoring:plan", block.TypeName)
	assert.JSONEq(t, `{"title":"Ship"}`, string(block.DefaultContent))
	assert.JSONEq(t, `{"type":"object","properties":{"title":{"type":"string"}}}`, string(block.ContentSchema))
	assert.Equal(t, []uint{9}, block.Filters.NoteTypeIDs)

	require.NoError(t, pm.DisablePlugin("authoring"))
	snapshot = pm.AuthoringSnapshot()
	assert.Empty(t, snapshot.Plugins)
	assert.Empty(t, snapshot.Shortcodes)
	assert.Empty(t, snapshot.Blocks)
}

func TestAuthoringSnapshotExcludesRegistrationsUntilPluginPublished(t *testing.T) {
	pm := &PluginManager{
		shortcodes: map[string][]*PluginShortcode{
			"loading": {{PluginName: "loading", TypeName: "plugin:loading:status"}},
		},
		blockTypes: map[string][]*PluginBlockType{
			"loading": {{PluginName: "loading", TypeName: "plugin:loading:plan"}},
		},
	}

	// init() may register capabilities before loadPlugin publishes its
	// PluginInfo. That intermediate state is not active and must never reach an
	// authoring prompt.
	snapshot := pm.AuthoringSnapshot()
	assert.Empty(t, snapshot.Plugins)
	assert.Empty(t, snapshot.Shortcodes)
	assert.Empty(t, snapshot.Blocks)

	pm.plugins = []PluginInfo{{Name: "loading", Version: "1.0"}}
	snapshot = pm.AuthoringSnapshot()
	require.Len(t, snapshot.Plugins, 1)
	require.Len(t, snapshot.Shortcodes, 1)
	require.Len(t, snapshot.Blocks, 1)
}
