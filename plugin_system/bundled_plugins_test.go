package plugin_system

import (
	"context"
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
// changes most often. Pin the shape its UI depends on: every model offered by a
// `model` selector must be one build_request knows how to route.
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

	for _, id := range []string{"colorize", "upscale", "restore", "edit", "vectorize", "polish"} {
		if _, ok := actions[id]; !ok {
			t.Errorf("action %q not registered", id)
		}
	}

	// Each model option must have at least one parameter gated on it, which is
	// how the form knows to reveal that model's controls. A model added to the
	// selector but never wired up shows an empty form and fails at request time.
	for actionID, wantModels := range map[string][]string{
		"upscale": {"clarity", "crystal", "esrgan", "creative", "seedvr", "bria_creative", "topaz", "drct", "aura_sr"},
		"restore": {"photo_restoration", "codeformer", "swin2sr", "nafnet_denoise", "nafnet_deblur"},
		"edit":    {"flux2", "flux2pro", "nanobanana2", "nanobananapro", "gptimage2", "seedream5", "grok2", "flux1dev"},
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
		for _, m := range wantModels {
			if !offered[m] {
				t.Errorf("action %q: model %q missing from the selector", actionID, m)
			}
			if !gated[m] {
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
		"nanobanana2", "nanobananapro", "gptimage2", "seedream5", "grok2",
		"imagen4", "imagen4_fast", "imagen4_ultra",
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
