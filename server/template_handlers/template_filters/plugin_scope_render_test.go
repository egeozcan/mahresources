package template_filters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/plugin_system"
)

// enableScopeProbePlugin loads a plugin registering one shortcode whose output is
// unmistakable, so a test can tell "the plugin ran" from "it did not".
func enableScopeProbePlugin(t *testing.T) *plugin_system.PluginManager {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "probe")
	if err := writeProbePlugin(pluginDir); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	pm, err := plugin_system.NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("probe"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

// renderDescription runs a template that pushes an entity description through the
// process_shortcodes tag, exactly as templates/partials/description.tpl does.
func renderDescription(t *testing.T, pm *plugin_system.PluginManager, reqCtx context.Context, description string) string {
	t.Helper()
	tpl, err := pongo2.FromString(`{% process_shortcodes description entity %}`)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := tpl.Execute(pongo2.Context{
		"description":     description,
		"entity":          &models.Note{ID: 1, Name: "n", Description: description},
		"_pluginManager":  pm,
		"_requestContext": reqCtx,
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}
	return out
}

// A group-confined principal must not be able to make plugin Lua run.
//
// This is the regression test for the actual vulnerability: plugin host functions
// use the process-wide unscoped DB handle, and authz_policy.go only denies URL
// paths under /v1/plugins/ and /plugins/. An entity description is rendered
// through process_shortcodes, and a confined *user* may write descriptions inside
// its own subtree, so before the fix it could author a [plugin:...] shortcode on
// an entity it owns and execute unscoped plugin code just by viewing it.
//
// The predicate test in auth/ and the source-level guard in internal/arch/ both
// check pieces of this. Neither checks the behaviour.
func TestScopedPrincipalCannotRenderPluginShortcode(t *testing.T) {
	pm := enableScopeProbePlugin(t)
	const description = `before [plugin:probe:mark] after`
	scopeGroup := uint(3)

	cases := []struct {
		name      string
		principal *auth.Principal
		wantRun   bool
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}, true},
		{"unscoped user", &auth.Principal{UserID: 2, Role: models.RoleUser}, true},
		{"group-limited user", &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scopeGroup}, false},
		{"guest", &auth.Principal{UserID: 4, Role: models.RoleGuest, ScopeGroupID: &scopeGroup}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqCtx := auth.WithPrincipal(context.Background(), tc.principal)
			out := renderDescription(t, pm, reqCtx, description)

			ran := strings.Contains(out, "PROBE-RAN")
			if ran != tc.wantRun {
				t.Fatalf("plugin executed = %v, want %v\nrendered: %s", ran, tc.wantRun, out)
			}
			if !tc.wantRun && strings.Contains(out, "PROBE-RAN") {
				t.Fatalf("confined principal reached plugin code: %s", out)
			}
			// The surrounding text must survive either way: the shortcode is
			// suppressed, the description is not.
			if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
				t.Fatalf("surrounding description text was lost: %s", out)
			}
		})
	}
}

// A request context with no principal at all fails closed. withAuthentication
// attaches one to every non-public path in both auth modes, and no public path
// renders a plugin surface, so its absence means the render path lost track of
// the caller, and running unscoped plugin code there
// would be the exact failure the gate exists to prevent.
func TestMissingPrincipalCannotRenderPluginShortcode(t *testing.T) {
	pm := enableScopeProbePlugin(t)
	out := renderDescription(t, pm, context.Background(), `x [plugin:probe:mark] y`)
	if strings.Contains(out, "PROBE-RAN") {
		t.Fatalf("plugin ran with no principal on the context: %s", out)
	}
}

// writeProbePlugin writes a minimal plugin whose shortcode emits a sentinel.
func writeProbePlugin(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	const src = `
plugin = { name = "probe", version = "1.0", description = "scope probe" }

function init()
    mah.shortcode({
        name = "mark",
        label = "Mark",
        render = function(ctx)
            return "<span>PROBE-RAN</span>"
        end,
    })
end
`
	return os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(src), 0o644)
}

// renderDescriptionWithAccess is renderDescription with a per-plugin rule on the
// context, which is what every real page render now supplies.
func renderDescriptionWithAccess(t *testing.T, pm *plugin_system.PluginManager, reqCtx context.Context, description string, access auth.PluginAccess) string {
	t.Helper()
	tpl, err := pongo2.FromString(`{% process_shortcodes description entity %}`)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := tpl.Execute(pongo2.Context{
		"description":     description,
		"entity":          &models.Note{ID: 1, Name: "n", Description: description},
		"_pluginManager":  pm,
		"_requestContext": reqCtx,
		"_pluginAccess":   access,
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}
	return out
}

// The deny is no longer all-or-nothing: an operator may mark one plugin as
// reachable by group-limited users without opening the rest. So the same
// confined principal renders one plugin's shortcode and not another's, and
// which one is a per-plugin operator decision rather than a property of the
// caller.
func TestPluginAccess_ConfinedPrincipalReachesOnlyAllowedPlugins(t *testing.T) {
	pm := enableScopeProbePlugin(t)
	const description = `before [plugin:probe:mark] after`
	scopeGroup := uint(3)
	confined := &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scopeGroup}
	reqCtx := auth.WithPrincipal(context.Background(), confined)

	for _, tc := range []struct {
		name    string
		allowed map[string]bool
		wantRun bool
	}{
		{"this plugin is allowed", map[string]bool{"probe": true}, true},
		{"a different plugin is allowed", map[string]bool{"other": true}, false},
		{"nothing is allowed", map[string]bool{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access := auth.PluginAccessFor(reqCtx, func(name string) bool { return tc.allowed[name] })
			out := renderDescriptionWithAccess(t, pm, reqCtx, description, access)

			if ran := strings.Contains(out, "PROBE-RAN"); ran != tc.wantRun {
				t.Fatalf("plugin executed = %v, want %v\nrendered: %s", ran, tc.wantRun, out)
			}
			if !tc.wantRun {
				// A refusal renders what a context with no plugin renderer at
				// all renders. Anything more specific would let a reader
				// enumerate which plugins exist, or which ones it may not use.
				if !strings.Contains(out, "plugin unavailable in this context") {
					t.Errorf("a refused plugin shortcode rendered %q, which is not the neutral comment", out)
				}
			}
		})
	}
}

// The other direction, and the one that protects existing deployments: a render
// path that supplies no per-plugin rule keeps the rule that came before it.
// Absence must not blank out plugins for an admin.
func TestPluginAccess_MissingPredicateFallsBackToTheWholeRequestRule(t *testing.T) {
	pm := enableScopeProbePlugin(t)
	const description = `before [plugin:probe:mark] after`
	scopeGroup := uint(3)

	for _, tc := range []struct {
		name      string
		principal *auth.Principal
		wantRun   bool
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}, true},
		{"unscoped user", &auth.Principal{UserID: 2, Role: models.RoleUser}, true},
		{"group-limited user", &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scopeGroup}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reqCtx := auth.WithPrincipal(context.Background(), tc.principal)
			out := renderDescription(t, pm, reqCtx, description) // no _pluginAccess
			if ran := strings.Contains(out, "PROBE-RAN"); ran != tc.wantRun {
				t.Fatalf("plugin executed = %v, want %v\nrendered: %s", ran, tc.wantRun, out)
			}
		})
	}
}
