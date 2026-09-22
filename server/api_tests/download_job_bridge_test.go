package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/download_queue"
	"mahresources/jobs"
)

// This file drives the compatibility bridge at the HTTP seam: every route the
// deployed UI, CLI and API already call keeps working, resolves the identifier it
// is given, and — for a Retry — creates a new durable Job rather than re-running an
// execution that already has an outcome.
//
// It runs against a real router and real middleware, with a control plane installed
// on the test context, because the property worth pinning is that the *deployed*
// routes reach it at all.

// installJobControlPlane gives one test context the durable Job control plane the
// compatibility bridge needs: the service, the Kinds this context's executors own,
// and a replay keyring so a submission's input can be sealed.
func installJobControlPlane(t *testing.T, tc *TestContext) {
	t.Helper()
	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: "SQLITE", Ephemeral: true})
	if err != nil {
		t.Fatalf("build replay keyring: %v", err)
	}
	tc.AppCtx.SetJobReplayKeyring(ring)

	service := jobs.NewService()
	tc.AppCtx.SetJobService(service)
	for _, kind := range []string{application_context.JobKindRemoteDownload, application_context.JobKindDeferredDownload} {
		if _, ok := service.AdapterFor(kind, 1); !ok {
			t.Fatalf("installing the control plane did not register %s", kind)
		}
	}
	// The dispatch loop is what adopts a submitted transfer and publishes its
	// progress and outcome into the Job; a deployment always runs one, and a test
	// without it would be asserting against a Job nobody is executing.
	// 100ms rather than 20ms: a runtime claims once per registered Kind per tick, and
	// this fixture's database is a shared-cache in-memory one where a reader and a writer
	// of one table can collide in a way the file-backed production DSN never sees. The
	// tests that need the loop wait seconds, so a slower tick costs them nothing.
	runtime := application_context.NewJobRuntime(tc.AppCtx, service, application_context.JobRuntimeConfig{
		Claimant: "api-bridge-test",
		Interval: 100 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)
}

// submitFailingDownload submits a download against a server that fails every
// request, waits for the queue entry to fail, and returns the legacy id the client
// keeps using.
func submitFailingDownload(t *testing.T, tc *TestContext) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	res := tc.MakeRequest(http.MethodPost, "/v1/download/submit",
		map[string]any{"URL": srv.URL + "/bridge-test.bin"})
	if res.Code != http.StatusAccepted {
		t.Fatalf("submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		Jobs []struct {
			ID             string `json:"id"`
			CanonicalJobID string `json:"canonicalJobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || len(submitted.Jobs) != 1 {
		t.Fatalf("unexpected submit response %s (%v)", res.Body.String(), err)
	}
	if submitted.Jobs[0].CanonicalJobID == "" {
		t.Fatalf("the submitted row names no durable job: %s", res.Body.String())
	}

	jobID := submitted.Jobs[0].ID
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := tc.AppCtx.DownloadManager().GetJob(jobID)
		if ok && job.GetStatus() == download_queue.JobStatusFailed {
			return jobID
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("the download never failed; last status %s", func() string {
		job, _ := tc.AppCtx.DownloadManager().GetJob(jobID)
		if job == nil {
			return "missing"
		}
		return string(job.GetStatus())
	}())
	return ""
}

// waitForLegacyStatus polls one legacy row until it reports the status its own
// vocabulary uses, which is how a legacy client decides what to do next.
func waitForLegacyStatus(t *testing.T, tc *TestContext, handle, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		res := tc.MakeRequest(http.MethodGet, "/v1/jobs/get?id="+handle, nil)
		if res.Code == http.StatusOK {
			var row struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &row); err == nil {
				last = row.Status
				if row.Status == want {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("the legacy row never reached %s; it is %s", want, last)
}

// TestLegacyRetryKeepsTheHandleAndCreatesANewCanonicalJob is acceptance property 18
// at the HTTP seam: fail → legacy Retry → the same legacy id now names the new
// execution, and the new execution has its own identity.
func TestLegacyRetryKeepsTheHandleAndCreatesANewCanonicalJob(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	handle := submitFailingDownload(t, tc)

	// The canonical Job reaches its failed outcome once the dispatch loop has
	// adopted the finished transfer and published what it found.
	waitForLegacyStatus(t, tc, handle, "failed")

	// get resolves the handle and reports the canonical identity behind it.
	res := tc.MakeRequest(http.MethodGet, "/v1/jobs/get?id="+handle, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("legacy get answered %d: %s", res.Code, res.Body.String())
	}
	var before struct {
		ID             string `json:"id"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &before); err != nil {
		t.Fatalf("decode legacy get: %v", err)
	}
	if before.ID != handle || before.CanonicalJobID == "" {
		t.Fatalf("legacy get reported %+v, want id %s with a canonical id", before, handle)
	}

	res = tc.MakeRequest(http.MethodPost, "/v1/download/retry?id="+handle, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("legacy retry answered %d: %s", res.Code, res.Body.String())
	}
	var retried struct {
		Status         string `json:"status"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &retried); err != nil {
		t.Fatalf("decode legacy retry: %v", err)
	}
	if retried.Status != "retrying" {
		t.Fatalf("legacy retry answered status %q, want retrying", retried.Status)
	}
	if retried.CanonicalJobID == "" || retried.CanonicalJobID == before.CanonicalJobID {
		t.Fatalf("the retry created canonical job %q, want a new one beside %q",
			retried.CanonicalJobID, before.CanonicalJobID)
	}

	// The unchanged handle now names the successor, and reports the same id the
	// client has always used.
	res = tc.MakeRequest(http.MethodGet, "/v1/jobs/get?id="+handle, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("legacy get after the retry answered %d: %s", res.Code, res.Body.String())
	}
	var after struct {
		ID             string `json:"id"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode legacy get after the retry: %v", err)
	}
	if after.ID != handle {
		t.Fatalf("the legacy id changed to %q; a handle never becomes a new identity", after.ID)
	}
	if after.CanonicalJobID != retried.CanonicalJobID {
		t.Fatalf("the handle still names %q, want the successor %q", after.CanonicalJobID, retried.CanonicalJobID)
	}

	// A second retry while that successor is still active is refused rather than
	// forking the lineage.
	res = tc.MakeRequest(http.MethodPost, "/v1/download/retry?id="+handle, nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("a second retry answered %d: %s", res.Code, res.Body.String())
	}
}

// TestLegacyGetAndControlsRefuseAnUnknownOrHiddenId keeps the two answers apart at
// the compatibility boundary: an id that resolves to nothing is 404, and an unknown
// one cannot be told from a job somebody else owns.
func TestLegacyGetAndControlsRefuseAnUnknownOrHiddenId(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/jobs/get?id=no-such-download"},
		{http.MethodPost, "/v1/download/cancel?id=no-such-download"},
		{http.MethodPost, "/v1/download/retry?id=no-such-download"},
		{http.MethodPost, "/v1/download/resume?id=no-such-download"},
	} {
		res := tc.MakeRequest(route.method, route.path, nil)
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s %s answered %d, want 404", route.method, route.path, res.Code)
		}
	}
}
