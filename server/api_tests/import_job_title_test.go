package api_tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"mahresources/jobs"
)

// uploadImportNamedForTest uploads one archive under a file name of the caller's
// choosing and returns the parse's handle and canonical Job.
func uploadImportNamedForTest(t *testing.T, tc *TestContext, fileName string) (handle, canonicalID string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(importTarForTest(t)); err != nil {
		t.Fatalf("write the archive: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close the writer: %v", err)
	}
	rec := doReq(tc, http.MethodPost, "/v1/groups/import/parse",
		map[string]string{"Content-Type": writer.FormDataContentType()}, nil, &body)
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
	return submitted.JobID, submitted.CanonicalJobID
}

// An import's Jobs are named by the archive the person uploaded, so two imports can
// be told apart in every list, and the parse's summary says the same name rather
// than the file the server staged it as.
func TestAnImportsJobsAreTitledByTheUploadedArchive(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	handle, canonicalID := uploadImportNamedForTest(t, tc, "photos 2026.tar")
	parse := waitForCanonicalState(t, tc, canonicalID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if parse.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", parse.State, parse.Failure)
	}
	if parse.Title != "Import of photos 2026.tar" {
		t.Fatalf("parse title = %q, want %q", parse.Title, "Import of photos 2026.tar")
	}
	var summary struct {
		FileName string `json:"fileName"`
	}
	if err := json.Unmarshal(parse.Summary, &summary); err != nil || summary.FileName != "photos 2026.tar" {
		t.Fatalf("parse summary = %s, want it to name the uploaded file", parse.Summary)
	}

	res := tc.MakeRequest(http.MethodPost, "/v1/imports/"+handle+"/apply", map[string]any{})
	if res.Code != http.StatusAccepted {
		t.Fatalf("the apply answered %d: %s", res.Code, res.Body.String())
	}
	var applied struct {
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &applied); err != nil {
		t.Fatalf("decode %s: %v", res.Body.String(), err)
	}
	apply := waitForCanonicalState(t, tc, applied.CanonicalJobID, "the apply to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if apply.Title != "Apply import of photos 2026.tar" {
		t.Fatalf("apply title = %q, want %q", apply.Title, "Apply import of photos 2026.tar")
	}
}

// A browser that sends the client's whole path names the file by its last element,
// and a name that is nothing but a path still gets the Kind's own title.
func TestAnImportTitleKeepsOnlyTheUploadedFilesOwnName(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	for fileName, want := range map[string]string{
		`C:\Users\someone\exports\trip.tar`: "Import of trip.tar",
		"../../":                            "Group import",
	} {
		_, canonicalID := uploadImportNamedForTest(t, tc, fileName)
		parse := waitForCanonicalState(t, tc, canonicalID, "the parse to be recorded", func(jobs.Snapshot) bool { return true })
		if parse.Title != want {
			t.Errorf("an upload named %q is titled %q, want %q", fileName, parse.Title, want)
		}
	}
}

// The owner of a parsed import reopens its review from the parse's Job, and another
// user can learn nothing about it: the link, the plan and the handle answer them
// exactly as a handle that does not exist does.
func TestTheOwnerReopensAnImportReviewAndAStrangerFindsNothing(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	ownerBearer, _ := plainUserBearer(t, tc, "import-review-owner")
	strangerBearer, _ := plainUserBearer(t, tc, "import-review-stranger")

	handle, canonicalID := submitImportParseForTest(t, tc, map[string]string{"Authorization": ownerBearer})
	if parse := waitForCanonicalState(t, tc, canonicalID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	}); parse.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", parse.State, parse.Failure)
	}

	as := func(bearer string) map[string]string {
		return map[string]string{"Authorization": bearer, "Accept": "application/json"}
	}
	link := doReq(tc, http.MethodGet, "/v1/jobs/"+canonicalID+"/outputs?key=review", as(ownerBearer), nil, nil)
	if link.Code != http.StatusSeeOther || link.Header().Get("Location") != "/admin/import?job="+handle {
		t.Fatalf("the owner's review link answered %d to %q, want a redirect to the review",
			link.Code, link.Header().Get("Location"))
	}
	if plan := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/plan", as(ownerBearer), nil, nil); plan.Code != http.StatusOK {
		t.Fatalf("the owner could not read the plan the review restores: %d %s", plan.Code, plan.Body.String())
	}

	for _, probe := range []struct{ path, missing string }{
		{"/v1/jobs/" + canonicalID + "/outputs?key=review", "/v1/jobs/01a0e1d9-0000-7000-8000-000000000000/outputs?key=review"},
		{"/v1/imports/" + handle + "/plan", "/v1/imports/imp-does-not-exist/plan"},
		{"/v1/imports/" + handle + "/result", "/v1/imports/imp-does-not-exist/result"},
		{"/v1/jobs/get?id=" + handle, "/v1/jobs/get?id=imp-does-not-exist"},
	} {
		stranger := doReq(tc, http.MethodGet, probe.path, as(strangerBearer), nil, nil)
		missing := doReq(tc, http.MethodGet, probe.missing, as(strangerBearer), nil, nil)
		if stranger.Code != http.StatusNotFound || stranger.Code != missing.Code ||
			strings.TrimSpace(stranger.Body.String()) != strings.TrimSpace(missing.Body.String()) {
			t.Errorf("GET %s answered a stranger %d %q, want what a missing one answers: %d %q",
				probe.path, stranger.Code, stranger.Body.String(), missing.Code, missing.Body.String())
		}
	}
}
