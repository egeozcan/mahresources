package api_handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

type promptPluginProvider struct {
	pm *plugin_system.PluginManager
}

func (p promptPluginProvider) PluginManager() *plugin_system.PluginManager { return p.pm }

// The generation prompt is an authoring reference, not merely an autocomplete
// index. Pin every field from BuiltinDocs so a future attempt to make it
// "compact" cannot silently reduce attributes to names or examples to the
// first entry again.
func TestBuiltinGenerationPromptDocsAreComplete(t *testing.T) {
	docs := shortcodes.BuiltinDocs()
	var all strings.Builder
	for _, doc := range docs {
		writeBuiltinShortcodeDoc(&all, doc)
	}

	if count := strings.Count(all.String(), "(block form: "); count != len(docs) {
		t.Fatalf("prompt contains %d built-in entries, want %d", count, len(docs))
	}
	for _, doc := range docs {
		if strings.TrimSpace(doc.Name) == "" || strings.TrimSpace(doc.Syntax) == "" {
			t.Errorf("built-in shortcode has an empty name or syntax: %#v", doc)
		}
		if strings.TrimSpace(doc.Description) == "" {
			t.Errorf("built-in shortcode %q has no description", doc.Name)
		}
		if len(doc.Examples) == 0 {
			t.Errorf("built-in shortcode %q has no examples", doc.Name)
		}
		var section strings.Builder
		writeBuiltinShortcodeDoc(&section, doc)
		got := section.String()
		if !strings.Contains(got, "- "+doc.Syntax+" (block form: "+string(doc.IsBlock)+")") {
			t.Errorf("prompt missing %q syntax and block capability", doc.Name)
		}
		if !strings.Contains(got, "Description: "+oneLine(doc.Description)) {
			t.Errorf("prompt missing %q description", doc.Name)
		}
		for _, attr := range doc.Attrs {
			if strings.TrimSpace(attr.Name) == "" || strings.TrimSpace(attr.Type) == "" || strings.TrimSpace(attr.Description) == "" {
				t.Errorf("built-in shortcode %q has an incomplete attribute declaration: %#v", doc.Name, attr)
			}
			if !strings.Contains(got, "  - "+attr.Name+" ("+attr.Type) {
				t.Errorf("prompt missing %q attribute %q and its type", doc.Name, attr.Name)
			}
			if !strings.Contains(got, "): "+oneLine(attr.Description)) {
				t.Errorf("prompt missing %q attribute %q description", doc.Name, attr.Name)
			}
			if attr.Default != "" && !strings.Contains(got, "default="+attr.Default) {
				t.Errorf("prompt missing %q attribute %q default %q", doc.Name, attr.Name, attr.Default)
			}
			if len(attr.Enum) > 0 && !strings.Contains(got, "values="+strings.Join(attr.Enum, "|")) {
				t.Errorf("prompt missing %q attribute %q enum", doc.Name, attr.Name)
			}
		}
		for _, example := range doc.Examples {
			if strings.TrimSpace(example.Title) == "" || strings.TrimSpace(example.Code) == "" {
				t.Errorf("built-in shortcode %q has an incomplete example: %#v", doc.Name, example)
			}
			if !strings.Contains(got, oneLine(example.Code)) {
				t.Errorf("prompt missing %q example %q", doc.Name, example.Title)
			}
			if example.Notes != "" && !strings.Contains(got, "Notes: "+oneLine(example.Notes)) {
				t.Errorf("prompt missing notes for %q example %q", doc.Name, example.Title)
			}
		}
	}
}

