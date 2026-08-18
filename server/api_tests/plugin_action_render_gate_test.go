package api_tests

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/models"
)

// The action buttons a browser actually shows do not come from
// GET /v1/plugin/actions. Nothing in src/ or templates/ calls that endpoint; the
// buttons are rendered server-side from pluginDetailActions, pluginCardActions
// and pluginBulkActions, which wrapContextWithPlugins publishes straight out of
// GetActionsForPlacement. So filtering the listing alone leaves a group-limited
// account looking at the same buttons it looked at before, each one a 403 on
// POST /v1/jobs/action/run.
//
// Like the listing, this is a dead control rather than a containment boundary:
// rendering a button runs no plugin code, and the 403 is what holds. It is the
// control the fix set out to remove, and it is the one the user can see.
//
// The predicate is already on the page context as _pluginAccess
// (auth.PluginAccessFor, set about fifty lines above the three assignments), so
// there is a per-plugin answer in scope at the point of the defect.

// groupActionLabel and resourceActionLabel are what the assertions read out of
// the page. All three partials write {{ action.Label }}, so one string covers
// the detail sidebar, the card menu and the bulk bar without an assertion per
// partial's own markup.
//
// Two entities because wrapContextWithPlugins assigns the card and bulk lists
// from a switch on the request path: a filter applied to the group branch alone
// would leave resources and notes exactly as they were.
func groupActionLabel(plugin string) string { return plugin + "-group-action-label" }

func resourceActionLabel(plugin string) string { return plugin + "-resource-action-label" }

// writeGroupActionPlugin drops a plugin registering a group action and a
// resource action into root/<name>, placed on every surface a page renders.
//
// writeActionPlugin cannot stand in for it: its action is a resource action and
// takes the default placement, which is detail only. Against that fixture no
// list page draws anything for anybody, so a page-level assertion would have
// been green before any filter existed and would have measured nothing.
func writeGroupActionPlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeGroupActionPlugin %s: MkdirAll: %v", name, err)
	}
	lua := fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "action render gate test plugin" }

function init()
    mah.action({
        id = "act_group",
        label = %q,
        entity = "group",
        placement = { "detail", "card", "bulk" },
        handler = function(ctx) return { success = true } end,
    })
    mah.action({
        id = "act_resource",
        label = %q,
        entity = "resource",
        placement = { "card", "bulk" },
        handler = function(ctx) return { success = true } end,
    })
end
`, name, groupActionLabel(name), resourceActionLabel(name))
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("writeGroupActionPlugin %s: WriteFile: %v", name, err)
	}
}

// actionPage is a page that draws plugin action buttons, paired with the label
// its entity's action carries.
type actionPage struct {
	name  string
	url   string
	label func(plugin string) string
}

// htmlPageAs fetches a server-rendered page as the holder of bearer. An empty
// bearer sends no Authorization header, which is what an auth-off deployment
// does. Deliberately not Accept: application/json — that would take the JSON
// branch and skip the templates this test is about.
func htmlPageAs(t *testing.T, tc *TestContext, url, bearer string) string {
	t.Helper()
	headers := map[string]string{"Accept": browserAccept}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodGet, url, headers, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %.400s", url, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// A group-limited account is offered a plugin's action buttons exactly when an
// operator has opened that plugin to it, on every page that draws them.
func TestPluginActionButtons_OfferedOnlyWherePluginIsOpenToTheCaller(t *testing.T) {
	tc := setupPluginEnv(t, true, writeGroupActionPlugin, "open-plugin", "shut-plugin")

	// The confined account's own scope group: it is both the detail page it may
	// read and the one row its list page has, so the same group exercises the
	// sidebar, the card menu and the bulk bar.
	scopeGroup := &models.Group{Name: "render-gate-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	scopedBearer := scopedUserBearer(t, tc, scopeGroup.ID)

	// The resource list has no rows for either account, which is deliberate: its
	// bulk bar renders unconditionally, so the page still draws the buttons and
	// the assertion does not depend on a fixture resource being in subtree.
	pages := []actionPage{
		{"group detail", fmt.Sprintf("/group?id=%d", scopeGroup.ID), groupActionLabel},
		{"group list", "/groups", groupActionLabel},
		{"resource list", "/resources", resourceActionLabel},
	}

	offered := func(t *testing.T, page actionPage, bearer, plugin string) bool {
		t.Helper()
		return strings.Contains(htmlPageAs(t, tc, page.url, bearer), page.label(plugin))
	}

	// The control, and a fatal rather than an error: if the admin is offered
	// nothing then the page draws no plugin actions at all, and every absence
	// asserted below would be vacuous.
	for _, page := range pages {
		for _, plugin := range []string{"open-plugin", "shut-plugin"} {
			if !offered(t, page, adminBearer, plugin) {
				t.Fatalf("%s: an admin is not offered %s, so this test measures nothing", page.name, plugin)
			}
		}
	}

	// Neither plugin is open to group-limited accounts, which is the default an
	// existing deployment upgrades into.
	for _, page := range pages {
		for _, plugin := range []string{"open-plugin", "shut-plugin"} {
			if offered(t, page, scopedBearer, plugin) {
				t.Errorf("%s: a group-limited account is offered %s, whose run answers 403", page.name, plugin)
			}
		}
	}

	// Opening one plugin moves that plugin's buttons and no others: the
	// operator's decision is per plugin, and a fix that simply hides every
	// button from confined accounts would fail here.
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", true); err != nil {
		t.Fatalf("open plugin to scoped principals: %v", err)
	}
	for _, page := range pages {
		if !offered(t, page, scopedBearer, "open-plugin") {
			t.Errorf("%s: open-plugin was opened to group-limited accounts and is still not offered", page.name)
		}
		if offered(t, page, scopedBearer, "shut-plugin") {
			t.Errorf("%s: shut-plugin is offered though only open-plugin was opened", page.name)
		}
	}

	// And revoking takes them back on the next page load, for the same reason
	// the listing reads the toggle per request.
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", false); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, page := range pages {
		if offered(t, page, scopedBearer, "open-plugin") {
			t.Errorf("%s: open-plugin is still offered after the grant was revoked", page.name)
		}
	}
}

// With auth off every request runs as the implicit administrator, so the pages
// draw exactly what they always drew. A deployment with no group-limited
// accounts must not lose a button to this filter.
func TestPluginActionButtons_AuthDisabledOffersEveryAction(t *testing.T) {
	tc := setupPluginEnv(t, false, writeGroupActionPlugin, "open-plugin", "shut-plugin")

	g := &models.Group{Name: "render-gate-open"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	for _, page := range []actionPage{
		{"group detail", fmt.Sprintf("/group?id=%d", g.ID), groupActionLabel},
		{"group list", "/groups", groupActionLabel},
		{"resource list", "/resources", resourceActionLabel},
	} {
		body := htmlPageAs(t, tc, page.url, "")
		for _, plugin := range []string{"open-plugin", "shut-plugin"} {
			if !strings.Contains(body, page.label(plugin)) {
				t.Errorf("auth off: %s does not offer %s", page.url, plugin)
			}
		}
	}
}
