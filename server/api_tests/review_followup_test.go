package api_tests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/models"
)

// #1: GetRecentActivity is raw SQL, so it must apply the principal's subtree
// scope explicitly. A group-limited user must not see out-of-subtree entities
// in the dashboard activity feed.
func TestScopedUser_RecentActivityConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "act-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "act-outside"}
	tc.DB.Create(outside)
	tc.DB.Create(&models.Note{Name: "act-note-in", OwnerId: &root.ID})
	tc.DB.Create(&models.Note{Name: "act-note-out", OwnerId: &outside.ID})
	tc.DB.Create(&models.Resource{Name: "act-res-out", OwnerId: &outside.ID})

	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 9, Role: models.RoleUser, ScopeGroupID: &root.ID})
	entries, err := scoped.GetRecentActivity(100)
	if err != nil {
		t.Fatalf("GetRecentActivity: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["act-note-in"] {
		t.Fatalf("scoped activity should include in-subtree note, got %v", names)
	}
	if names["act-note-out"] || names["act-res-out"] || names["act-outside"] {
		t.Fatalf("scoped activity must exclude out-of-subtree entities, got %v", names)
	}

	// Admin sees everything.
	adminEntries, err := tc.AppCtx.GetRecentActivity(100)
	if err != nil {
		t.Fatalf("admin GetRecentActivity: %v", err)
	}
	adminNames := map[string]bool{}
	for _, e := range adminEntries {
		adminNames[e.Name] = true
	}
	if !adminNames["act-note-out"] || !adminNames["act-note-in"] {
		t.Fatalf("admin activity should include every entity, got %v", adminNames)
	}
}

// #2: a scoped user's background download must target a group inside its
// subtree. Out-of-subtree owner/groups, a missing owner, and group creation via
// GroupName are all refused; an in-subtree target is accepted.
func TestScopedUser_DownloadSubmitConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "dl-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "dl-outside"}
	tc.DB.Create(outside)
	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Content-Type": "application/json", "Authorization": bearer}

	post := func(body string) int {
		return doReq(tc, http.MethodPost, "/v1/download/submit", h, nil, strings.NewReader(body)).Code
	}

	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, outside.ID)); c != http.StatusForbidden {
		t.Fatalf("out-of-subtree owner should be 403, got %d", c)
	}
	if c := post(`{"URL":"http://example.com/a"}`); c != http.StatusForbidden {
		t.Fatalf("missing owner should be 403 for a scoped user, got %d", c)
	}
	if c := post(`{"URL":"http://example.com/a","GroupName":"brand-new"}`); c != http.StatusForbidden {
		t.Fatalf("group creation via GroupName should be 403 for a scoped user, got %d", c)
	}
	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","Groups":[%d]}`, outside.ID)); c != http.StatusForbidden {
		t.Fatalf("out-of-subtree attached group should be 403, got %d", c)
	}
	// In-subtree owner is accepted (enqueued).
	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, root.ID)); c == http.StatusForbidden {
		t.Fatalf("in-subtree owner should not be 403, got %d", c)
	}

	// An admin is unrestricted: an out-of-subtree owner is fine.
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	ah := map[string]string{"Content-Type": "application/json", "Authorization": adminBearer}
	if c := doReq(tc, http.MethodPost, "/v1/download/submit", ah, nil,
		strings.NewReader(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, outside.ID))).Code; c == http.StatusForbidden {
		t.Fatalf("admin download should not be scope-restricted, got %d", c)
	}
}

// #4: cancel/pause/resume/retry must verify job ownership; a non-owner gets 404.
func TestDownloadMutation_OwnershipEnforced(t *testing.T) {
	tc := setupAuthEnv(t)
	aBearer, aID := plainUserBearer(t, tc, "dlm-a")
	bBearer, _ := plainUserBearer(t, tc, "dlm-b")

	job, err := tc.AppCtx.DownloadManager().SubmitJob("test", "queued",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			<-ctx.Done() // stay cancellable until the test cancels it
			return nil
		})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	job.SetOwnerUserID(aID)
	jobID := job.ID

	// Non-owner cannot cancel — 404 (not 403, to prevent ID enumeration).
	if c := doReq(tc, http.MethodPost, "/v1/download/cancel?id="+jobID,
		map[string]string{"Authorization": bBearer}, nil, nil).Code; c != http.StatusNotFound {
		t.Fatalf("non-owner cancel should be 404, got %d", c)
	}
	// Owner can cancel.
	if c := doReq(tc, http.MethodPost, "/v1/download/cancel?id="+jobID,
		map[string]string{"Authorization": aBearer}, nil, nil).Code; c != http.StatusOK {
		t.Fatalf("owner cancel should be 200, got %d", c)
	}
}