func TestPluginGenerationPromptDocsAreBoundedByWholeEntry(t *testing.T) {
	docs := []plugin_system.PluginShortcodeInfo{
		{
			FullName:    "plugin:oversized:chip",
			Description: strings.Repeat("x", maxPluginShortcodePromptBytes),
			Examples:    []plugin_system.ShortcodeDocExample{{Title: "Chip", Code: "[plugin:oversized:chip]"}},
		},
		{
			FullName:    "plugin:small:chip",
			Description: "A small chip.",
			Examples:    []plugin_system.ShortcodeDocExample{{Title: "Chip", Code: "[plugin:small:chip]"}},
		},
	}

	var prompt strings.Builder
	omitted := appendPluginShortcodeDocsForPrompt(&prompt, docs)
	if omitted != 1 {
		t.Fatalf("omitted %d plugin docs, want 1", omitted)
	}
	got := prompt.String()
	if strings.Contains(got, "plugin:oversized:chip") {
		t.Error("oversized plugin entry was partially included")
	}
	if !strings.Contains(got, "plugin:small:chip") {
		t.Error("a later complete entry that fit the remaining budget was omitted")
	}
	if len(got) > maxPluginShortcodePromptBytes {
		t.Fatalf("plugin prompt is %d bytes, limit is %d", len(got), maxPluginShortcodePromptBytes)
	}
}

func TestBundledPluginGenerationDocsFitSafetyBudget(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	for _, plugin := range pm.DiscoveredPlugins() {
		if err := pm.EnablePlugin(plugin.Name); err != nil {
			t.Fatalf("enable bundled plugin %q: %v", plugin.Name, err)
		}
	}

	var prompt strings.Builder
	if omitted := appendPluginShortcodeDocsForPrompt(&prompt, pm.AllShortcodeDocs()); omitted != 0 {
		t.Fatalf("%d bundled plugin shortcode docs exceed the generation prompt safety budget", omitted)
	}
	if prompt.Len() > maxPluginShortcodePromptBytes {
		t.Fatalf("bundled plugin docs use %d bytes, limit is %d", prompt.Len(), maxPluginShortcodePromptBytes)
	}
}

// Enabled plugins extend the same request-time catalogue as built-ins. Keep
// their complete declaration intact too: that declaration is the only source
// the generation model has for plugin-specific behavior.
func TestPluginGenerationPromptDocsAreComplete(t *testing.T) {
	doc := plugin_system.PluginShortcodeInfo{
		FullName:    "plugin:charts:trend",
		Name:        "trend",
		PluginName:  "charts",
		Label:       "Trend",
		Description: "Draw a trend for one numeric field.",
		Attrs: []plugin_system.ShortcodeDocAttr{
			{Name: "path", Type: "string", Required: true, Description: "Dot-path to the numeric field"},
			{Name: "points", Type: "number", Default: "12", Description: "Maximum points to draw"},
		},
		Examples: []plugin_system.ShortcodeDocExample{
			{Title: "Default trend", Code: `[plugin:charts:trend path="metrics.value"]`, Notes: "Uses twelve points."},
			{Title: "Short trend", Code: `[plugin:charts:trend path="metrics.value" points="5"]`},
		},
		Notes: []string{"Requires numeric input."},
	}

	var prompt strings.Builder
	writePluginShortcodeDoc(&prompt, doc)
	got := prompt.String()
	for _, want := range []string{
		`[plugin:charts:trend path="…"] (plugin shortcode; block form: optional)`,
		"Description: Draw a trend for one numeric field.",
		"path (string, required): Dot-path to the numeric field",
		"points (number, optional, default=12): Maximum points to draw",
		`Example (Default trend): [plugin:charts:trend path="metrics.value"] | Notes: Uses twelve points.`,
		`Example (Short trend): [plugin:charts:trend path="metrics.value" points="5"]`,
		"Note: Requires numeric input.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plugin prompt docs are missing %q\n%s", want, got)
		}
	}
}

