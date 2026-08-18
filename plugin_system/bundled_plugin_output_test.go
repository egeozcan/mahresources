package plugin_system

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"mahresources/shortcodes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin's output is template source: Process re-expands it whichever form the
// author wrote the call in. So everything a bundled plugin prints is template
// source too, and the bundled plugins print values a plain `user` role can
// write -- a meta value as a badge label, as a barcode caption, as a link's
// href, as an editor's current value.
//
// Each bundled plugin rolls its own html_escape (data-views:19, widgets:19,
// meta-editors:19, fal-ai) rather than calling mah.html_escape, and none of the
// four touches square brackets. parseAttrs then runs html.UnescapeString over
// the attribute span, so the quoting they do apply is undone again inside a
// bracket. A value somebody typed into a meta field therefore reaches the page
// as a shortcode, and the viewer's own render runs it under the viewer's scope.
//
// These tests run the real bundled Lua, wired to Process the way the four
// request paths wire it (server/routes.go:331 and its three siblings).

// bundledRenderer wires pm into Process the way the request paths do.
func bundledRenderer(pm *PluginManager) shortcodes.PluginRenderer {
	return func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
		return pm.RenderShortcode(context.Background(), pluginName, sc.Name,
			mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity,
			sc.InnerContent, sc.IsBlock)
	}
}

// enableAllBundledPlugins loads the shipped plugin directory and enables every
// plugin in it, so the sweeps below cover what a deployment actually runs.
func enableAllBundledPlugins(t *testing.T) *PluginManager {
	t.Helper()
	pm, err := NewPluginManager(bundledPluginDir(t))
	require.NoError(t, err)
	t.Cleanup(func() { pm.Close() })

	for _, dp := range pm.DiscoveredPlugins() {
		require.NoErrorf(t, pm.EnablePlugin(dp.Name), "EnablePlugin(%q)", dp.Name)
	}
	return pm
}

// bundledShortcodeNames lists every shortcode the enabled bundled plugins
// register, e.g. "plugin:data-views:badge".
func bundledShortcodeNames(t *testing.T, pm *PluginManager) []string {
	t.Helper()
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var names []string
	for _, scs := range pm.shortcodes {
		for _, sc := range scs {
			names = append(names, sc.TypeName)
		}
	}
	sort.Strings(names)
	return names
}

// excerpt trims a rendered page fragment down to the neighbourhood of needle.
// Some of these shortcodes emit a few hundred SVG rects, and a failure that
// prints all of them is unreadable.
func excerpt(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return s
	}
	start := max(i-60, 0)
	end := min(i+len(needle)+60, len(s))
	return "..." + s[start:end] + "..."
}

// The canonical use of the badge, straight out of its own registered example:
// a CustomSummary prints one meta field per card. That field is written by
// whoever may edit the entity, which under -auth includes the `user` role.
func TestBundledBadgeLabelIsNotTemplateSource(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("data-views"))

	const injected = "INJECTED-FROM-A-VALUE-A-USER-TYPED"
	ctx := shortcodes.MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"[property path=\"Name\" raw=\"true\"]"}`),
		Entity:     struct{ Name string }{injected},
	}

	out := shortcodes.Process(context.Background(),
		`[plugin:data-views:badge path="status"]`, ctx, bundledRenderer(pm), nil)

	assert.NotContains(t, out, injected,
		`the meta value ran as a shortcode, and raw="true" then printed the entity field unescaped: %s`,
		excerpt(out, injected))
	// The label must still be printed, only inert. Rendering nothing would
	// satisfy the assertion above while breaking the shortcode.
	assert.Contains(t, out, "property", "the badge stopped printing its label: %s", out)
}

// The sweep: no bundled shortcode may turn a value it prints into template
// source, whichever of them prints one. Kept as a loop over the live registry
// rather than a hand-written list, so a shortcode added later is covered without
// anyone remembering to add it here, and so a fix applied to one echo site
// rather than to the escaping every site shares still fails. A shortcode that
// prints no value, or that needs a database this harness has none of, contains
// no marker and passes.
func TestNoBundledShortcodeTurnsAValueIntoTemplateSource(t *testing.T) {
	pm := enableAllBundledPlugins(t)

	names := bundledShortcodeNames(t, pm)
	require.NotEmpty(t, names, "no bundled plugin registered a shortcode")
	require.Contains(t, names, "plugin:data-views:badge",
		"the sweep no longer covers the shortcode the finding was found on")

	const injected = "INJECTED-FROM-A-VALUE-A-USER-TYPED"
	// Two sources, because resolve_data_source reads either one: a meta value
	// (path=) and an entity field (field=). Both are user-written.
	sources := []struct {
		name  string
		attrs string
		ctx   shortcodes.MetaShortcodeContext
	}{
		{
			name:  "meta value",
			attrs: `path="status"`,
			ctx: shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   1,
				Meta:       json.RawMessage(`{"status":"[property path=\"Name\" raw=\"true\"]"}`),
				Entity:     struct{ Name string }{injected},
			},
		},
		{
			name:  "entity field",
			attrs: `field="Description"`,
			ctx: shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   1,
				Meta:       json.RawMessage(`{}`),
				Entity: struct {
					Name        string
					Description string
				}{injected, `[property path="Name" raw="true"]`},
			},
		},
	}

	renderer := bundledRenderer(pm)
	for _, src := range sources {
		for _, name := range names {
			out := shortcodes.Process(context.Background(),
				"["+name+" "+src.attrs+"]", src.ctx, renderer, nil)
			if strings.Contains(out, injected) {
				t.Errorf("%s expanded the %s it printed: %s",
					name, src.name, excerpt(out, injected))
			}
		}
	}
}

// The same hole aimed at the page's query budget rather than at an entity
// field: a value somebody typed becomes an [mrql] the render executes, under
// the viewer's scope and against the viewer's budget. The authored input names
// no [mrql] at all, so any executor call whatsoever came from the printed value.
func TestNoBundledShortcodeRunsAQueryFromAValueItPrints(t *testing.T) {
	pm := enableAllBundledPlugins(t)

	names := bundledShortcodeNames(t, pm)
	require.Contains(t, names, "plugin:data-views:badge")

	ctx := shortcodes.MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"[mrql query=\"notes\"]"}`),
	}

	renderer := bundledRenderer(pm)
	for _, name := range names {
		ran := ""
		executor := func(_ context.Context, query string, _ shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
			ran = query
			return &shortcodes.QueryResult{EntityType: "note", Mode: "flat"}, nil
		}

		shortcodes.Process(context.Background(),
			"["+name+` path="status"]`, ctx, renderer, executor)

		if ran != "" {
			t.Errorf("%s let a value it printed run the query %q", name, ran)
		}
	}
}
