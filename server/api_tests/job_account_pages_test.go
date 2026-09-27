package api_tests

import (
	"html"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
)

// The Job Center names whose each Job is for an administrator, and lets them
// filter by a person rather than a user number. The delete dialog for an account
// says how many of its Jobs have not finished.
func TestJobPagesNameAccountsForAnAdministrator(t *testing.T) {
	tc := setupAuthEnv(t)
	adminCookie, _ := loginCookieAndCSRF(t, tc)
	alice, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "alice", DisplayName: "Alice Liddell", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
			Kind: application_context.JobKindRemoteDownload, KindVersion: 1, State: jobs.StateQueued,
			Origin: "api", OwnerUserID: &alice.ID, ActorUserID: &alice.ID, Title: "alice's download",
			Replay: jobs.ReplayInput{NonReplayable: true},
		}); err != nil {
			t.Fatalf("accept alice's job: %v", err)
		}
	}

	page := doReq(tc, http.MethodGet, "/jobs", map[string]string{"Accept": "text/html"}, []*http.Cookie{adminCookie}, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("/jobs answered %d", page.Code)
	}
	body := html.UnescapeString(page.Body.String())
	if !strings.Contains(body, "Owner:</span> Alice Liddell (alice)") {
		t.Fatalf("the administrator's /jobs does not name alice as the owner of her jobs")
	}
	if !strings.Contains(body, `id="job-filter-owner"`) || !strings.Contains(body, `>Alice Liddell (alice)</option>`) {
		t.Fatalf("the administrator's owner filter does not offer alice by name")
	}
	if !strings.Contains(body, `name="ownerDeleted"`) {
		t.Fatalf("the administrator's filters do not offer a deleted owner")
	}

	users := doReq(tc, http.MethodGet, "/admin/users", map[string]string{"Accept": "text/html"}, []*http.Cookie{adminCookie}, nil)
	if users.Code != http.StatusOK {
		t.Fatalf("/admin/users answered %d", users.Code)
	}
	if !strings.Contains(html.UnescapeString(users.Body.String()), "Delete user alice? This also destroys their tokens and sessions, and clears them as the creator of everything they made. 2 of their jobs have not finished") {
		t.Fatalf("alice's delete dialog does not count her unfinished jobs")
	}
}
