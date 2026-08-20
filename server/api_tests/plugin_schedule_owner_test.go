package api_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// writeSchedulePlugin writes a plugin that declares one schedule. The manifest
// half is in the `plugin` table rather than beside it. It is declared rather
// than omitted on purpose, though not because it has to be — a manifest-less
// plugin is "legacy" and keeps the whole mah surface, so it would get
// mah.schedule too. Declaring it is what a real scheduled plugin looks like, and
// it exercises `schedule` as the separate capability it deliberately is: folding
// it into `jobs` would have widened every plugin already consented to jobs into
// one that can run unattended timer work.
func writeSchedulePlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeSchedulePlugin %s: MkdirAll: %v", name, err)
	}
	lua := fmt.Sprintf(`
plugin = {
    name = %q,
    version = "1.0",
    api_version = 1,
    description = "schedule ownership test plugin",
    capabilities = { "schedule" },
}

function init()
    mah.schedule({
        id = "poll",
        every = "15m",
        handler = function() end,
    })
end
`, name)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("writeSchedulePlugin %s: WriteFile: %v", name, err)
	}
}

// adminBearerWithID mints an admin token and hands back the id as well, because
// the assertion here is about *which* user ends up on the row. roleBearer
// returns only the header.
func adminBearerWithID(t *testing.T, tc *TestContext, username string) (uint, string) {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %s: %v", username, err)
	}
	return u.ID, "Bearer " + raw
}

// enableThroughRouter enables a plugin the way the manage page and the CLI both
// do it — through the endpoint, which is the seam under test.
func enableThroughRouter(t *testing.T, tc *TestContext, bearer, name string) {
	t.Helper()
	postPluginState(t, tc, bearer, "/v1/plugin/enable", name)
}

// disableThroughRouter is its counterpart. Enabling an already-enabled plugin is
// refused with a 400, so a re-enable has to go the whole way round.
func disableThroughRouter(t *testing.T, tc *TestContext, bearer, name string) {
	t.Helper()
	postPluginState(t, tc, bearer, "/v1/plugin/disable", name)
}

func postPluginState(t *testing.T, tc *TestContext, bearer, path, name string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("name="+name))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", bearer)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s %s: status %d, body %s", path, name, rr.Code, rr.Body.String())
	}
}

// setupScheduleEnv is setupPluginEnv without the enable, because enabling is the
// thing being tested and doing it on the app context directly is exactly the
// short-cut that hid this defect.
func setupScheduleEnv(t *testing.T, names ...string) *TestContext {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		writeSchedulePlugin(t, root, name)
	}
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.PluginPath = root
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
	})
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := tc.AppCtx.EnsureAdminUser("bootstrap-admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}
	return tc
}

// A schedule row's owner is what decides whether it ever runs: both
// DuePluginSchedules and ClaimPluginSchedule carry `created_by_user_id IS NOT
// NULL`, because a row with no live registration must be inert. The sync that
// records the owner has its own test, and that test supplies the principal by
// hand. The route did not — it ran on the unscoped singleton, where the acting
// user resolves to nobody and, under -auth, to NULL rather than to root. Both
// halves are correct alone; composed, every schedule was created unowned and
// therefore never claimed, so the feature did nothing in exactly the deployment
// mode its ownership rule exists for.
//
// Only a test through the router can see this, which is why it lives here and
// not beside the sync.
func TestPluginEnableThroughTheRouterRecordsTheOperator(t *testing.T) {
	tc := setupScheduleEnv(t, "poller")
	operatorID, bearer := adminBearerWithID(t, tc, "operator")

	enableThroughRouter(t, tc, bearer, "poller")

	var row models.PluginSchedule
	if err := tc.DB.Where("plugin_name = ? AND schedule_id = ?", "poller", "poll").
		First(&row).Error; err != nil {
		t.Fatalf("read schedule row: %v", err)
	}
	if row.CreatedByUserId == nil {
		t.Fatal("the schedule row has no owner, so DuePluginSchedules and " +
			"ClaimPluginSchedule both skip it and this schedule never runs")
	}
	if *row.CreatedByUserId != operatorID {
		t.Fatalf("owner = %d, want %d (the operator who enabled it)", *row.CreatedByUserId, operatorID)
	}
}

// Re-enabling transfers the schedule to whoever did it. The sync's rule is
// "only ever set, never clear": a principal-less sync (a restart) leaves an
// existing owner alone, but an operator who enables the plugin takes it on,
// because enabling *is* the consent gesture and the runs are about to happen as
// them. Rows survive a disable precisely so that binding has something to
// survive on.
//
// This is the assertion that proves the route binds *the caller* rather than
// merely some principal: a hard-coded bind — to root, to the first admin — would
// satisfy the test above and fail this one.
func TestReEnableTransfersTheScheduleToTheNewOperator(t *testing.T) {
	tc := setupScheduleEnv(t, "poller")
	firstID, firstBearer := adminBearerWithID(t, tc, "first-operator")
	secondID, secondBearer := adminBearerWithID(t, tc, "second-operator")

	enableThroughRouter(t, tc, firstBearer, "poller")
	ownerAfter := func(step string) uint {
		t.Helper()
		var row models.PluginSchedule
		if err := tc.DB.Where("plugin_name = ? AND schedule_id = ?", "poller", "poll").
			First(&row).Error; err != nil {
			t.Fatalf("%s: read schedule row: %v", step, err)
		}
		if row.CreatedByUserId == nil {
			t.Fatalf("%s: the schedule row has no owner, so it never runs", step)
		}
		return *row.CreatedByUserId
	}

	if got := ownerAfter("first enable"); got != firstID {
		t.Fatalf("owner = %d, want %d (the operator who enabled it)", got, firstID)
	}

	disableThroughRouter(t, tc, secondBearer, "poller")
	enableThroughRouter(t, tc, secondBearer, "poller")

	if got := ownerAfter("re-enable"); got != secondID {
		t.Fatalf("owner = %d, want %d; re-enabling did not transfer the schedule to "+
			"the operator who did it", got, secondID)
	}
}

// Enabling a plugin that is already enabled is refused, and the refusal used to
// take the durable state with it. SetPluginEnabled writes enabled=true before
// calling EnablePlugin, and reverted to false on any error — but "already
// enabled" is an error whose correct outcome is that the row stays true. The
// plugin goes on running with the database saying it is off, so the next restart
// does not load it and its schedules stop.
//
// The disable branch already had the right shape: it consults IsEnabled before
// reverting, because a state that is already correct must not be undone.
func TestFailedReEnableDoesNotTurnOffARunningPlugin(t *testing.T) {
	tc := setupScheduleEnv(t, "poller")
	_, bearer := adminBearerWithID(t, tc, "operator")

	enableThroughRouter(t, tc, bearer, "poller")

	// The second enable is refused; that is the documented behaviour and not
	// what this test is about.
	req := httptest.NewRequest(http.MethodPost, "/v1/plugin/enable", strings.NewReader("name=poller"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", bearer)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("second enable: status %d, want 400 (%s)", rr.Code, rr.Body.String())
	}

	if !tc.AppCtx.PluginManager().IsEnabled("poller") {
		t.Fatal("the plugin stopped running, which is not what a refused enable should do")
	}

	var state models.PluginState
	if err := tc.DB.Where("plugin_name = ?", "poller").First(&state).Error; err != nil {
		t.Fatalf("read plugin state: %v", err)
	}
	if !state.Enabled {
		t.Fatal("the refused enable wrote enabled=false while the plugin is still loaded: " +
			"the database and the process now disagree, and the next restart drops the plugin " +
			"and its schedules")
	}
}
