package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// Running a plugin action is a write. requiredCapability classifies
// POST /v1/jobs/action/run as capWrite (it is in none of isReadViaPost,
// isEditorPath, isTaxonomyPath or isSystemPath), and principalSatisfies answers
// capWrite with Principal.CanWrite, which a guest fails. So withAuthorization
// refuses every guest that endpoint whatever the per-plugin toggle says.
//
// The per-plugin filter cannot see that: it sits inside the handlers, below the
// middleware that made the decision. An operator who opens a plugin to
// group-limited accounts therefore hands every guest the buttons again, each one
// a 403 — which is the defect the filter was written to remove, still standing
// for one of the two roles this repo calls group-limited.
//
// Guest is not a corner: it is half of "group-limited", and the toggle's own
// help text names it ("group-limited users and guests").
//
// These go through the real router on purpose. The bare handler mounts this
// package uses elsewhere have no middleware above them, so the capability
// refusal does not exist there and an agreement test built on them cannot see
// this.

// confinedBearer creates an account of the given role confined to scopeGroupID
// and returns its bearer header.
//
// Neither existing helper can stand in. roleBearer builds its own scope group,
// so two principals from it are confined to different subtrees and cannot be
// pointed at one entity; scopedUserBearer is fixed to a username and to
// RoleUser, so it cannot be called twice and cannot produce a guest.
func confinedBearer(t *testing.T, tc *TestContext, username string, role models.Role, scopeGroupID uint) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: role, ScopeGroupId: &scopeGroupID,
	})
	if err != nil {
		t.Fatalf("create %s %q: %v", role, username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %q: %v", username, err)
	}
	return "Bearer " + raw
}

// runActionThroughRouter posts a one-entity run through the full chain,
// middleware included, and returns the status and body. Bearer requests carry no
// cookie, so no CSRF token is involved; Accept: application/json makes
// denyAccess take its JSON branch.
func runActionThroughRouter(tc *TestContext, bearer, plugin, action string, entityID uint) (int, string) {
	body := strings.NewReader(fmt.Sprintf(`{"plugin":%q,"action":%q,"entity_ids":[%d]}`, plugin, action, entityID))
	rr := doReq(tc, http.MethodPost, "/v1/jobs/action/run", map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": bearer,
	}, nil, body)
	return rr.Code, strings.TrimSpace(rr.Body.String())
}

// A guest is refused every action run, so it must be offered none — including
// from a plugin an operator has opened to group-limited accounts, which is the
// only configuration in which the listing still offers it anything.
func TestPluginActions_GuestIsNotOfferedActionsTheRunPathRefuses(t *testing.T) {
	tc := setupPluginListingEnv(t, true, "open-plugin")
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", true); err != nil {
		t.Fatalf("open plugin to scoped principals: %v", err)
	}

	scopeGroup := &models.Group{Name: "guest-listing-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	guest := confinedBearer(t, tc, "listing_guest", models.RoleGuest, scopeGroup.ID)

	// The control. It holds today and after any repair: a guest may not write,
	// and the entity id is irrelevant because the refusal happens above the
	// handler.
	status, body := runActionThroughRouter(tc, guest, "open-plugin", "act", 1)
	if status != http.StatusForbidden {
		t.Fatalf("guest POST /v1/jobs/action/run: status %d body %s, want 403", status, body)
	}

	if got := routerPluginActions(t, tc, guest); len(got) != 0 {
		t.Errorf("guest is offered %v, every one of which answers %d %s", got, status, body)
	}

	// And an admin still is, so the absence above is the filter and not an empty
	// plugin manager.
	admin := roleBearer(t, tc, models.RoleAdmin)
	if got := strings.Join(routerPluginActions(t, tc, admin), ","); got != "open-plugin" {
		t.Fatalf("admin is offered %q, want \"open-plugin\" — this test would measure nothing", got)
	}
}

