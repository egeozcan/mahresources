package api_tests

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// Plugin host functions (mah.db.*) run against the UNSCOPED application context,
// so a group-confined principal could read or mutate entities outside its
// subtree through any plugin endpoint. Until plugin data access is itself
// scope-aware, confined principals (group-scoped users and guests) are denied
// every plugin-code-executing endpoint outright (fail-closed). Unscoped
// principals are unaffected.
func TestPluginEndpoints_ConfinedPrincipalsDenied(t *testing.T) {
	tc := setupAuthEnv(t)

	g := &models.Group{Name: "plugin-scope-grp"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	scopedBearer := scopedUserBearer(t, tc, g.ID)
	guestBearer := roleBearer(t, tc, models.RoleGuest)
	unscopedBearer := roleBearer(t, tc, models.RoleUser) // user with no scope group

	type ep struct {
		method, path, body string
	}
	endpoints := []ep{
		{http.MethodPost, "/v1/plugins/foo/some/action", `{}`},
		{http.MethodGet, "/v1/plugins/foo/some/action", ""},
		{http.MethodPost, "/v1/plugins/foo/display/render", `{"type":"x"}`},
		{http.MethodGet, "/v1/plugins/foo/block/render?blockId=1&mode=view", ""},
	}

	reqBody := func(e ep) io.Reader {
		if e.body == "" {
			return nil
		}
		return strings.NewReader(e.body)
	}

	confined := map[string]string{"scoped-user": scopedBearer, "guest": guestBearer}
	for label, bearer := range confined {
		for _, e := range endpoints {
			h := map[string]string{"Accept": "application/json", "Authorization": bearer}
			if e.body != "" {
				h["Content-Type"] = "application/json"
			}
			rr := doReq(tc, e.method, e.path, h, nil, reqBody(e))
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: confined principal must be 403, got %d", label, e.method, e.path, rr.Code)
			}
		}
	}

	// An unscoped user must NOT be blocked by the confinement guard: the request
	// reaches the plugin handler (which 404s/503s because no plugin is loaded),
	// never the 403 deny.
	for _, e := range endpoints {
		h := map[string]string{"Accept": "application/json", "Authorization": unscopedBearer}
		if e.body != "" {
			h["Content-Type"] = "application/json"
		}
		rr := doReq(tc, e.method, e.path, h, nil, reqBody(e))
		if rr.Code == http.StatusForbidden {
			t.Errorf("unscoped user %s %s should not be 403 (confinement guard misfired)", e.method, e.path)
		}
	}
}
