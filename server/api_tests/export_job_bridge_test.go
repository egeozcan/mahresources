package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/archive"
	"mahresources/download_queue"
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

func TestDurableExportDownloadWithholdsQueueFallbackBeforePublication(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "staged-export-root"}
	if err := tc.DB.Create(root).Error; err != nil {
		t.Fatalf("create export root: %v", err)
	}
	child := &models.Group{Name: "staged-export-child", OwnerId: &root.ID}
	if err := tc.DB.Create(child).Error; err != nil {
		t.Fatalf("create export child: %v", err)
	}
	outside := &models.Group{Name: "staged-export-outside"}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create outside group: %v", err)
	}
	owner, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "staged-export-owner", Password: "password1", Role: models.RoleUser, ScopeGroupId: &root.ID,
	})
	if err != nil {
		t.Fatalf("create scoped export owner: %v", err)
	}
	token, _, err := tc.AppCtx.CreateApiToken(owner.ID, "test", nil)
	if err != nil {
		t.Fatalf("create owner token: %v", err)
	}
	service := jobs.NewService()
	tc.AppCtx.SetJobService(service)
	const legacyID = "staged-scope-export"
	accepted, err := service.Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: application_context.JobKindGroupExport, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: &owner.ID, Title: "Export of one group",
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: application_context.GroupExportHandleNamespace, Handle: legacyID}},
	})
	if err != nil {
		t.Fatalf("accept durable export: %v", err)
	}
	var archiveBytes bytes.Buffer
	archiveWriter, err := archive.NewWriter(&archiveBytes, false)
	if err != nil {
		t.Fatalf("create staged archive: %v", err)
	}
	if err := archiveWriter.WriteManifest(&archive.Manifest{
		SchemaVersion: archive.SchemaVersion,
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 2},
		Entries: archive.Entries{Groups: []archive.GroupEntry{
			{ExportID: "g0001", Name: "root", SourceID: root.ID, Path: "groups/g0001.json"},
			{ExportID: "g0002", Name: "child", SourceID: child.ID, Path: "groups/g0002.json"},
		}},
	}); err != nil {
		t.Fatalf("write staged archive manifest: %v", err)
	}
	if err := archiveWriter.Close(); err != nil {
		t.Fatalf("close staged archive: %v", err)
	}
	path := "_exports/" + accepted.ID + ".tar"
	if err := tc.Fs.MkdirAll("_exports", 0o755); err != nil {
		t.Fatalf("create export directory: %v", err)
	}
	if err := afero.WriteFile(tc.Fs, path, archiveBytes.Bytes(), 0o600); err != nil {
		t.Fatalf("write staged archive: %v", err)
	}
	_, err = tc.AppCtx.DownloadManager().SubmitJobWithOptions(download_queue.JobOptions{
		Source: download_queue.JobSourceGroupExport, JobID: legacyID, OwnerUserID: &owner.ID,
		Canonical: &download_queue.CanonicalRef{JobID: accepted.ID}, InitialPhase: "completed",
	}, func(_ context.Context, _ *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		sink.SetResultPath(path)
		return nil
	})
	if err != nil {
		t.Fatalf("stage completed queue entry: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entry, found := tc.AppCtx.DownloadManager().GetJob(legacyID)
		if found && entry.GetStatus() == download_queue.JobStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	entry, found := tc.AppCtx.DownloadManager().GetJob(legacyID)
	if !found || entry.GetStatus() != download_queue.JobStatusCompleted {
		t.Fatalf("queue fallback is not completed: found=%v entry=%+v", found, entry)
	}
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", child.ID).Update("owner_id", outside.ID).Error; err != nil {
		t.Fatalf("move exported descendant out of scope: %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	if hidden := doReq(tc, http.MethodGet, "/v1/group?id="+itoa(int(child.ID)), headers, nil, nil); hidden.Code == http.StatusOK {
		t.Fatalf("moved child remains visible to scoped owner: %s", hidden.Body.String())
	}
	poll := doReq(tc, http.MethodGet, "/v1/jobs/get?id="+legacyID, headers, nil, nil)
	if poll.Code != http.StatusOK {
		t.Fatalf("poll staged export: %d %s", poll.Code, poll.Body.String())
	}
	var projected struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(poll.Body.Bytes(), &projected); err != nil {
		t.Fatalf("decode staged export status: %v", err)
	}
	if projected.Status == string(download_queue.JobStatusCompleted) {
		t.Fatal("legacy status completed before the scoped archive was published")
	}
	listing := doReq(tc, http.MethodGet, "/v1/jobs/queue", headers, nil, nil)
	if listing.Code != http.StatusOK {
		t.Fatalf("list staged export: %d %s", listing.Code, listing.Body.String())
	}
	var listed struct {
		Jobs []struct{ ID, Status string } `json:"jobs"`
	}
	if err := json.Unmarshal(listing.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode staged export listing: %v", err)
	}
	listedExport := false
	for _, row := range listed.Jobs {
		if row.ID == legacyID {
			listedExport = true
			if row.Status == string(download_queue.JobStatusCompleted) {
				t.Fatal("legacy queue listed an export as completed before its archive was published")
			}
		}
	}
	if !listedExport {
		t.Fatal("staged export missing from legacy queue listing")
	}
	response := doReq(tc, http.MethodGet, "/v1/exports/"+legacyID+"/download", headers, nil, nil)
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "manifest.json") {
		t.Fatalf("unpublished scoped artifact response = %d %q, want 409 with no archive bytes", response.Code, response.Body.String())
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
	installJobControlPlaneWithoutRuntime(t, tc)
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

// TestAQueuedExportIsReadableAndCancellableThroughTheJobRoutes is the compatibility
// surface of a queue-backed Kind that is waiting rather than running.
//
// `/v1/jobs/get` and `/v1/jobs/cancel` are the two routes a CLI or a bookmark uses for any
// background job, and the id they are given is the legacy handle the submission answered
// with. A Job admitted for later has no queue entry in this process at all, so an id
// space the compatibility projector did not resolve was a 404 for an id the server itself
// had just handed out — the client could neither read the work it was promised nor cancel
// it.
func TestAQueuedExportIsReadableAndCancellableThroughTheJobRoutes(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlaneWithoutRuntime(t, tc)
	tc.AppCtx.Config.MaxJobConcurrency = 1
	occupyTheDeploymentBudget(t, tc)

	groupID := createGroupForExport(t, tc, "export-queued-routes")
	legacyID, canonicalID := submitGroupExport(t, tc, []uint{groupID})
	if canonicalID == "" {
		t.Fatalf("an export refused admission for capacity: legacy id %s", legacyID)
	}
	if snap := waitForCanonicalState(t, tc, canonicalID, "the export to be recorded", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateQueued
	}); snap.State != jobs.StateQueued {
		t.Fatalf("the export is %s while the deployment has no room for it, want queued", snap.State)
	}

	// Readable through the route the CLI polls with.
	res := tc.MakeRequest(http.MethodGet, "/v1/jobs/get?id="+legacyID, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/jobs/get answered %d for an export the server accepted: %s", res.Code, res.Body.String())
	}
	var row struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		Source         string `json:"source"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &row); err != nil {
		t.Fatalf("decode /v1/jobs/get %s: %v", res.Body.String(), err)
	}
	if row.ID != legacyID {
		t.Fatalf("the row reports id %q, want the handle %q it was asked for", row.ID, legacyID)
	}
	if row.CanonicalJobID != canonicalID {
		t.Fatalf("the row names job %q, want %q", row.CanonicalJobID, canonicalID)
	}
	if row.Source != download_queue.JobSourceGroupExport {
		t.Fatalf("the row is labelled %q, want %q: an export was projected as a download",
			row.Source, download_queue.JobSourceGroupExport)
	}
	if row.Status != string(download_queue.JobStatusPending) {
		t.Fatalf("a queued export reads as %q, want %q", row.Status, download_queue.JobStatusPending)
	}

	// And cancellable through the same id, through the route a person's own control uses.
	cancel := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id="+legacyID, map[string]any{})
	if cancel.Code != http.StatusOK {
		t.Fatalf("POST /v1/jobs/cancel answered %d for an accepted export: %s", cancel.Code, cancel.Body.String())
	}
	cancelled := waitForCanonicalState(t, tc, canonicalID, "the export to be cancelled", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if cancelled.State != jobs.StateCancelled {
		t.Fatalf("the cancelled export ended %s (%+v)", cancelled.State, cancelled.Failure)
	}
}
