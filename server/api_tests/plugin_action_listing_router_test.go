package api_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// setupPluginListingEnv builds a router whose app context really loads the named
// plugins, enables them, and (when auth is asked for) bootstraps an admin. The
// handler tests use a double for the toggle; this one goes through the live
// PluginAllowsScopedPrincipals, so it also proves the endpoint is wired to it.
func setupPluginListingEnv(t *testing.T, authEnabled bool, names ...string) *TestContext {
	t.Helper()
	return setupPluginEnv(t, authEnabled, writeActionPlugin, names...)
}

// setupPluginEnv is the same environment with the fixture left to the caller.
// The rendered surfaces need an action the list and detail templates actually
// draw, which writeActionPlugin's resource/detail action is not.
func setupPluginEnv(t *testing.T, authEnabled bool, write func(t *testing.T, root, name string), names ...string) *TestContext {
	t.Helper()

	root := t.TempDir()
	for _, name := range names {
		write(t, root, name)
	}

	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.PluginPath = root
		if authEnabled {
			c.AuthEnabled = true
			c.SessionTTL = time.Hour
		}
	})
	// Same reason setupAuthEnv pins the pool: shared-cache SQLite raises
	// SQLITE_LOCKED on a writer/reader conflict, which busy_timeout does not
	// retry.
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if authEnabled {
		if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
			t.Fatalf("bootstrap admin: %v", err)
		}
	}

	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}
	for _, name := range names {
		if err := tc.AppCtx.SetPluginEnabled(name, true); err != nil {
			t.Fatalf("enable %s: %v", name, err)
		}
	}
	return tc
}

// confinedUserBearer mints a token for a user actually confined to a group.
// roleBearer attaches a scope group only to roles that REQUIRE one, and a user
// does not — so reusing it here would produce an unscoped account and the
// confinement this test is about would not exist.
func confinedUserBearer(t *testing.T, tc *TestContext) string {
	t.Helper()
	g := &models.Group{Name: "plugin-listing-scope"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	return scopedUserBearer(t, tc, g.ID)
}

// routerPluginActions lists resource actions through the real router. An empty
// bearer sends no Authorization header, which is what an auth-off deployment
// does.
func routerPluginActions(t *testing.T, tc *TestContext, bearer string) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/plugin/actions?entity=resource", nil)
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return listedPlugins(t, rr)
}

// The endpoint, not just the handler function: a filter added to the handler but
// never handed the live per-plugin decision would pass the double tests and fail
// here.
func TestPluginActions_ScopedListingThroughTheRouter(t *testing.T) {
	tc := setupPluginListingEnv(t, true, "open-plugin", "shut-plugin")
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", true); err != nil {
		t.Fatalf("open plugin to scoped principals: %v", err)
	}

	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	scopedBearer := confinedUserBearer(t, tc)

	both := "open-plugin,shut-plugin"
	assert := func(step string, bearer string, want string) {
		t.Helper()
		got := strings.Join(routerPluginActions(t, tc, bearer), ",")
		if got != want {
			t.Fatalf("%s: listed %q, want %q", step, got, want)
		}
	}

	assert("admin", adminBearer, both)
	assert("group-limited user", scopedBearer, "open-plugin")

	// Revoking has to reach the listing. The cache behind
	// PluginAllowsScopedPrincipals is invalidated by the write, so no wait is
	// involved and a stale answer here is a real defect, not a race.
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", false); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	assert("after revoke, admin", adminBearer, both)
	assert("after revoke, group-limited user", scopedBearer, "")

	// And granting the other one moves the listing the other way, per plugin.
	if err := tc.AppCtx.SetPluginScopedAccess("shut-plugin", true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	assert("after grant, group-limited user", scopedBearer, "shut-plugin")
}

// With auth disabled every request runs as the implicit administrator, so the
// listing is exactly what it always was. Both plugins here have the toggle off,
// which is the default an existing deployment upgrades into: it must not lose
// its action buttons.
func TestPluginActions_AuthDisabledListsEveryAction(t *testing.T) {
	tc := setupPluginListingEnv(t, false, "open-plugin", "shut-plugin")

	got := strings.Join(routerPluginActions(t, tc, ""), ",")
	if want := "open-plugin,shut-plugin"; got != want {
		t.Fatalf("listed %q, want %q", got, want)
	}
}
