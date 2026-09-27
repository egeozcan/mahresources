package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// staleDownloadFixture is one queued download Job for an account, and a server
// that counts every request the transfer would make.
type staleDownloadFixture struct {
	ctx   *MahresourcesContext
	jobID string
	hits  *atomic.Int64
}

func acceptStaleDownload(t *testing.T, ctx *MahresourcesContext, principal *models.User, ownerGroup uint) staleDownloadFixture {
	t.Helper()
	hits := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("body"))
	}))
	t.Cleanup(server.Close)
	creator := &query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "stale.txt", OwnerId: ownerGroup},
		URL:               server.URL + "/stale.txt",
	}
	input, err := remoteDownloadInputJSON(creator, "")
	if err != nil {
		t.Fatalf("encode download input: %v", err)
	}
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		State: jobs.StateQueued, Origin: "api", Title: "stale.txt",
		OwnerUserID: &principal.ID, ActorUserID: &principal.ID,
		Replay: jobs.ReplayInput{Input: input},
	})
	if err != nil {
		t.Fatalf("accept the download: %v", err)
	}
	return staleDownloadFixture{ctx: ctx, jobID: accepted.ID, hits: hits}
}

// dispatch claims the Job and runs its Kind's dispatch once, as the runtime
// would, and answers the Job as it was left and the access the claim carried.
func (f staleDownloadFixture) dispatch(t *testing.T) (jobs.Snapshot, jobs.Access) {
	t.Helper()
	execution, claimed, err := f.ctx.JobService().Claim(context.Background(), f.ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, JobID: f.jobID, Claimant: "stale-principal-test",
	})
	if err != nil || !claimed {
		t.Fatalf("claim the download: claimed=%v err=%v", claimed, err)
	}
	adapter, ok := f.ctx.JobService().AdapterFor(JobKindRemoteDownload, jobDownloadKindVersion)
	if !ok {
		t.Fatal("no download adapter registered")
	}
	dispatchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- adapter.Dispatch(dispatchCtx, execution) }()
	snapshot := waitForSnapshot(t, f.ctx, f.jobID, "the dispatch to decide", func(s jobs.Snapshot) bool {
		return s.State.Terminal() || s.State == jobs.StateBlocked
	})
	cancel()
	<-done
	return snapshot, execution.Access
}

func (f staleDownloadFixture) blockedReason(t *testing.T) string {
	t.Helper()
	events, err := f.ctx.GetJobTimeline(f.jobID, 0, 200)
	if err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	reason := ""
	for _, event := range events {
		if event.Type == jobs.EventBlocked {
			reason = string(event.Detail)
		}
	}
	return reason
}

func (f staleDownloadFixture) assertNothingRan(t *testing.T) {
	t.Helper()
	if hits := f.hits.Load(); hits != 0 {
		t.Fatalf("the transfer reached its server %d times", hits)
	}
	if entries := f.ctx.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("the download was handed to the queue: %d entries", len(entries))
	}
	var resources int64
	if err := f.ctx.db.Model(&models.Resource{}).Count(&resources).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if resources != 0 {
		t.Fatalf("%d resources were created", resources)
	}
}

