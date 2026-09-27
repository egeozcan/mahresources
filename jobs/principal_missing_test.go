package jobs

import (
	"context"
	"errors"
	"testing"

	"mahresources/models"

	"gorm.io/gorm"
)

// A Job accepted to act as an account that has since been deleted can never run:
// ids are not reused, and no other principal may be substituted. Found waiting,
// it ends failed and says why, rather than blocking with a Resume that queues it
// only to block it again.
func TestAWaitingJobWhoseAccountWasDeletedFailsSayingWhy(t *testing.T) {
	_, deps := newDispatchDatabase(t, "principal-deleted.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	actor := uint(41)
	accepted := acceptQueued(t, svc, deps, &actor)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", accepted.ID).
		Updates(map[string]any{"actor_user_id": nil, "owner_user_id": nil}).Error; err != nil {
		t.Fatalf("delete the account: %v", err)
	}

	if _, _, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
	}); err == nil {
		t.Fatal("a claim of a job whose account is gone answered no error")
	}
	if adapter.dispatchedCount() != 0 {
		t.Fatal("a job whose account is gone was handed to its adapter")
	}
	ended, err := svc.Get(deps, Access{Administrator: true}, accepted.ID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if ended.State != StateFailed || ended.Failure == nil || ended.Failure.Code != "principal-missing" {
		t.Fatalf("the job ended %s with %+v, want failed as principal-missing", ended.State, ended.Failure)
	}
	if ended.Failure.Message == "" {
		t.Fatal("the failure says nothing a person can read")
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State == models.JobClaimStateHeld {
		t.Fatalf("the failed job still holds its claim (%s)", claim.State)
	}
}

// A Job an earlier release blocked for the same reason keeps its block, and is
// offered no Resume: resuming it only queues it to fail.
func TestABlockedJobWhoseAccountWasDeletedOffersNoResume(t *testing.T) {
	h := newCommandHarness(t)
	h.adapter.advertise = func(context.Context, CommandContext) ([]Command, error) {
		return []Command{{Key: CommandCancel, Label: "Cancel"}, {Key: CommandResume, Label: "Resume"}}, nil
	}
	actor := uint(41)
	job := h.acceptReplayable(&actor)
	execution := h.claim(job.ID)
	h.now()
	if _, err := h.svc.Transition(h.deps, Transition{
		JobID: job.ID, ExpectedVersion: jobRow(t, h.deps, job.ID).Version, ExecutionToken: execution.ExecutionToken,
		To: StateBlocked, Event: EventInput{Type: EventBlocked},
	}); err != nil {
		t.Fatalf("block the job: %v", err)
	}
	admin := Access{UserID: 1, Administrator: true}
	requireCommandKeys(t, "a blocked job whose account exists", h.advertise(job.ID, admin),
		CommandCancel, CommandResume, CommandPin, CommandUnpin, CommandPinLineage)

	if err := h.deps.DB.Model(&models.Job{}).Where("id = ?", job.ID).
		Updates(map[string]any{"actor_user_id": nil, "owner_user_id": nil}).Error; err != nil {
		t.Fatalf("delete the account: %v", err)
	}
	requireCommandKeys(t, "a blocked job whose account was deleted", h.advertise(job.ID, admin),
		CommandCancel, CommandPin, CommandUnpin, CommandPinLineage)

	h.adapter.selectCommand = func(_ context.Context, request CommandFilterRequest) (*gorm.DB, bool, error) {
		return request.Jobs.Select("jobs.id"), true, nil
	}
	page, err := h.svc.List(h.deps, admin, Filter{Command: CommandResume}, Cursor{}, 0)
	if err != nil {
		t.Fatalf("list command=resume: %v", err)
	}
	if len(page.Jobs) != 0 {
		t.Fatalf("command=resume lists %d jobs; the only candidate's account is gone", len(page.Jobs))
	}
}

// Deleting an account nulls the owner reference, so a filter on the owner's id
// can never find that account's Jobs again. OwnerDeleted is how an
// administrator asks for them.
func TestOwnerDeletedListsTheJobsOfDeletedAccounts(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	admin := Access{UserID: 1, Administrator: true}
	owner, other := uint(41), uint(42)
	accept := func(ownerID *uint) Snapshot {
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: ownerID, ActorUserID: ownerID, Replay: ReplayInput{NonReplayable: true},
		})
	}
	orphaned := accept(&owner)
	kept := accept(&other)
	system := accept(nil)
	if err := deps.DB.Model(&models.Job{}).Where("owner_user_id = ?", owner).
		Updates(map[string]any{"owner_user_id": nil, "owner_deleted": true, "actor_user_id": nil, "actor_deleted": true}).Error; err != nil {
		t.Fatalf("delete the account: %v", err)
	}

	page, err := svc.List(deps, admin, Filter{OwnerDeleted: true}, Cursor{}, 0)
	if err != nil {
		t.Fatalf("list ownerDeleted: %v", err)
	}
	if len(page.Jobs) != 1 || page.Jobs[0].ID != orphaned.ID || !page.Jobs[0].OwnerDeleted {
		t.Fatalf("ownerDeleted lists %+v, want only the deleted account's job", page.Jobs)
	}
	for _, job := range []Snapshot{kept, system} {
		read, err := svc.Get(deps, admin, job.ID)
		if err != nil {
			t.Fatalf("read %s: %v", job.ID, err)
		}
		if read.OwnerDeleted {
			t.Fatalf("job %s reads as a deleted account's", job.ID)
		}
	}
	if _, err := svc.List(deps, admin, Filter{OwnerDeleted: true, OwnerID: &other}, Cursor{}, 0); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("an owner id beside ownerDeleted = %v, want refused", err)
	}
}

