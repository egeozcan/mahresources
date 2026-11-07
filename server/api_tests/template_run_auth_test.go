package api_tests

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// namedBearer creates a user and returns its ID and an Authorization header.
func namedBearer(t *testing.T, tc *TestContext, username string, role models.Role) (uint, http.Header) {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: username, Password: "password1", Role: role})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID, http.Header{"Authorization": {"Bearer " + raw}}
}

// The finish route is gated like the generate route of the same carrier.
func TestTemplateRunFinishRouteFollowsTheCarrierRoleGate(t *testing.T) {
	tc := setupAuthEnv(t)
	headers := map[models.Role]http.Header{}
	for _, role := range []models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser, models.RoleGuest} {
		headers[role] = http.Header{"Authorization": {roleBearer(t, tc, role)}}
	}
	allowed := map[string]map[models.Role]bool{
		"category":         {models.RoleAdmin: true},
		"resourceCategory": {models.RoleAdmin: true},
		"noteType":         {models.RoleAdmin: true, models.RoleEditor: true},
	}
	for carrier, roles := range allowed {
		for role, header := range headers {
			rr := postFinish(tc, "/v1/"+carrier+"/generateTemplate/finish", "no-such-run", header)
			if roles[role] && rr.Code != http.StatusNotFound {
				t.Errorf("%s as %s: want 404 (reached the handler), got %d %s", carrier, role, rr.Code, rr.Body.String())
			}
			if !roles[role] && rr.Code != http.StatusForbidden {
				t.Errorf("%s as %s: want 403, got %d", carrier, role, rr.Code)
			}
		}
	}
}

// With accounts on, a run belongs to its user: another admin cannot finish it,
// and demoting the account mid-run ends the run with nothing released.
func TestHarnessRunBelongsToItsAccountAndEndsWhenDemoted(t *testing.T) {
	tc := setupAuthEnv(t)
	ownerID, owner := namedBearer(t, tc, "owner", models.RoleAdmin)
	_, other := namedBearer(t, tc, "other", models.RoleAdmin)

	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	conv := &fakeConversation{step: func(ctx context.Context, n int, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return application_context.ConversationResponse{}, ctx.Err()
		}
		return submitOnce(".leaked{}")(ctx, n, req)
	}}
	installHarness(tc, conv, 5)
	srv := newHarnessServer(t, tc)

	dry := map[string]any{"dryRun": true}
	for k, v := range harnessSlotBody() {
		dry[k] = v
	}
	rr := postRunJSON(tc, categoryGenerate, dry, owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "read live data as everything") {
		t.Fatalf("the Disclosure does not state whose access the run uses: %s", rr.Body.String())
	}
	hash := disclose2(t, rr.Body.String())

	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), owner)
	r := newSSEReader(resp.Body)
	runID := runIDOf(t, r.nextWithin(t, 5*time.Second))
	<-entered

	finish := "/v1/category/generateTemplate/finish"
	if rr := postFinish(tc, finish, runID, other); rr.Code != http.StatusNotFound {
		t.Fatalf("another account must get 404, got %d", rr.Code)
	}
	if _, err := tc.AppCtx.UpdateUser(ownerID, &application_context.UserUpdate{Role: application_context.UserField[models.Role]{Set: true, Value: models.RoleEditor}}); err != nil {
		t.Fatalf("demote: %v", err)
	}
	close(release)
	frames := r.readUntilEnd(t, 5*time.Second)
	last := frames[len(frames)-1]
	if last.Event != "error" || !strings.Contains(last.Data, `"status":403`) {
		t.Fatalf("a demoted account's run must end in a 403 error event, got %q %s", last.Event, last.Data)
	}
	for _, f := range frames {
		if f.Event == "result" || strings.Contains(f.Raw, ".leaked{}") || strings.Contains(f.Raw, `"transcript"`) {
			t.Fatalf("the demoted account was sent run output: %s", f.Raw)
		}
	}
}