// #5: import lifecycle endpoints must verify the parse job's owner; another user
// who guesses the job ID cannot inspect, apply, or delete it.
func TestImportLifecycle_OwnershipEnforced(t *testing.T) {
	tc := setupAuthEnv(t)
	aBearer, aID := plainUserBearer(t, tc, "imp-a")
	bBearer, _ := plainUserBearer(t, tc, "imp-b")

	job, err := tc.AppCtx.DownloadManager().SubmitJob(download_queue.JobSourceGroupImportParse, "queued",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			return nil
		})
	if err != nil {
		t.Fatalf("submit parse job: %v", err)
	}
	job.SetOwnerUserID(aID)
	jobID := job.ID

	bH := map[string]string{"Authorization": bBearer, "Content-Type": "application/json"}
	cases := []struct{ method, path string }{
		{http.MethodGet, "/v1/imports/" + jobID + "/plan"},
		{http.MethodGet, "/v1/imports/" + jobID + "/result"},
		{http.MethodPost, "/v1/imports/" + jobID + "/apply"},
		{http.MethodDelete, "/v1/imports/" + jobID},
	}
	for _, c := range cases {
		if code := doReq(tc, c.method, c.path, bH, nil, strings.NewReader("{}")).Code; code != http.StatusNotFound {
			t.Fatalf("non-owner %s %s should be 404, got %d", c.method, c.path, code)
		}
	}

	// The owner passes the ownership gate (delete proceeds, not a 404).
	if code := doReq(tc, http.MethodDelete, "/v1/imports/"+jobID,
		map[string]string{"Authorization": aBearer}, nil, nil).Code; code == http.StatusNotFound {
		t.Fatalf("owner delete should pass the ownership gate, got 404")
	}
}

// Round-4 review: the ownership gate above expires. The owner tag lives only on the
// in-memory queue record, which "Clear completed" removes at once and the retention
// sweep removes an hour later — while the files it authorised (the staged tar, the
// plan, the result) stay on disk, and every one of these endpoints works from those
// files by id. An unknown job used to fall through to the handler's own path, which
// is to read the plan, apply it, or delete the files. So under -auth the check simply
// stopped applying, and any other authenticated user holding the id could act on
// somebody else's import from that moment on.
func TestImportLifecycle_OwnershipSurvivesTheJobBeingCleared(t *testing.T) {
	tc := setupAuthEnv(t)
	aBearer, aID := plainUserBearer(t, tc, "impc-a")
	bBearer, _ := plainUserBearer(t, tc, "impc-b")

	job, err := tc.AppCtx.DownloadManager().SubmitJob(download_queue.JobSourceGroupImportParse, "queued",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			return nil
		})
	if err != nil {
		t.Fatalf("submit parse job: %v", err)
	}
	job.SetOwnerUserID(aID)
	jobID := job.ID

	// The control: while the job is in the queue, the gate is known to work — this is
	// the state TestImportLifecycle_OwnershipEnforced covers.
	bH := map[string]string{"Authorization": bBearer, "Content-Type": "application/json"}
	if code := doReq(tc, http.MethodGet, "/v1/imports/"+jobID+"/plan", bH, nil, nil).Code; code != http.StatusNotFound {
		t.Fatalf("precondition: non-owner plan read should be 404 while the job exists, got %d", code)
	}

	// Now the job goes, the way it goes in practice: the owner presses "Clear
	// completed", or an hour passes.
	waitForJobStatus(t, tc, jobID, download_queue.JobStatusCompleted)
	tc.AppCtx.DownloadManager().ClearFinished(nil)
	if _, stillThere := tc.AppCtx.DownloadManager().GetJob(jobID); stillThere {
		t.Fatalf("precondition: the job is still in the queue, so this test measured nothing")
	}

	cases := []struct{ method, path string }{
		{http.MethodGet, "/v1/imports/" + jobID + "/plan"},
		{http.MethodGet, "/v1/imports/" + jobID + "/result"},
		{http.MethodPost, "/v1/imports/" + jobID + "/apply"},
		{http.MethodDelete, "/v1/imports/" + jobID},
	}
	for _, c := range cases {
		if code := doReq(tc, c.method, c.path, bH, nil, strings.NewReader("{}")).Code; code != http.StatusNotFound {
			t.Errorf("after the job was cleared, non-owner %s %s answered %d — the authorization evidence went with the job", c.method, c.path, code)
		}
	}

	// The positive control the fail-closed rule has to survive: while the job is in
	// the queue — which is the whole of the parse/review/apply flow these endpoints
	// exist for — its owner is unaffected by any of this. Round 5 caught this test
	// discarding A's token and never checking it.
	// DELETE and not the plan read, for the reason the test above uses it: the plan
	// endpoint 404s for a job with no plan file on disk, so it cannot tell a refused
	// gate from a missing artefact. Delete works from disk and answers 204.
	if code := doReq(tc, http.MethodDelete, "/v1/imports/"+ownedJobID(t, tc, aID),
		map[string]string{"Authorization": aBearer}, nil, nil).Code; code == http.StatusNotFound {
		t.Errorf("the owner of a live import job was refused by the ownership gate with 404")
	}

	// A second positive control: an admin is not locked out by the fail-closed reading,
	// because an admin may see every job whatever its owner. Without it, denying
	// every unknown id outright would pass everything above.
	admin, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1")
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	adminRaw, _, err := tc.AppCtx.CreateApiToken(admin.ID, "t", nil)
	if err != nil {
		t.Fatalf("admin token: %v", err)
	}
	adminH := map[string]string{"Authorization": "Bearer " + adminRaw}
	if code := doReq(tc, http.MethodDelete, "/v1/imports/"+jobID, adminH, nil, nil).Code; code == http.StatusNotFound {
		t.Errorf("an admin was refused a cleared import job's delete with 404")
	}
}



// ownedJobID enqueues a parse job owned by uid and returns its id, for assertions
// about a job that is still in the queue.
func ownedJobID(t *testing.T, tc *TestContext, uid uint) string {
	t.Helper()
	job, err := tc.AppCtx.DownloadManager().SubmitJob(download_queue.JobSourceGroupImportParse, "queued",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			<-ctx.Done()
			return nil
		})
	if err != nil {
		t.Fatalf("submit parse job: %v", err)
	}
	job.SetOwnerUserID(uid)
	return job.ID
}
