package api_tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
)

// scopedActionRunner is the action-run double with the per-plugin toggle under
// the test's control.
type scopedActionRunner struct {
	*testPluginRunner
	allowScoped bool
}

func (r *scopedActionRunner) PluginAllowsScopedPrincipals(string) bool { return r.allowScoped }

// An action is the most direct way to make a plugin's Lua run — more direct
// than the pages and shortcodes the URL deny covers — so the operator's
// per-plugin decision has to govern it too. Gating the indirect surfaces while
// leaving this one open would make the setting mean something other than what
// it says.
func TestActionRun_GroupLimitedCallerIsRefusedAPluginNotOpenedToThem(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableSyncActionPlugin(t, t.TempDir(), 0)

	scopeGroup := uint(7)
	confined := &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scopeGroup}

	for _, tc2 := range []struct {
		name        string
		allowScoped bool
		wantStatus  int
	}{
		{"plugin not opened to limited users", false, http.StatusForbidden},
		{"plugin opened to limited users", true, http.StatusOK},
	} {
		t.Run(tc2.name, func(t *testing.T) {
			runner := &scopedActionRunner{
				testPluginRunner: &testPluginRunner{
					pm:         pm,
					reader:     tc.AppCtx.ActionEntityRefReader(),
					dataReader: tc.AppCtx.ActionEntityDataReader(),
				},
				allowScoped: tc2.allowScoped,
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

			req, _ := http.NewRequest("POST", "/v1/jobs/action/run",
				strings.NewReader(`{"plugin":"bulk-plugin","action":"act","entity_ids":[1]}`))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(auth.WithPrincipal(context.Background(), confined))

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc2.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", tc2.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// The other half: an unscoped caller is unaffected by the toggle, because the
// toggle is about group-limited accounts and nothing else. A deployment with no
// scoped users must see no change at all.
func TestActionRun_UnscopedCallerIgnoresTheToggle(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableSyncActionPlugin(t, t.TempDir(), 0)

	runner := &scopedActionRunner{
		testPluginRunner: &testPluginRunner{
			pm:         pm,
			reader:     tc.AppCtx.ActionEntityRefReader(),
			dataReader: tc.AppCtx.ActionEntityDataReader(),
		},
		allowScoped: false,
	}

	for _, principal := range []*auth.Principal{
		{UserID: 1, Role: models.RoleAdmin},
		{UserID: 2, Role: models.RoleUser}, // unscoped
		nil,                                // auth off
	} {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

		req, _ := http.NewRequest("POST", "/v1/jobs/action/run",
			strings.NewReader(`{"plugin":"bulk-plugin","action":"act","entity_ids":[1]}`))
		req.Header.Set("Content-Type", "application/json")
		if principal != nil {
			req = req.WithContext(auth.WithPrincipal(context.Background(), principal))
		}

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("principal %+v: expected 200, got %d body=%s", principal, rr.Code, rr.Body.String())
		}
	}
}

var _ = plugin_system.ActionRegistration{}
