package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/gorm"
)

// failReadsOf makes every read of one table fail while the returned switch is on.
func failReadsOf(t *testing.T, ctx *MahresourcesContext, table string) *atomic.Bool {
	t.Helper()
	failing := &atomic.Bool{}
	name := "test:fail-dispatch-read-" + table
	if err := ctx.db.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
		if failing.Load() && db.Statement.Table == table {
			_ = db.AddError(errors.New("injected read failure"))
		}
	}); err != nil {
		t.Fatalf("register the failing read: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })
	return failing
}

// givenBackCount counts the times a Job went back to the queue because its
// dispatch could not check its account.
func givenBackCount(t *testing.T, ctx *MahresourcesContext, jobID string) int {
	t.Helper()
	var events []models.JobEvent
	if err := ctx.db.Where("job_id = ? AND type = ?", jobID, jobs.EventQueued).Find(&events).Error; err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	count := 0
	for _, event := range events {
		if strings.Contains(string(event.Detail), jobDispatchChecksUnansweredReason) {
			count++
		}
	}
	return count
}

func blockedEvents(t *testing.T, ctx *MahresourcesContext, jobID string) int64 {
	t.Helper()
	var count int64
	if err := ctx.db.Model(&models.JobEvent{}).Where("job_id = ? AND type = ?", jobID, jobs.EventBlocked).Count(&count).Error; err != nil {
		t.Fatalf("count blocked events: %v", err)
	}
	return count
}

// TestADispatchWhoseAccountCheckFailedWaitsAndRunsOnceItAnswers pins the rule a
// failed read never becomes a verdict, at the dispatch of a download and of an
// export: the account or scope read fails, and the Job goes back to the queue
// saying why instead of being blocked for a person to resume. This process passes
// over it until its deferral runs out, so a failure that lasts costs one attempt
// per interval rather than one per tick, and once the read answers the Job runs.
func TestADispatchWhoseAccountCheckFailedWaitsAndRunsOnceItAnswers(t *testing.T) {
	for _, table := range []string{"users", "groups"} {
		for _, kind := range []string{JobKindRemoteDownload, JobKindGroupExport} {
			t.Run(table+"/"+kind, func(t *testing.T) {
				ctx := newJobHarnessContext(t, false)
				runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{Claimant: "dispatch-check-test", Interval: time.Hour})
				t.Cleanup(runtime.Stop)

				inside := createGroupNamed(t, ctx, "dispatch-check-"+table+"-"+kind, nil)
				user, err := ctx.CreateUser(&UserInput{Username: "dispatch-check-" + table + "-" + kind, Password: "password1",
					Role: models.RoleUser, ScopeGroupId: &inside.ID})
				if err != nil {
					t.Fatalf("create user: %v", err)
				}
				var input json.RawMessage
				switch kind {
				case JobKindRemoteDownload:
					server := plainContentServer(t, "dispatch check body "+table)
					creator := &query_models.ResourceFromRemoteCreator{URL: server.URL + "/checked.bin"}
					creator.OwnerId = inside.ID
					input, err = remoteDownloadInputJSON(creator, "")
				default:
					input, err = json.Marshal(exportJobInput{Request: *exportRequestForTest(inside.ID)})
				}
				if err != nil {
					t.Fatalf("input: %v", err)
				}
				accepted := acceptJobFor(t, ctx, jobs.Acceptance{
					Kind: kind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
					OwnerUserID: &user.ID, ActorUserID: &user.ID,
					Replay: jobs.ReplayInput{Input: input},
				})

				failing := failReadsOf(t, ctx, table)
				failing.Store(true)
				runtime.tick(context.Background())
				waiting := waitForSnapshot(t, ctx, accepted.ID, "the Job to be given back", func(snap jobs.Snapshot) bool {
					return snap.State != jobs.StateRunning && givenBackCount(t, ctx, accepted.ID) > 0
				})
				if waiting.State != jobs.StateQueued || waiting.Progress.Message != pluginActionWaitingForChecks {
					t.Fatalf("a Job whose account check failed is %s saying %q, want queued and waiting for the checks",
						waiting.State, waiting.Progress.Message)
				}

				// Inside its deferral, this process does not claim it again.
				for i := 0; i < 5; i++ {
					runtime.tick(context.Background())
				}
				time.Sleep(100 * time.Millisecond)
				if got := givenBackCount(t, ctx, accepted.ID); got != 1 {
					t.Fatalf("the Job was given back %d times inside its deferral, want once", got)
				}

				failing.Store(false)
				time.Sleep(giveBackDeferral(1) + 50*time.Millisecond)
				runtime.tick(context.Background())
				finished := waitForSnapshot(t, ctx, accepted.ID, "the Job to run once the check answers",
					func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
				if finished.State != jobs.StateSucceeded {
					t.Fatalf("the Job ended %s (%+v) once its account could be checked", finished.State, finished.Failure)
				}
				if blocked := blockedEvents(t, ctx, accepted.ID); blocked != 0 {
					t.Fatalf("the Job was blocked %d times over a read that failed", blocked)
				}
			})
		}
	}
}

// TestADispatchDeferralDoublesAndForgetsAnAnsweredJob pins the backoff the pass
// over follows: one second, doubling with each failure in a row, capped at the
// plugin-action admission's cap, and forgotten once the Job's checks answer.
func TestADispatchDeferralDoublesAndForgetsAnAnsweredJob(t *testing.T) {
	deferrals := newDispatchCheckDeferrals()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, pluginActionGiveBackCap, pluginActionGiveBackCap} {
		if got := deferrals.deferJob("a", JobKindRemoteDownload, now); got != want {
			t.Fatalf("failure %d deferred %s, want %s", i+1, got, want)
		}
	}
	if got := deferrals.passOver(JobKindRemoteDownload, now); len(got) != 1 || got[0] != "a" {
		t.Fatalf("pass over = %v, want [a]", got)
	}
	if got := deferrals.passOver(JobKindGroupExport, now); len(got) != 0 {
		t.Fatalf("another Kind passes over %v", got)
	}
	if got := deferrals.passOver(JobKindRemoteDownload, now.Add(pluginActionGiveBackCap)); len(got) != 0 {
		t.Fatalf("a deferral that ran out still passes over %v", got)
	}
	deferrals.answered("a")
	if got := deferrals.deferJob("a", JobKindRemoteDownload, now); got != time.Second {
		t.Fatalf("an answered Job's next failure deferred %s, want a fresh second", got)
	}
	// A streak long past is forgotten too.
	if got := deferrals.deferJob("b", JobKindRemoteDownload, now); got != time.Second {
		t.Fatalf("first deferral %s", got)
	}
	if got := deferrals.deferJob("b", JobKindRemoteDownload, now.Add(time.Second+dispatchCheckForget+time.Second)); got != time.Second {
		t.Fatalf("a lapsed streak deferred %s, want a fresh second", got)
	}
}

