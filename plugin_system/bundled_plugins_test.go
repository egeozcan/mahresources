package plugin_system

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The plugins shipped in ../plugins are only ever executed by a running server,
// and a plugin whose Lua fails to parse is skipped with a log line rather than an
// error — so a typo in one of them ships silently. These tests run the real
// discovery and load path over the bundled directory.

func bundledPluginDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "plugins"))
	if err != nil {
		t.Fatalf("resolving plugin dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("bundled plugin dir not readable: %v", err)
	}
	return dir
}

func TestBundledPluginsLoad(t *testing.T) {
	dir := bundledPluginDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading plugin dir: %v", err)
	}
	var want []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "plugin.lua")); err == nil {
			want = append(want, e.Name())
		}
	}
	if len(want) == 0 {
		t.Fatal("no bundled plugins found")
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	discovered := map[string]bool{}
	for _, dp := range pm.DiscoveredPlugins() {
		discovered[dp.Name] = true
	}

	for _, name := range want {
		if !discovered[name] {
			t.Errorf("plugin %q was not discovered (its plugin.lua failed to parse or declared no plugin table)", name)
			continue
		}
		// init() runs on enable; a bad action/page/doc registration surfaces here.
		if err := pm.EnablePlugin(name); err != nil {
			t.Errorf("EnablePlugin(%q): %v", name, err)
		}
	}
}

// The fal.ai plugin is the largest bundled plugin and the one whose model list
// changes most often. Pin the shape its UI depends on: the exact selector lists,
// their configurable controls, and the few deliberately fixed/shared cases.
func TestFalAIPluginRegistersModels(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("fal-ai"); err != nil {
		t.Fatalf("EnablePlugin(fal-ai): %v", err)
	}

	actions := map[string]ActionRegistration{}
	for _, a := range pm.GetActions("resource", nil) {
		if a.PluginName == "fal-ai" {
			actions[a.ID] = a
		}
	}

	for _, id := range []string{"colorize", "adjust", "upscale", "restore", "edit", "vectorize", "polish"} {
		if _, ok := actions[id]; !ok {
			t.Errorf("action %q not registered", id)
		}
	}

	// fal.ai has many similarly named controls whose meaning changes by model.
	// Keep the user-facing help complete as the live schemas evolve.
	for actionID, action := range actions {
		for _, p := range action.Params {
			if p.Description == "" {
				t.Errorf("action %q param %q has no inline description", actionID, p.Name)
			}
		}
	}

	// Each configurable model must have at least one non-info parameter gated on
	// it. Static help does not prove that a request option was actually wired.
	// Models with a fixed payload or only shared controls are named explicitly so
	// a newly added, accidentally empty model cannot silently weaken this check.
	modelsWithoutOwnControls := map[string]map[string]bool{
		// DDColor has no endpoint controls; Save Result As is shared.
		"colorize": {"ddcolor": true},
		// Both Topaz Adjust presets share output_format and differ only by the
		// endpoint's model value selected in build_request.
		"adjust": {"adjust_v2": true, "white_balance": true},
		// Topaz Transparent is deliberately a fixed 4x PNG operation.
		"upscale": {"topaz_transparent": true},
	}
	for actionID, wantModels := range map[string][]string{
		"colorize": {"ddcolor", "topaz_colorize"},
		"adjust":   {"adjust_v2", "white_balance"},
		"upscale":  {"clarity", "crystal", "esrgan", "creative", "seedvr", "bria_creative", "topaz", "topaz_generative", "topaz_creative", "topaz_transparent", "drct", "aura_sr"},
		"restore":  {"photo_restoration", "codeformer", "swin2sr", "nafnet_denoise", "nafnet_deblur", "topaz_restore", "topaz_denoise"},
		"edit":     {"flux2", "flux2pro", "nanobanana2", "nanobananapro", "nanobanana_lite", "gptimage2", "seedream5", "grok2", "muse", "fibo15", "flux1dev"},
		"polish":   {"post_processing", "topaz_sharpen"},
	} {
		action, ok := actions[actionID]
		if !ok {
			continue // already reported above
		}
		var selector *ActionParam
		gated := map[string]bool{}
		for i := range action.Params {
			p := &action.Params[i]
			if p.Name == "model" {
				selector = p
			}
			if p.Type == "info" {
				continue
			}
			for _, v := range showWhenValues(p, "model") {
				gated[v] = true
			}
		}
		if selector == nil {
			t.Errorf("action %q has no model selector", actionID)
			continue
		}
		if len(selector.Options) != len(wantModels) {
			t.Errorf("action %q: model options = %v, want %v", actionID, selector.Options, wantModels)
		}
		offered := map[string]bool{}
		for _, o := range selector.Options {
			offered[o] = true
		}
		for m := range modelsWithoutOwnControls[actionID] {
			if !offered[m] {
				t.Errorf("action %q: control exemption names unoffered model %q", actionID, m)
			}
			if gated[m] {
				t.Errorf("action %q: model %q now has a gated control; remove its exemption", actionID, m)
			}
		}
		for _, m := range wantModels {
			if !offered[m] {
				t.Errorf("action %q: model %q missing from the selector", actionID, m)
			}
			if !gated[m] && !modelsWithoutOwnControls[actionID][m] {
				t.Errorf("action %q: model %q has no parameter gated on it", actionID, m)
			}
		}
	}
}