// The same for the buttons, which is where a user actually meets an action:
// nothing in the browser calls the listing endpoint, and the detail sidebar, the
// card menu and the bulk bar render straight out of the page context.
func TestPluginActionButtons_GuestIsNotOfferedButtonsTheRunPathRefuses(t *testing.T) {
	tc := setupPluginEnv(t, true, writeGroupActionPlugin, "open-plugin")
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", true); err != nil {
		t.Fatalf("open plugin to scoped principals: %v", err)
	}

	// The guest's own scope group: the detail page it may read, and the one row
	// its group list has.
	scopeGroup := &models.Group{Name: "guest-render-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	guest := confinedBearer(t, tc, "render_guest", models.RoleGuest, scopeGroup.ID)
	adminBearer := roleBearer(t, tc, models.RoleAdmin)

	pages := []actionPage{
		{"group detail", fmt.Sprintf("/group?id=%d", scopeGroup.ID), groupActionLabel},
		{"group list", "/groups", groupActionLabel},
		{"resource list", "/resources", resourceActionLabel},
	}

	// The control, fatal: if the admin is offered nothing then the pages draw no
	// plugin actions at all and every absence below is vacuous.
	for _, page := range pages {
		if !strings.Contains(htmlPageAs(t, tc, page.url, adminBearer), page.label("open-plugin")) {
			t.Fatalf("%s: an admin is not offered open-plugin, so this test measures nothing", page.name)
		}
	}

	for _, page := range pages {
		if strings.Contains(htmlPageAs(t, tc, page.url, guest), page.label("open-plugin")) {
			t.Errorf("%s: a guest is offered open-plugin's button, whose run answers 403", page.name)
		}
	}
}

// The general property, across every role, through the routed chain: an action
// is offered exactly when running it is not refused.
//
// TestPluginActions_ListingAgreesWithTheRunPath asserts the same thing over the
// bare handlers, where it can only ever see the per-plugin gate. Half of the run
// path's answer is the capability check in withAuthorization, so the agreement
// has to be measured where both halves are.
//
// The action targets the confined accounts' own scope group, so entity
// visibility passes for every principal here and the only 403s left are the two
// this is about.
func TestPluginActions_ListingAgreesWithTheRoutedRunPath(t *testing.T) {
	tc := setupPluginEnv(t, true, writeGroupActionPlugin, "open-plugin", "shut-plugin")
	if err := tc.AppCtx.SetPluginScopedAccess("open-plugin", true); err != nil {
		t.Fatalf("open plugin to scoped principals: %v", err)
	}

	scopeGroup := &models.Group{Name: "agreement-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}

	principals := []struct {
		name   string
		bearer string
	}{
		{"admin", roleBearer(t, tc, models.RoleAdmin)},
		{"editor", roleBearer(t, tc, models.RoleEditor)},
		{"unscoped user", roleBearer(t, tc, models.RoleUser)},
		{"group-limited user", confinedBearer(t, tc, "agree_user", models.RoleUser, scopeGroup.ID)},
		{"guest", confinedBearer(t, tc, "agree_guest", models.RoleGuest, scopeGroup.ID)},
	}

	// The control, fatal: an admin's run of an open plugin has to succeed, or
	// every "refused" below could be a broken fixture rather than a policy.
	if status, body := runActionThroughRouter(tc, principals[0].bearer, "open-plugin", "act_group", scopeGroup.ID); status >= http.StatusBadRequest {
		t.Fatalf("admin run: status %d body %s — the fixture does not run, so agreement means nothing", status, body)
	}

	for _, p := range principals {
		offered := map[string]bool{}
		for _, name := range routerActionsFor(t, tc, p.bearer, "group") {
			offered[name] = true
		}

		for _, plugin := range []string{"open-plugin", "shut-plugin"} {
			status, body := runActionThroughRouter(tc, p.bearer, plugin, "act_group", scopeGroup.ID)
			refused := status == http.StatusForbidden
			if offered[plugin] == refused {
				t.Errorf("%s / %s: offered=%v, run answered %d %s", p.name, plugin, offered[plugin], status, body)
			}
		}
	}
}