// TestADispatchWhoseAccountWasDeletedAfterItsClaimFails pins the three answers a
// dispatch's account check can give, at the moment the claim-time check cannot
// see: the account is deleted after the Job was claimed and before its dispatch
// checks it. Deletion is permanent, so the Job ends failed with the same
// principal-missing failure the claim path records, never blocked with a Resume
// that could not run it. A disabled account is still a block (an administrator
// can enable it again), and a read that fails still sends the Job back to the
// queue.
func TestADispatchWhoseAccountWasDeletedAfterItsClaimFails(t *testing.T) {
	for _, kind := range []string{JobKindRemoteDownload, JobKindGroupExport} {
		t.Run(kind, func(t *testing.T) {
			ctx := newJobHarnessContext(t, false)
			group := createGroupNamed(t, ctx, "dispatch-deleted-"+kind, nil)
			claimFor := func(name string, disable bool) (jobs.Execution, *models.User) {
				t.Helper()
				user, err := ctx.CreateUser(&UserInput{Username: "dispatch-" + name + "-" + kind, Password: "password1", Role: models.RoleUser})
				if err != nil {
					t.Fatalf("create user: %v", err)
				}
				var input json.RawMessage
				if kind == JobKindRemoteDownload {
					creator := &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/" + name + ".bin"}
					creator.OwnerId = group.ID
					input, err = remoteDownloadInputJSON(creator, "")
				} else {
					input, err = json.Marshal(exportJobInput{Request: *exportRequestForTest(group.ID)})
				}
				if err != nil {
					t.Fatalf("input: %v", err)
				}
				accepted := acceptJobFor(t, ctx, jobs.Acceptance{
					Kind: kind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
					OwnerUserID: &user.ID, ActorUserID: &user.ID,
					Replay: jobs.ReplayInput{Input: input},
				})
				execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
					Kind: kind, KindVersion: 1, JobID: accepted.ID, Claimant: "dispatch-deleted-test", Lease: time.Minute,
				})
				if err != nil || !claimed {
					t.Fatalf("claim: claimed=%v err=%v", claimed, err)
				}
				if disable {
					if err := ctx.db.Model(&models.User{}).Where("id = ?", user.ID).Update("disabled", true).Error; err != nil {
						t.Fatalf("disable the account: %v", err)
					}
				}
				return execution, user
			}
			dispatch := func(execution jobs.Execution) {
				t.Helper()
				adapter, ok := ctx.JobService().AdapterFor(kind, 1)
				if !ok {
					t.Fatalf("no adapter for %s", kind)
				}
				_ = adapter.Dispatch(context.Background(), execution)
			}

			// Deleted between the claim and the dispatch's check.
			deleted, user := claimFor("deleted", false)
			if err := ctx.DeleteUser(user.ID); err != nil {
				t.Fatalf("delete the account: %v", err)
			}
			dispatch(deleted)
			ended := jobSnapshot(t, ctx.JobService(), ctx, deleted.JobID)
			if ended.State != jobs.StateFailed || ended.Failure == nil || ended.Failure.Code != "principal-missing" {
				t.Fatalf("a Job whose account was deleted after its claim is %s (%+v), want failed as principal-missing", ended.State, ended.Failure)
			}
			if blocked := blockedEvents(t, ctx, deleted.JobID); blocked != 0 {
				t.Fatalf("the Job was blocked %d times on its way to failing", blocked)
			}

			// Disabled between the claim and the dispatch's check: a block.
			disabled, _ := claimFor("disabled", true)
			dispatch(disabled)
			if snap := jobSnapshot(t, ctx.JobService(), ctx, disabled.JobID); snap.State != jobs.StateBlocked {
				t.Fatalf("a Job whose account was disabled after its claim is %s, want blocked", snap.State)
			}

			// A read that fails: back to the queue.
			unread, _ := claimFor("unread", false)
			failing := failReadsOf(t, ctx, "users")
			failing.Store(true)
			dispatch(unread)
			failing.Store(false)
			if snap := jobSnapshot(t, ctx.JobService(), ctx, unread.JobID); snap.State != jobs.StateQueued {
				t.Fatalf("a Job whose account could not be read is %s, want queued", snap.State)
			}
		})
	}
}
