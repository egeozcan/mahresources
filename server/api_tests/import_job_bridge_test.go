package api_tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/archive"
	"mahresources/jobs"
	"mahresources/models"
)

// This file drives the import and maintenance Kinds at the HTTP seam: the deployed
// routes have to reach the submission funnels, or every durable property the application
// layer holds is one nothing calls.
//
// It is deliberately smaller than the application-seam suite: what is pinned here is that
// the routes accept a durable Job at all, that the id they answer with is the Job's
// handle, and that the authorization behind an import outlives the queue entry — because
// those are the parts a change to the wiring can silently drop.

// importTarForTest builds one small, valid archive to upload.
func importTarForTest(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "BridgeImported", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  1,
		Name:      "BridgeImported",
		GUID:      "99999999-8888-7777-6666-555555555555",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf.Bytes()
}

// importMultipartForTest builds the multipart body the parse route accepts.
func importMultipartForTest(t *testing.T, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "import.tar")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write the archive: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close the writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

// submitImportParseForTest uploads one archive through the deployed route and returns the
// two identifiers it answers with.
func submitImportParseForTest(t *testing.T, tc *TestContext, headers map[string]string) (legacyID, canonicalID string) {
	t.Helper()
	body, contentType := importMultipartForTest(t, importTarForTest(t))
	withType := map[string]string{"Content-Type": contentType}
	for key, value := range headers {
		withType[key] = value
	}
	rec := doReq(tc, http.MethodPost, "/v1/groups/import/parse", withType, nil, body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("parse answered %d: %s", rec.Code, rec.Body.String())
	}
	var submitted struct {
		JobID          string `json:"jobId"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &submitted); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if submitted.JobID == "" || submitted.CanonicalJobID == "" {
		t.Fatalf("the parse answered %s, want a handle and a canonical job", rec.Body.String())
	}
	return submitted.JobID, submitted.CanonicalJobID
}

// TestImportParseRouteAcceptsADurableJob pins the wiring: the deployed parse route
// answers with the durable Job's handle as its jobId and names the Job beside it, the parse
// it dispatched reaches a plan, and that plan is readable through the deployed route by the
// handle the client holds.
func TestImportParseRouteAcceptsADurableJob(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	handle, canonicalID := submitImportParseForTest(t, tc, nil)
	snap := waitForCanonicalState(t, tc, canonicalID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.Kind != "group-import-parse" {
		t.Fatalf("the parse job is %q, want group-import-parse", snap.Kind)
	}
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", snap.State, snap.Failure)
	}

	res := tc.MakeRequest(http.MethodGet, "/v1/imports/"+handle+"/plan", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("reading the plan by its handle answered %d: %s", res.Code, res.Body.String())
	}
}

// TestImportPlanReadSurvivesTheQueueEntryBeingCleared is the deployed half of "the
// authorization is durable": the owner reads their own plan after the in-memory queue entry
// is gone, because the canonical Job behind the handle still says the import is theirs —
// and a stranger still reads nothing.
func TestImportPlanReadSurvivesTheQueueEntryBeingCleared(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	ownerBearer, _ := plainUserBearer(t, tc, "import-plan-owner")

	handle, canonicalID := submitImportParseForTest(t, tc, map[string]string{"Authorization": ownerBearer})
	waitForCanonicalState(t, tc, canonicalID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	// The queue entry goes, the way it goes in practice: "Clear completed", or an hour.
	tc.AppCtx.DownloadManager().ClearFinished(nil)
	if _, stillThere := tc.AppCtx.DownloadManager().GetJob(handle); stillThere {
		t.Fatalf("precondition: the queue entry is still there, so this test measured nothing")
	}

	owner := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/plan",
		map[string]string{"Authorization": ownerBearer}, nil, nil)
	if owner.Code != http.StatusOK {
		t.Fatalf("the owner was refused after the queue entry was cleared: %d (%s)",
			owner.Code, owner.Body.String())
	}

	strangerBearer, _ := plainUserBearer(t, tc, "import-plan-stranger")
	stranger := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/plan",
		map[string]string{"Authorization": strangerBearer}, nil, nil)
	if stranger.Code != http.StatusNotFound {
		t.Fatalf("a stranger read somebody else's plan: %d (%s)", stranger.Code, stranger.Body.String())
	}
}

// TestRecomputeSimilaritiesRouteAcceptsADurableJob pins the admin route's wiring: the
// maintenance Job is accepted with the *request's* principal — which is what makes the route
// reach the funnel rather than submitting through the singleton — and the Kind fixes it as
// admin-only whatever the submitter was.
func TestRecomputeSimilaritiesRouteAcceptsADurableJob(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/admin/similarity/recompute", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("recompute answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || submitted.JobID == "" {
		t.Fatalf("the recompute answered %s (%v)", res.Body.String(), err)
	}

	page, err := tc.AppCtx.ListJobs(jobs.Filter{Kinds: []string{"similarity-recompute"}}, jobs.Cursor{}, 10)
	if err != nil {
		t.Fatalf("list the maintenance jobs: %v", err)
	}
	if len(page.Jobs) == 0 {
		t.Fatalf("the recompute route accepted no durable job")
	}
	job := page.Jobs[0]
	if job.Visibility != jobs.VisibilityAdmin {
		t.Fatalf("the maintenance job is %q-visible, want admin-only", job.Visibility)
	}
	if job.Origin != "admin" {
		t.Fatalf("the maintenance job's origin is %q, want admin", job.Origin)
	}
}

// TestExportSubmitRouteKeepsTheQueueIdAsItsHandle guards the one part of the export response
// a deployed client depends on: the id it is answered with is the one the queue entry and
// the archive route answer to.
func TestExportSubmitRouteKeepsTheQueueIdAsItsHandle(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	group := &models.Group{Name: "handle-source"}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	legacyID, canonicalID := submitGroupExport(t, tc, []uint{group.ID})
	if legacyID == canonicalID {
		t.Fatalf("the legacy id is the canonical one: two id spaces were collapsed")
	}
	waitForCanonicalState(t, tc, canonicalID, "the export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	res := tc.MakeRequest(http.MethodGet, "/v1/exports/"+legacyID+"/download", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("the export download answered %d: %s", res.Code, res.Body.String())
	}
	if res.Body.Len() == 0 {
		t.Fatalf("the export download answered an empty archive")
	}

	// An export nobody submitted is answered as missing rather than as somebody's.
	unknown := tc.MakeRequest(http.MethodGet, "/v1/exports/no-such-export/download", nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("an unknown export answered %d: %s", unknown.Code, unknown.Body.String())
	}
}

// TestAnImportApplyWaitingForCapacityAnswersItsLegacyId is the answer a client polls with.
//
// The apply route consumes the plan before it accepts anything — that is what makes a
// second /apply on one review a refusal — and it answers with the legacy id. A submission
// admitted while the deployment's budget is full has no queue entry, so the id it answers
// with is the only name that work will ever have: an empty one left the caller holding a
// 202 for an import it could neither poll nor cancel, while the controller had already
// consumed the plan it decided on.
func TestAnImportApplyWaitingForCapacityAnswersItsLegacyId(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	tc.AppCtx.Config.MaxJobConcurrency = 1

	handle, canonicalID := submitImportParseForTest(t, tc, nil)
	waitForCanonicalState(t, tc, canonicalID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateSucceeded
	})

	// The deployment's one slot is taken, so the apply is accepted to wait rather than
	// dispatched here.
	occupyTheDeploymentBudget(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/imports/"+handle+"/apply",
		map[string]any{"decisions": map[string]any{}})
	if res.Code != http.StatusAccepted {
		t.Fatalf("the apply answered %d: %s", res.Code, res.Body.String())
	}
	var applied struct {
		JobID          string `json:"jobId"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &applied); err != nil {
		t.Fatalf("decode %s: %v", res.Body.String(), err)
	}
	if applied.CanonicalJobID == "" {
		t.Fatalf("the accepted apply created no durable job: %s", res.Body.String())
	}
	if applied.JobID == "" {
		t.Fatalf("an accepted apply answered no legacy id for a client to poll: %s", res.Body.String())
	}

	// The id it answered with is the one the durable Job answers to, and the Job is
	// waiting rather than running.
	snap := waitForCanonicalState(t, tc, applied.CanonicalJobID, "the apply to be recorded", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateQueued
	})
	if snap.State != jobs.StateQueued {
		t.Fatalf("the apply is %s while the deployment has no room for it, want queued", snap.State)
	}
	resolved, err := tc.AppCtx.ResolveJobHandle(application_context.ImportApplyHandleNamespace, applied.JobID)
	if err != nil {
		t.Fatalf("the id the apply answered with resolves to no job: %v", err)
	}
	if resolved.ID != applied.CanonicalJobID {
		t.Fatalf("the answered id resolves to %s, want %s", resolved.ID, applied.CanonicalJobID)
	}
}