// owner=me narrows a listing to the asker's own Jobs without naming their id,
// under the same visibility predicate as every other filter: it is how an
// administrator's drawer counts the work that needs their own attention.
func TestOwnedByViewerListsTheAskersOwnJobs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	mine, theirs := uint(1), uint(2)
	accept := func(owner *uint) Snapshot {
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, ActorUserID: owner, Replay: ReplayInput{NonReplayable: true},
		})
	}
	own := accept(&mine)
	accept(&theirs)
	accept(nil)

	admin := Access{UserID: mine, Administrator: true}
	page, err := svc.List(deps, admin, Filter{OwnedByViewer: true}, Cursor{}, 0)
	if err != nil {
		t.Fatalf("list owner=me: %v", err)
	}
	if len(page.Jobs) != 1 || page.Jobs[0].ID != own.ID {
		t.Fatalf("owner=me lists %d jobs to an administrator, want only their own", len(page.Jobs))
	}
	counts, err := svc.CountByState(deps, admin, Filter{OwnedByViewer: true})
	if err != nil || counts[string(StateQueued)] != 1 {
		t.Fatalf("owner=me counts %v, %v; want the one own job", counts, err)
	}
	host, err := svc.List(deps, Access{Administrator: true}, Filter{OwnedByViewer: true}, Cursor{}, 0)
	if err != nil || len(host.Jobs) != 0 {
		t.Fatalf("owner=me for a principal with no account lists %d jobs, %v; want none", len(host.Jobs), err)
	}
	other, err := svc.List(deps, Access{UserID: theirs}, Filter{OwnedByViewer: true}, Cursor{}, 0)
	if err != nil || len(other.Jobs) != 1 || other.Jobs[0].OwnerUserID == nil || *other.Jobs[0].OwnerUserID != theirs {
		t.Fatalf("owner=me for another account = %+v, %v", other.Jobs, err)
	}
	for _, filter := range []Filter{{OwnedByViewer: true, OwnerID: &theirs}, {OwnedByViewer: true, OwnerDeleted: true}} {
		if _, err := svc.List(deps, admin, filter, Cursor{}, 0); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("owner=me beside another owner filter %+v = %v, want refused", filter, err)
		}
	}
}