// The Generate page builds its model dropdown from the same table the job handler
// resolves endpoints and payloads from, so rendering the empty form proves that
// table is well-formed and that every model in it is reachable from the UI.
func TestFalAIGeneratePageListsModels(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("fal-ai"); err != nil {
		t.Fatalf("EnablePlugin(fal-ai): %v", err)
	}
	// The page short-circuits to a "not configured" notice without a key; the
	// value is never sent anywhere because no prompt is submitted below.
	pm.SetPluginSettings("fal-ai", map[string]any{"api_key": "test-key-not-used"})

	html, err := pm.HandlePage(context.Background(), "fal-ai", "generate", PageContext{
		Path:   "generate",
		Method: "GET",
	})
	if err != nil {
		t.Fatalf("HandlePage(generate): %v", err)
	}

	for _, model := range []string{
		"nanobanana2", "nanobananapro", "nanobanana_lite", "gptimage2",
		"seedream5", "grok2", "muse", "fibo15",
	} {
		if !strings.Contains(html, `<option value="`+model+`">`) {
			t.Errorf("generate form is missing the %q model option", model)
		}
	}
	// The first option is what an un-touched form submits.
	if i := strings.Index(html, `<option value="`); i < 0 || !strings.HasPrefix(html[i:], `<option value="nanobanana2">`) {
		t.Error("nanobanana2 is no longer the generate form's default model")
	}
}

// showWhenValues returns the values of param.ShowWhen[key], which the plugin API
// accepts either as a single string or as a list of them.
func showWhenValues(p *ActionParam, key string) []string {
	raw, ok := p.ShowWhen[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

func TestBundledPluginsDeclareManifests(t *testing.T) {
	// Every bundled plugin ships a manifest, for two reasons. The boot warning
	// for an unmanifested plugin tells the operator to add one — and for these
	// six that meant telling them to edit files the project ships. And running
	// the six real plugins under real grants is the only test of the taxonomy
	// that is not a test fixture: if a capability is missing, their e2e suite
	// fails on a nil call.
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	discovered := pm.DiscoveredPlugins()
	if len(discovered) == 0 {
		t.Fatal("no bundled plugins discovered")
	}
	for _, dp := range discovered {
		if !dp.Manifest.Declared {
			t.Errorf("bundled plugin %q has no manifest, so it loads with the full mah surface and warns "+
				"the operator to fix a file we ship", dp.Name)
			continue
		}
		granted := dp.Manifest.Capabilities()
		if len(granted) == len(AllCapabilities) {
			t.Errorf("bundled plugin %q declares every capability; the point is to declare what it uses", dp.Name)
		}
		if len(granted) == 0 {
			t.Errorf("bundled plugin %q declares no capabilities but is a working plugin", dp.Name)
		}
	}
}

// The escape helpers neutralise the shortcode brackets as HTML entities rather
// than by removing them, and that choice is what keeps the editors working: an
// editor's x-data attribute carries JSON arrays and a JS spread, both written by
// the plugin, and both pass through the same helper as the value they wrap. The
// browser decodes an attribute before Alpine ever sees it, so the entities cost
// nothing there -- but a helper that stripped the brackets instead, or escaped
// into something the HTML parser does not decode, would leave Alpine parsing
// broken JS with nothing failing until a page rendered it.
func TestBundledEditorAttributeSurvivesEscaping(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("meta-editors"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	rendered, err := pm.RenderShortcode(
		context.Background(),
		"meta-editors",
		"plugin:meta-editors:multi-select",
		"resource", 1,
		json.RawMessage(`{"tags":["a"]}`),
		map[string]string{"path": "tags", "options": "a,b"},
		nil,
		"", false,
	)
	if err != nil {
		t.Fatalf("RenderShortcode: %v", err)
	}

	// Read the attribute the way the HTML parser does: it ends at the first
	// unescaped quote, which is itself part of what escaping guarantees.
	const marker = `x-data="`
	i := strings.Index(rendered, marker)
	if i < 0 {
		t.Fatalf("no x-data attribute in %q", rendered)
	}
	rest := rendered[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("unterminated x-data attribute in %q", rendered)
	}
	xdata := html.UnescapeString(rest[:end])

	for _, want := range []string{`options: ["a","b"]`, `let a = [...this.val]`} {
		if !strings.Contains(xdata, want) {
			t.Errorf("x-data lost %q after escaping and decoding: %s", want, xdata)
		}
	}
}
