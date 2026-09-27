package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

// An administrator sees every account's Jobs, so the Job Center names each Job's
// owner and actor for them. Anyone else sees only their own Jobs, so they are
// told their own name and never another account's.
func TestJobAccountLabelsNameAccountsForTheViewerWhoMayKnowThem(t *testing.T) {
	ctx := newStampTestContext(t, true)
	alice, err := ctx.CreateUser(&UserInput{Username: "alice", DisplayName: "Alice Liddell", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := ctx.CreateUser(&UserInput{Username: "bob", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	admin := ctx.WithPrincipal(&auth.Principal{UserID: 9999, Role: models.RoleAdmin})
	labels, err := admin.JobAccountLabels([]uint{alice.ID, bob.ID, alice.ID, 424242})
	if err != nil {
		t.Fatalf("label as an administrator: %v", err)
	}
	if labels[alice.ID] != "Alice Liddell (alice)" || labels[bob.ID] != "bob" {
		t.Fatalf("an administrator's labels = %v", labels)
	}
	if _, found := labels[424242]; found {
		t.Fatalf("an id with no account was labelled: %v", labels)
	}

	self := ctx.WithPrincipal(auth.FromUser(alice))
	labels, err = self.JobAccountLabels([]uint{alice.ID, bob.ID})
	if err != nil {
		t.Fatalf("label as alice: %v", err)
	}
	if labels[alice.ID] != "Alice Liddell (alice)" {
		t.Fatalf("alice is not told her own name: %v", labels)
	}
	if _, found := labels[bob.ID]; found {
		t.Fatalf("alice is told another account's name: %v", labels)
	}

	options, err := admin.JobAccountOptions()
	if err != nil {
		t.Fatalf("list account options: %v", err)
	}
	var names []string
	for _, option := range options {
		names = append(names, option.Label)
	}
	if len(options) < 2 {
		t.Fatalf("an administrator is offered %v", names)
	}
	if others, err := self.JobAccountOptions(); err != nil || len(others) != 0 {
		t.Fatalf("a user is offered accounts %v, %v", others, err)
	}
}

// Deleting an account leaves its unfinished Jobs with nobody to run as, so the
// delete dialog says how many there are. Finished work, and work that acts as
// somebody else, is not counted.
func TestUnfinishedJobCountsCountWorkThatActsAsEachAccount(t *testing.T) {
	ctx := newStampTestContext(t, true)
	svc := jobs.NewService()
	accept := func(owner, actor *uint, state jobs.State) jobs.Snapshot {
		accepted, err := svc.Accept(jobs.Deps{DB: ctx.db}, jobs.Acceptance{
			Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
			OwnerUserID: owner, ActorUserID: actor, Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if state != jobs.StateQueued {
			if err := ctx.db.Model(&models.Job{}).Where("id = ?", accepted.ID).Update("state", string(state)).Error; err != nil {
				t.Fatalf("move the job to %s: %v", state, err)
			}
		}
		return accepted
	}
	alice, bob, carol := uint(3), uint(4), uint(5)
	accept(&alice, &alice, jobs.StateQueued)
	accept(&alice, &alice, jobs.StateScheduled)
	accept(&alice, &alice, jobs.StateSucceeded)
	accept(&alice, &bob, jobs.StateRunning)
	accept(nil, nil, jobs.StateQueued)
	// A row from before the execution class was recorded, with only an owner:
	// dispatch runs it as that owner, so it acts as them.
	legacy := accept(&carol, nil, jobs.StateQueued)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", legacy.ID).Update("execution_principal", "").Error; err != nil {
		t.Fatalf("clear the execution class: %v", err)
	}
	// A row like it whose actor was deleted: dispatch refuses it as the deleted
	// actor's, so it does not act as the owner who remains.
	orphaned := accept(&carol, &bob, jobs.StateQueued)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", orphaned.ID).
		Updates(map[string]any{"execution_principal": "", "actor_user_id": nil, "actor_deleted": true}).Error; err != nil {
		t.Fatalf("delete the legacy row's actor: %v", err)
	}

	counts, err := ctx.UnfinishedJobCounts()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if counts[alice] != 2 || counts[bob] != 1 || counts[carol] != 1 || len(counts) != 3 {
		t.Fatalf("counts = %v, want alice 2, bob 1 and carol 1", counts)
	}
}