// An execution can carry the id of an account that no longer exists: a claim
// that read its candidate before a deletion's sweep committed, or a Job an
// in-flight request accepted after the sweep. Neither carries the deleted-account
// marker, so the claim does not end it as principal-missing; the dispatch check
// finds no account behind the id and ends it the way the claim would have,
// failed as principal-missing, before anything runs. A block would offer a Resume
// that could never run it.
func TestAnExecutionNamingADeletedAccountNeverRuns(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	user, err := ctx.CreateUser(&UserInput{Username: "stale-actor", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}
	group := &models.Group{Name: "stale-actor-target"}
	if err := ctx.db.Create(group).Error; err != nil {
		t.Fatalf("create the group: %v", err)
	}
	fixture := acceptStaleDownload(t, ctx, user, group.ID)
	// The account goes, and the sweep that would mark its Jobs does not reach this
	// one: that is the state either interleaving leaves.
	if err := ctx.db.Delete(&models.User{}, user.ID).Error; err != nil {
		t.Fatalf("delete the account row: %v", err)
	}

	snapshot, access := fixture.dispatch(t)
	if access.UserID != user.ID || access.Administrator {
		t.Fatalf("the claim carried %+v, want the deleted account's id and no administrator", access)
	}
	if snapshot.State != jobs.StateFailed || snapshot.Failure == nil || snapshot.Failure.Code != "principal-missing" {
		t.Fatalf("the Job is %s (%+v, blocked %s), want failed as principal-missing", snapshot.State, snapshot.Failure, fixture.blockedReason(t))
	}
	fixture.assertNothingRan(t)
}

// A Retry the preflight admitted can reach dispatch after its target left the
// asker's scope. The dispatch check asks the scope as it stands then, and blocks
// the Job before anything runs.
func TestADownloadWhoseTargetLeftTheScopeAfterAcceptanceNeverRuns(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	scope := &models.Group{Name: "stale-scope-root"}
	if err := ctx.db.Create(scope).Error; err != nil {
		t.Fatalf("create the scope group: %v", err)
	}
	target := &models.Group{Name: "stale-scope-target", OwnerId: &scope.ID}
	if err := ctx.db.Create(target).Error; err != nil {
		t.Fatalf("create the target group: %v", err)
	}
	user, err := ctx.CreateUser(&UserInput{Username: "stale-scope", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scope.ID})
	if err != nil {
		t.Fatalf("create the scoped user: %v", err)
	}
	fixture := acceptStaleDownload(t, ctx, user, target.ID)
	// An administrator moves the target out of the user's subtree after the Job
	// was accepted.
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", target.ID).Update("owner_id", nil).Error; err != nil {
		t.Fatalf("move the target out of scope: %v", err)
	}

	snapshot, access := fixture.dispatch(t)
	if access.UserID != user.ID {
		t.Fatalf("the claim carried %+v, want the asker", access)
	}
	if snapshot.State != jobs.StateBlocked || !strings.Contains(fixture.blockedReason(t), "scope-refused") {
		t.Fatalf("the Job is %s (%s), want blocked as scope-refused", snapshot.State, fixture.blockedReason(t))
	}
	fixture.assertNothingRan(t)
}

// A Job written before its execution principal was recorded derives it from its
// references. When the account it acted as is deleted, the sweep clears those
// references and marks them; the derivation reads the marks, so the Job ends as
// principal-missing instead of reading as work the host does for itself.
func TestALegacyJobWhoseAccountWasDeletedNeverRunsAsTheHost(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	user, err := ctx.CreateUser(&UserInput{Username: "legacy-actor", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}
	group := &models.Group{Name: "legacy-actor-target"}
	if err := ctx.db.Create(group).Error; err != nil {
		t.Fatalf("create the group: %v", err)
	}
	fixture := acceptStaleDownload(t, ctx, user, group.ID)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", fixture.jobID).
		UpdateColumn("execution_principal", "").Error; err != nil {
		t.Fatalf("make the Job a legacy row: %v", err)
	}
	if err := ctx.DeleteUser(user.ID); err != nil {
		t.Fatalf("delete the user: %v", err)
	}

	_, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, JobID: fixture.jobID, Claimant: "legacy-principal-test",
	})
	if claimed {
		t.Fatalf("a legacy Job whose account was deleted was claimed to run (err=%v)", err)
	}
	ended, err := ctx.GetJob(fixture.jobID)
	if err != nil {
		t.Fatalf("read the Job: %v", err)
	}
	if ended.State != jobs.StateFailed || ended.Failure == nil || ended.Failure.Code != "principal-missing" {
		t.Fatalf("the legacy Job is %s with %+v, want failed as principal-missing", ended.State, ended.Failure)
	}
	fixture.assertNothingRan(t)
}
