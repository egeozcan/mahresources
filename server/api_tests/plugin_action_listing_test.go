package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
)

// writeActionPlugin drops a minimal plugin registering one resource action into
// root/<name>. Several of them share one root on purpose: the toggle is per
// plugin, so a test of the listing filter needs one plugin opened to
// group-limited accounts beside one that is not, loaded by the same manager.
// enableSyncActionPlugin builds a manager per call and cannot be composed that
// way.
func writeActionPlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeActionPlugin %s: MkdirAll: %v", name, err)
	}
	lua := fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "action listing test plugin" }

function init()
    mah.action({
        id = "act",
        label = "Act",
        entity = "resource",
        handler = function(ctx) return { success = true } end,
    })
end
`, name)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("writeActionPlugin %s: WriteFile: %v", name, err)
	}
}

// enableActionPlugins loads and enables every named plugin from one manager.
func enableActionPlugins(t *testing.T, names ...string) *plugin_system.PluginManager {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		writeActionPlugin(t, root, name)
	}
	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	for _, name := range names {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin %s: %v", name, err)
		}
	}
	return pm
}

// listingRunner is the action double with a per-plugin toggle. scopedActionRunner
// answers the same for every name, which cannot express the case the filter is
// about: one plugin open to group-limited accounts and one shut.
type listingRunner struct {
	*testPluginRunner
	allow map[string]bool
}

func (r *listingRunner) PluginAllowsScopedPrincipals(name string) bool { return r.allow[name] }

// pluginActionsResponse asks the listing handler for resource actions as the
// given principal. A nil principal is a request context that carries none, which
// is how the action-run tests in this package express admin / auth-off.
func pluginActionsResponse(t *testing.T, runner api_handlers.PluginActionRunner, p *auth.Principal) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/plugin/actions?entity=resource", nil)
	if p != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), p))
	}
	rr := httptest.NewRecorder()
	api_handlers.GetPluginActionsHandler(runner)(rr, req)
	return rr
}

// listedPlugins names the plugins whose actions a listing offered, sorted
// because GetActions walks a map and the order is not part of the contract.
func listedPlugins(t *testing.T, rr *httptest.ResponseRecorder) []string {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("listing returned %d: %s", rr.Code, rr.Body.String())
	}
	var actions []plugin_system.ActionRegistration
	if err := json.Unmarshal(rr.Body.Bytes(), &actions); err != nil {
		t.Fatalf("decode listing %q: %v", rr.Body.String(), err)
	}
	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, a.PluginName)
	}
	sort.Strings(names)
	return names
}

// The listing offers what the caller may run. A group-limited account is refused
// POST /v1/jobs/action/run unless an operator opened that plugin to it, so
// offering it the button is an invitation to a 403. Every other principal is
// unaffected: the toggle is about group-limited accounts and nothing else, and a
// deployment with none of them must see no change.
func TestPluginActions_GroupLimitedCallerSeesOnlyPluginsOpenedToThem(t *testing.T) {
	pm := enableActionPlugins(t, "open-plugin", "shut-plugin")
	runner := &listingRunner{
		testPluginRunner: &testPluginRunner{pm: pm},
		allow:            map[string]bool{"open-plugin": true},
	}

	scope := uint(7)
	both := []string{"open-plugin", "shut-plugin"}

	for _, tc := range []struct {
		name      string
		principal *auth.Principal
		want      []string
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}, both},
		{"unscoped user", &auth.Principal{UserID: 2, Role: models.RoleUser}, both},
		{"group-limited user", &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope}, []string{"open-plugin"}},
		{"group-limited guest", &auth.Principal{UserID: 4, Role: models.RoleGuest, ScopeGroupID: &scope}, []string{"open-plugin"}},
		// A guest whose role mandates a scope but has no group is confined to
		// nothing, and the run path still puts it through the same per-plugin
		// toggle. Its emptiness bites at entity visibility, not here, so the
		// listing must not invent a stricter answer than the run.
		{"guest with no scope group", &auth.Principal{UserID: 5, Role: models.RoleGuest}, []string{"open-plugin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := listedPlugins(t, pluginActionsResponse(t, runner, tc.principal))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("listed %v, want %v", got, tc.want)
			}
		})
	}
}

// The listing reads the toggle on every request. An operator who revokes access
// has to see the buttons go with it on the next page load, and one who grants it
// has to see them appear without a restart.
func TestPluginActions_ListingFollowsTheToggle(t *testing.T) {
	pm := enableActionPlugins(t, "open-plugin")
	runner := &listingRunner{
		testPluginRunner: &testPluginRunner{pm: pm},
		allow:            map[string]bool{},
	}

	scope := uint(7)
	confined := &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope}

	for _, step := range []struct {
		allowed bool
		want    []string
	}{
		{false, nil},
		{true, []string{"open-plugin"}},
		{false, nil},
	} {
		runner.allow["open-plugin"] = step.allowed
		got := listedPlugins(t, pluginActionsResponse(t, runner, confined))
		if strings.Join(got, ",") != strings.Join(step.want, ",") {
			t.Fatalf("allowed=%v: listed %v, want %v", step.allowed, got, step.want)
		}
	}
}

// An empty listing still has to encode as [], not null. The browser maps over
// this response, and a filter that appends to a nil slice hands it null the
// moment everything is filtered out — the case a group-limited account with no
// plugin opened to it hits every time.
func TestPluginActions_FilteredEmptyListingIsStillAnArray(t *testing.T) {
	pm := enableActionPlugins(t, "shut-plugin")
	runner := &listingRunner{
		testPluginRunner: &testPluginRunner{pm: pm},
		allow:            map[string]bool{},
	}

	scope := uint(7)
	rr := pluginActionsResponse(t, runner, &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope})
	if got := strings.TrimSpace(rr.Body.String()); got != "[]" {
		t.Fatalf("body = %q, want []", got)
	}
}

// The two surfaces must not drift apart: an action is offered exactly when
// running it would not be refused for the plugin's sake. The expectation is the
// run handler's own answer rather than a second copy of the rule, so a change to
// either one that does not move the other fails here — the same doctrine
// DownloadHistoryQuery carries, where listing and mutation share one scope.
func TestPluginActions_ListingAgreesWithTheRunPath(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableActionPlugins(t, "open-plugin", "shut-plugin")

	scope := uint(7)
	principals := []struct {
		name string
		p    *auth.Principal
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}},
		{"unscoped user", &auth.Principal{UserID: 2, Role: models.RoleUser}},
		{"group-limited user", &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope}},
		{"group-limited guest", &auth.Principal{UserID: 4, Role: models.RoleGuest, ScopeGroupID: &scope}},
		// A context carrying no principal at all. Whatever the shared predicate
		// answers there, both surfaces have to answer it the same way.
		{"no principal", nil},
	}
	toggles := []map[string]bool{
		{},
		{"open-plugin": true},
		{"open-plugin": true, "shut-plugin": true},
	}

	for _, pc := range principals {
		for i, allow := range toggles {
			runner := &listingRunner{
				testPluginRunner: &testPluginRunner{
					pm:         pm,
					reader:     tc.AppCtx.ActionEntityRefReader(),
					dataReader: tc.AppCtx.ActionEntityDataReader(),
				},
				allow: allow,
			}

			listed := map[string]bool{}
			for _, name := range listedPlugins(t, pluginActionsResponse(t, runner, pc.p)) {
				listed[name] = true
			}

			for _, plugin := range []string{"open-plugin", "shut-plugin"} {
				// Every entity is visible to this double, so the only 403 the
				// run path can produce is the per-plugin gate.
				refused := runActionStatus(t, runner, pc.p, plugin) == http.StatusForbidden
				if listed[plugin] == refused {
					t.Errorf("principal=%s toggles=%v plugin=%s: listed=%v, run refused=%v",
						pc.name, toggles[i], plugin, listed[plugin], refused)
				}
			}
		}
	}
}

// runActionStatus posts a one-entity run of plugin's "act" action as p and
// returns the status the run path answered with.
func runActionStatus(t *testing.T, runner api_handlers.PluginActionRunner, p *auth.Principal, plugin string) int {
	t.Helper()
	body := fmt.Sprintf(`{"plugin":%q,"action":"act","entity_ids":[1]}`, plugin)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if p != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), p))
	}
	rr := httptest.NewRecorder()
	api_handlers.GetActionRunHandler(runner)(rr, req)
	return rr.Code
}