func TestGenerationPromptIncludesEnabledPluginShortcodes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "prompt-extension")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	source := `
plugin = { name = "prompt-extension", version = "1.0" }
function init()
    mah.shortcode({
        name = "chip",
        label = "Chip",
        render = function(ctx) return "<span>chip</span>" end,
        description = "Shows a compact chip.",
        attrs = {
            { name = "tone", type = "string", default = "neutral", description = "Visual tone" },
        },
        examples = {
            { title = "Warning", code = '[plugin:prompt-extension:chip tone="warning"]' },
        },
        notes = { "Use in compact slots." },
    })
end
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("prompt-extension"); err != nil {
		t.Fatal(err)
	}

	got := serializeShortcodeDocsForPrompt(promptPluginProvider{pm: pm})
	for _, want := range []string{
		"[plugin:prompt-extension:chip] (plugin shortcode; block form: optional)",
		"Description: Shows a compact chip.",
		"tone (string, optional, default=neutral): Visual tone",
		`Example (Warning): [plugin:prompt-extension:chip tone="warning"]`,
		"Note: Use in compact slots.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request-time prompt did not include enabled plugin detail %q", want)
		}
	}

	if err := pm.DisablePlugin("prompt-extension"); err != nil {
		t.Fatal(err)
	}
	if got := serializeShortcodeDocsForPrompt(promptPluginProvider{pm: pm}); strings.Contains(got, "plugin:prompt-extension:chip") {
		t.Error("disabled plugin shortcode remained in the request-time prompt")
	}
}

// Active-plugin context is distinct from shortcode documentation: a plugin can
// add a note block without adding any shortcode. The generation model needs
// both the plugin's purpose and the block's concrete usage contract so it can
// design a complementary custom template without trying to emit the block as
// shortcode markup.
func TestGenerationPromptIncludesActivePluginsAndRegisteredBlocks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project-tools")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	source := `
plugin = {
    name = "project-tools",
    version = "2.3",
    description = "Planning features for project notes."
}

function init()
    mah.shortcode({
        name = "sprint-status",
        label = "Sprint status",
        render = function(ctx) return "<span>status</span>" end,
        description = "Shows the current sprint status.",
        attrs = { { name = "tone", type = "string", description = "Status colour" } }
    })
    mah.block_type({
        type = "sprint-plan",
        label = "Sprint plan",
        description = "Tracks a sprint goal and its planned work.",
        content_schema = {
            type = "object",
            properties = { goal = { type = "string" } },
            required = { "goal" }
        },
        state_schema = {
            type = "object",
            properties = { collapsed = { type = "boolean" } }
        },
        default_content = { goal = "Ship the release" },
        default_state = { collapsed = false },
        filters = { note_type_ids = { 7 }, category_ids = { 12 } },
        render_view = function(ctx) return "<div>sprint</div>" end,
        render_edit = function(ctx) return "<div>sprint edit</div>" end
    })
end
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("project-tools"); err != nil {
		t.Fatal(err)
	}

	references := serializeTemplateGenerationPluginReferencesForPrompt(promptPluginProvider{pm: pm})
	if references.DocsBlock == "" {
		t.Fatal("shortcode reference disappeared while building the combined plugin snapshot")
	}
	if _, ok := references.Known["plugin:project-tools:sprint-status"]; !ok {
		t.Fatal("generation linter did not use the same snapshot as its plugin prompt")
	}
	got := references.PluginContext
	for _, want := range []string{
		"Plugin: project-tools (version 2.3)",
		"Description: Planning features for project notes.",
		"plugin:project-tools:sprint-plan (label: Sprint plan)",
		"Description: Tracks a sprint goal and its planned work.",
		"Usage: Add this structured block to a Note with the note block editor; its type is plugin:project-tools:sprint-plan. It is not template shortcode markup.",
		`Default content: {"goal":"Ship the release"}`,
		`Default state: {"collapsed":false}`,
		`Content validation schema: {"properties":{"goal":{"type":"string"}},"required":["goal"],"type":"object"}`,
		`State validation schema: {"properties":{"collapsed":{"type":"boolean"}},"type":"object"}`,
		"Availability: requires note type IDs 7 and owning-group category IDs 12.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("active-plugin context missing %q\n%s", want, got)
		}
	}

	if err := pm.DisablePlugin("project-tools"); err != nil {
		t.Fatal(err)
	}
	if got := serializePluginContextForPrompt(promptPluginProvider{pm: pm}); got != "" {
		t.Errorf("disabled plugin remained in generation context: %q", got)
	}
}

func TestPluginBlockJSONPreservesWhitespaceInStringValues(t *testing.T) {
	var prompt strings.Builder
	writePluginBlockJSON(&prompt, "Default content", json.RawMessage(`{
  "pattern": "keep  two   spaces",
  "title": "Release plan"
}`))

	got := prompt.String()
	if !strings.Contains(got, `{"pattern":"keep  two   spaces","title":"Release plan"}`) {
		t.Fatalf("block JSON string values were altered: %q", got)
	}
}
