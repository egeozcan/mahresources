package application_context

import (
	"encoding/json"
	"strings"
	"testing"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestReadinessListsUnfinishedJobsThatMayBelongToADeletedAccount pins the review
// list for the rows an earlier release left ambiguous: an unfinished Job written
// before Jobs recorded which principal they act as, with no owner, no actor and no
// deletion mark, runs as the host, and the row cannot say whether that is what it
// was accepted for or what a deleted account's work now looks like. Readiness
// lists them for an operator and stays ready: legitimately actorless work matches
// too, and readiness gates startup.
func TestReadinessListsUnfinishedJobsThatMayBelongToADeletedAccount(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 10, MaxBatches: 20, WritersDrained: true}); err != nil || !result.Complete {
		t.Fatalf("empty migration = %+v, %v", result, err)
	}
	input, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/review.bin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	accept := func(state jobs.State) jobs.Snapshot {
		return acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: state, Origin: "api",
			Replay: jobs.ReplayInput{Input: input},
		})
	}
	legacy := func(snap jobs.Snapshot, updates map[string]any) {
		updates["execution_principal"] = ""
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Updates(updates).Error; err != nil {
			t.Fatal(err)
		}
	}

	candidate := accept(jobs.StateQueued)
	legacy(candidate, map[string]any{})
	recorded := accept(jobs.StateQueued) // records its principal: the host, on purpose
	marked := accept(jobs.StateQueued)
	legacy(marked, map[string]any{"owner_deleted": true})
	ended := accept(jobs.StateQueued)
	legacy(ended, map[string]any{"state": string(jobs.StateCancelled)})

	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.Ready {
		t.Fatalf("readiness = %+v, want ready: the review list is not a blocker", readiness)
	}
	review := readiness.ReviewCandidates
	if review.Count != 1 || len(review.Jobs) != 1 || review.Jobs[0].ID != candidate.ID {
		t.Fatalf("review candidates = %+v, want only %s", review, candidate.ID)
	}
	if got := review.Jobs[0]; got.Kind != JobKindRemoteDownload || got.State != string(jobs.StateQueued) || got.AcceptedAt.IsZero() {
		t.Fatalf("the candidate is described as %+v", got)
	}
	for _, id := range []string{recorded.ID, marked.ID, ended.ID} {
		for _, listed := range review.Jobs {
			if listed.ID == id {
				t.Fatalf("%s is listed for review", id)
			}
		}
	}
	encoded, err := json.Marshal(readiness)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"reviewCandidates":{"count":1,"jobs":[{"id":"`+candidate.ID+`"`) {
		t.Fatalf("readiness JSON = %s", encoded)
	}
}
