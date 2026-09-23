package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/spf13/afero"
)

// This file drives the group-export Kind at the seam the deployed UI uses: the
// route it POSTs to, the runtime that dispatches what the route accepted, and the
// artifact the run produces.
//
// The properties worth pinning are about the chain — a Job that exists before
// anything is dispatched, a staged artifact published as a typed output with an
// expiry, and an outcome that follows the artifact rather than the queue's own
// status — and a fake at any link would let the test pass while a real export ran
// unmirrored.

// createGroupForExport makes one top-level group for an export to name.
func createGroupForExport(t *testing.T, tc *TestContext, name string) uint {
	t.Helper()
	group := &models.Group{Name: name}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return group.ID
}

// submitGroupExport POSTs the export route and returns the two identifiers it
// answers with.
func submitGroupExport(t *testing.T, tc *TestContext, groupIDs []uint) (legacyID, canonicalID string) {
	t.Helper()
	res := tc.MakeRequest(http.MethodPost, "/v1/groups/export", map[string]any{
		"rootGroupIds": groupIDs,
		"scope":        map[string]any{"subtree": true},
	})
	if res.Code != http.StatusAccepted {
		t.Fatalf("export submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		JobID          string `json:"jobId"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil {
		t.Fatalf("decode submit response %s: %v", res.Body.String(), err)
	}
	if submitted.JobID == "" {
		t.Fatalf("the export answered no legacy id: %s", res.Body.String())
	}
	return submitted.JobID, submitted.CanonicalJobID
}

// resolveHandleEventually resolves one legacy handle, retrying the fixture's own
// shared-cache table locks. The property under test is that the handle resolves, not that
// it resolves on the first attempt.
func resolveHandleEventually(t *testing.T, tc *TestContext, namespace, handle string) jobs.Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resolved, err := tc.AppCtx.ResolveJobHandle(namespace, handle)
		if err == nil {
			return resolved
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the handle %s never resolved: %v", handle, lastErr)
	return jobs.Snapshot{}
}

// waitForCanonicalState polls one durable Job until it reaches a terminal state
// the predicate accepts.
func waitForCanonicalState(t *testing.T, tc *TestContext, jobID, what string, cond func(jobs.Snapshot) bool) jobs.Snapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last jobs.Snapshot
	for time.Now().Before(deadline) {
		snap, err := tc.AppCtx.GetJob(jobID)
		if err == nil {
			last = snap
			if cond(snap) {
				return snap
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; the job is %s/%s", what, last.State, last.Phase)
	return last
}

// TestGroupExportSubmissionAcceptsADurableJobBeforeDispatch is the acceptance half
// of dual publication for an export: the Job is durable, named by the response,
// and carries the Kind whose artifact the rest of this file asks about. Nothing
// may be dispatched before it exists.
func TestGroupExportSubmissionAcceptsADurableJobBeforeDispatch(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	groupID := createGroupForExport(t, tc, "export-bridge-source")

	legacyID, canonicalID := submitGroupExport(t, tc, []uint{groupID})
	if canonicalID == "" {
		t.Fatalf("the export created no durable job: legacy id %s", legacyID)
	}
	if legacyID == canonicalID {
		t.Fatalf("the legacy id %q is the canonical one: two id spaces were collapsed", legacyID)
	}

	snap := waitForCanonicalState(t, tc, canonicalID, "the export to be recorded", func(jobs.Snapshot) bool { return true })
	if snap.Kind != "group-export" {
		t.Fatalf("the export job is %q, want group-export", snap.Kind)
	}
	// The legacy handle is what the deployed download route takes, and it must
	// resolve to the Job whose artifact it serves. Resolved with a short retry because
	// this fixture's shared-cache in-memory database can refuse a read that collides with
	// the runtime's own writes — a lock the file-backed production DSN never takes.
	resolved := resolveHandleEventually(t, tc, "group-export", legacyID)
	if resolved.ID != canonicalID {
		t.Fatalf("the handle resolves to %s, want %s", resolved.ID, canonicalID)
	}
}

// TestGroupExportJobPublishesAVerifiedArtifactAndSucceeds follows one export to
// its end: the tar exists, it is published as an artifact output with an expiry,
// and the Job's success is what says the artifact is there.
func TestGroupExportJobPublishesAVerifiedArtifactAndSucceeds(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	groupID := createGroupForExport(t, tc, "export-bridge-artifact")

	_, canonicalID := submitGroupExport(t, tc, []uint{groupID})
	snap := waitForCanonicalState(t, tc, canonicalID, "the export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the export ended %s/%s", snap.State, snap.Failure)
	}

	outputs, err := tc.AppCtx.GetJobOutputs(canonicalID)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	var artifact jobs.Output
	for _, output := range outputs {
		if output.Type == jobs.OutputTypeArtifact {
			artifact = output
		}
	}
	if artifact.Key == "" {
		t.Fatalf("the succeeded export published no artifact output: %+v", outputs)
	}
	if artifact.Availability != jobs.OutputAvailable {
		t.Fatalf("the artifact is %s, want available", artifact.Availability)
	}
	if artifact.ExpiresAt == nil {
		t.Fatalf("the artifact output carries no expiry: %+v", artifact)
	}
	var reference struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(artifact.Reference, &reference); err != nil {
		t.Fatalf("decode artifact reference %s: %v", artifact.Reference, err)
	}
	if reference.Path == "" {
		t.Fatalf("the artifact reference names no path: %s", artifact.Reference)
	}
	exists, err := afero.Exists(tc.Fs, reference.Path)
	if err != nil {
		t.Fatalf("stat the published artifact: %v", err)
	}
	if !exists {
		t.Fatalf("the job succeeded but its artifact %q is not on disk", reference.Path)
	}
}

// occupyTheDeploymentBudget takes every slot of the deployment's shared job budget
// with a claim of one registered Kind, which is what a deployment at its ceiling looks
// like to a submission. It is a real claim through the public control plane rather than
// a fake: the thing being tested is that a submission meets the budget admission.
func occupyTheDeploymentBudget(t *testing.T, tc *TestContext) {
	t.Helper()
	service := tc.AppCtx.JobService()
	if service == nil {
		t.Fatal("this test needs a job control plane")
	}
	deps := jobs.Deps{DB: tc.DB}
	accepted, err := service.Accept(deps, jobs.Acceptance{
		Kind: application_context.JobKindGroupExport, KindVersion: 1, State: jobs.StateQueued,
		Origin: "test", Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept the budget holder: %v", err)
	}
	execution, claimed, err := service.Claim(context.Background(), deps, jobs.ClaimRequest{
		Kind: application_context.JobKindGroupExport, KindVersion: 1, JobID: accepted.ID,
		Claimant: "budget-holder",
		// The deployment's own budget, which is the number the submission's admission
		// asks every claim to occupy.
		Capacity: []jobs.CapacityRef{{Group: jobs.CapacityGroupGlobal, Limit: tc.AppCtx.Config.MaxJobConcurrency}},
	})
	if err != nil || !claimed {
		t.Fatalf("claim the budget holder: claimed=%v err=%v", claimed, err)
	}
	t.Cleanup(func() {
		if _, err := service.ReleaseClaim(deps, jobs.ReleaseRequest{
			ExecutionRef: jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
			To:           jobs.StateQueued,
			Reason:       "test released the budget",
		}); err != nil {
			t.Logf("releasing the budget holder: %v", err)
		}
	})
}

// TestAnExportWaitingForCapacityIsAcceptedAndNotMissing is the deployment budget at the
// routes a client actually calls.
//
// A submission with no capacity to run it is accepted durably and answers the id a
// deployed client keeps — the same handle a dispatched export answers — rather than
// being refused. What must not follow is a 404 from the archive route for that id:
// the export exists, it simply has not run yet, and "not finished" is the honest
// answer for work the client holds an id for.
func TestAnExportWaitingForCapacityIsAcceptedAndNotMissing(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	tc.AppCtx.Config.MaxJobConcurrency = 1
	occupyTheDeploymentBudget(t, tc)

	groupID := createGroupForExport(t, tc, "export-waiting-for-capacity")
	legacyID, canonicalID := submitGroupExport(t, tc, []uint{groupID})
	if canonicalID == "" {
		t.Fatalf("an export refused admission for capacity: legacy id %s", legacyID)
	}

	snap := waitForCanonicalState(t, tc, canonicalID, "the export to be recorded", func(jobs.Snapshot) bool { return true })
	if snap.State != jobs.StateQueued {
		t.Fatalf("the export is %s while the deployment has no room for it, want queued", snap.State)
	}
	if _, found := tc.AppCtx.DownloadManager().GetJobByCanonicalJobID(canonicalID); found {
		t.Fatalf("the export started an executor it had no capacity to admit")
	}

	res := tc.MakeRequest(http.MethodGet, "/v1/exports/"+legacyID+"/download", nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("the archive route answered %d for an export waiting for capacity: %s", res.Code, res.Body.String())
	}
}
