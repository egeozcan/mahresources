package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/download_queue"
)

// WS9 — "Jobs and downloads cockpit".
//
// Findings 2, 40, 41 and 113 of docs/ui-bug-hunt-2026-07-29.md.
//
// Finding 2 is the last high-severity row in the campaign: a paused download could
// never be cancelled. It has three separate causes, and the HTTP status mapping is
// the one with other victims — `download_queue_handlers.go` mapped *any* manager
// error from Cancel to 404, so a state conflict was reported as a missing job.
//
// Findings 41 and 113 are template facts (which branches the panel renders, and
// what the progress bar is called), so they are asserted against the served markup
// here and measured in the browser in e2e/tests/regressions/ws9-jobs-cockpit.spec.ts.

// trickleServer serves a slow, known-length body so a download job stays in
// `downloading` long enough to be paused. The finding is specifically about a
// paused *download*; a generic (runFn) job can never be paused, because
// CanPause refuses a streaming runner.
func trickleServer(t *testing.T) *httptest.Server {
	t.Helper()
	const total = 1 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(total))
		w.WriteHeader(http.StatusOK)
		chunk := make([]byte, 4096)
		for sent := 0; sent < total; sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pausedDownloadJob submits a real download against a trickling server, waits for
// it to start, and pauses it through the API — the exact sequence in finding 2's
// reproduction steps.
func pausedDownloadJob(t *testing.T, tc *TestContext) string {
	t.Helper()
	srv := trickleServer(t)
	group := tc.CreateDummyGroup("ws9 downloads")

	res := tc.MakeRequest(http.MethodPost, "/v1/download/submit",
		map[string]any{"URL": srv.URL + "/slow.dat", "OwnerId": group.ID})
	if res.Code != http.StatusAccepted {
		t.Fatalf("submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		Jobs []struct{ ID string } `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || len(submitted.Jobs) != 1 {
		t.Fatalf("unexpected submit response %s (%v)", res.Body.String(), err)
	}
	jobID := submitted.Jobs[0].ID

	waitForJobStatus(t, tc, jobID, download_queue.JobStatusDownloading)

	if res := tc.MakeRequest(http.MethodPost, "/v1/jobs/pause?id="+jobID, nil); res.Code != http.StatusOK {
		t.Fatalf("pause answered %d: %s", res.Code, res.Body.String())
	}
	waitForJobStatus(t, tc, jobID, download_queue.JobStatusPaused)
	return jobID
}

// finishedTestJob queues a generic job that returns immediately and waits for it
// to reach `completed`, so the state-conflict assertions do not race the worker.
func finishedTestJob(t *testing.T, tc *TestContext) string {
	t.Helper()
	job, err := tc.AppCtx.DownloadManager().SubmitJob("test", "done",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			return nil
		})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusCompleted)
	return job.ID
}

func waitForJobStatus(t *testing.T, tc *TestContext, jobID string, want download_queue.JobStatus) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last download_queue.JobStatus
	for time.Now().Before(deadline) {
		job, ok := tc.AppCtx.DownloadManager().GetJob(jobID)
		if ok {
			last = job.GetStatus()
			if last == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %q (last seen %q)", jobID, want, last)
}

// ---------------------------------------------------------------- finding 2

func TestCancelPausedJob_IsAccepted(t *testing.T) {
	tc := SetupTestEnv(t)
	jobID := pausedDownloadJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id="+jobID, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("finding 2: cancelling a paused job answered %d %s, want 200", res.Code, res.Body.String())
	}

	job, ok := tc.AppCtx.DownloadManager().GetJob(jobID)
	if !ok {
		t.Fatalf("job disappeared")
	}
	if got := job.GetStatus(); got != download_queue.JobStatusCancelled {
		t.Errorf("finding 2: the paused job is %q after cancel, want cancelled", got)
	}
}

func TestCancelFinishedJob_Is409NotA404(t *testing.T) {
	tc := SetupTestEnv(t)
	jobID := finishedTestJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id="+jobID, nil)
	if res.Code != http.StatusConflict {
		t.Errorf("finding 2: cancelling a completed job answered %d, want 409 Conflict — a 404 says the job does not exist, which is a different problem for the caller to debug. body: %s",
			res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "cannot be cancelled") {
		t.Errorf("the 409 body should say what it refused, got %s", res.Body.String())
	}
}

// The control for the test above: a genuinely unknown id is still a 404, so the
// change did not simply replace one blanket status with another.
func TestCancelUnknownJob_IsStill404(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id=deadbeef", nil)
	if res.Code != http.StatusNotFound {
		t.Errorf("cancelling an unknown job answered %d, want 404. body: %s", res.Code, res.Body.String())
	}
}

func TestJobStateConflicts_AreAll409(t *testing.T) {
	// pause/resume/retry ran through statusCodeForError with a 400 fallback, and
	// its "cannot be" validation pattern claimed them — so a state conflict was a
	// Bad Request. They share the typed error with cancel now.
	for _, action := range []string{"pause", "resume", "retry"} {
		t.Run(action, func(t *testing.T) {
			tc := SetupTestEnv(t)
			// A completed job can be none of the three: pause and resume are for
			// jobs still in flight, and retry only accepts failed/cancelled.
			jobID := finishedTestJob(t, tc)

			res := tc.MakeRequest(http.MethodPost, "/v1/jobs/"+action+"?id="+jobID, nil)
			if res.Code != http.StatusConflict {
				t.Errorf("%s in the wrong state answered %d, want 409. body: %s", action, res.Code, res.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------- finding 40

func TestClearCompletedJobs_RemovesFinishedAndKeepsActive(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	finishedID := finishedTestJob(t, tc)
	pausedID := pausedDownloadJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/clearCompleted", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("finding 40: POST /v1/jobs/clearCompleted answered %d %s", res.Code, res.Body.String())
	}
	var payload struct {
		Cleared int `json:"cleared"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("clearCompleted response is not JSON: %v (%s)", err, res.Body.String())
	}
	if payload.Cleared < 1 {
		t.Errorf("finding 40: clearCompleted reported %d cleared, want at least the one completed job", payload.Cleared)
	}

	if _, ok := dm.GetJob(finishedID); ok {
		t.Errorf("finding 40: the completed job survived clearCompleted")
	}
	// Positive control in the same test: an unfinished job must not be swept away
	// with the rest. Clearing a paused job would discard a half-downloaded file.
	if _, ok := dm.GetJob(pausedID); !ok {
		t.Errorf("finding 40: clearCompleted removed a paused job")
	}
}

// ------------------------------------------------------- findings 41 and 113

// TestJobsPanel_PausedJobKeepsItsProgressAndCancel is findings 41 and 2's UI half.
// The progress block was rendered only for `downloading`, so pausing collapsed the
// row to "⏸ 100Mb.dat Paused <url> Resume" — bytes, percent, speed and bar all
// gone — and the Cancel button was gated on isActive(), which excludes paused.
func TestJobsPanel_PausedJobKeepsItsProgressAndCancel(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	bar := findOpenTag(body, `data-testid="cockpit-progressbar"`, "div")
	if bar == "" {
		t.Fatalf("the cockpit progress bar is not in the page — this test measured nothing")
	}

	// Go can read which expression gates the block; whether a *paused* job then
	// renders it is a browser question, asserted in ws9-jobs-cockpit.spec.ts, and
	// the predicate itself is unit-tested in src/components/downloadCockpit.test.js.
	progressBranch := templateCondition(body, `data-testid="cockpit-progressbar"`)
	if progressBranch == "job.status === 'downloading'" {
		t.Errorf("finding 41: the progress block's x-if is still downloading-only (%q), so pausing drops bytes, percent, speed and bar", progressBranch)
	}
	if !strings.Contains(progressBranch, "showsProgress(job)") {
		t.Errorf("finding 41: the progress block is not gated on showsProgress(job) (got %q)", progressBranch)
	}

	if !strings.Contains(body, `canCancel(job)`) {
		t.Errorf("finding 2: the Cancel button is not gated on canCancel(job), so a paused job offers no Cancel control")
	}
}

// TestJobsPanel_ProgressBarIsNamedAfterTheJob is finding 113: the accessible name
// was the literal prefix "Download progress: " plus formatProgress(job), which
// returns "" for an unknown total — so a screen reader announced an unnamed 0% bar
// for the whole transfer.
func TestJobsPanel_ProgressBarIsNamedAfterTheJob(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	bar := findOpenTag(body, `data-testid="cockpit-progressbar"`, "div")
	if bar == "" {
		t.Fatalf("the cockpit progress bar is not in the page — this test measured nothing")
	}

	if strings.Contains(bar, `'Download progress: ' + formatProgress(job)`) {
		t.Errorf("finding 113: the progress bar's name is still the bare prefix plus a value that is empty for unknown-size downloads.\ntag: %s",
			whitespaceRe.ReplaceAllString(bar, " "))
	}
	if !strings.Contains(bar, "progressLabel(job)") {
		t.Errorf("finding 113: the progress bar does not use progressLabel(job) for its accessible name.\ntag: %s",
			whitespaceRe.ReplaceAllString(bar, " "))
	}
	// An indeterminate progressbar must not claim a value. aria-valuenow bound to
	// null removes the attribute (the same idiom autocompleter.tpl already uses for
	// aria-invalid), so the binding has to be conditional rather than literal.
	if strings.Contains(bar, `aria-valuenow="0"`) || !strings.Contains(bar, `:aria-valuenow="progressValueNow(job)"`) {
		t.Errorf("finding 113: aria-valuenow is not conditional, so an unknown-size download announces a pinned 0%%.\ntag: %s",
			whitespaceRe.ReplaceAllString(bar, " "))
	}
	if !strings.Contains(bar, `:aria-valuetext="progressValueText(job)"`) {
		t.Errorf("finding 113: no aria-valuetext, so an indeterminate bar describes nothing.\ntag: %s",
			whitespaceRe.ReplaceAllString(bar, " "))
	}
}

func TestJobsPanel_HasAClearCompletedControl(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	if !strings.Contains(body, `data-testid="cockpit-clear-completed"`) {
		t.Errorf("finding 40: the jobs panel has no Clear completed control, so finished jobs accumulate forever")
	}
	if !strings.Contains(body, "clearCompleted()") {
		t.Errorf("finding 40: nothing calls clearCompleted()")
	}
}

// templateCondition returns the x-if expression of the <template> that most
// closely precedes marker in body. Used to assert which branch a block renders
// under, which is a property of the template source and therefore something Go
// can read; whether Alpine instantiated it is a browser question.
func templateCondition(body, marker string) string {
	at := strings.Index(body, marker)
	if at < 0 {
		return ""
	}
	head := body[:at]
	open := strings.LastIndex(head, "<template ")
	if open < 0 {
		return ""
	}
	const attr = `x-if="`
	from := strings.Index(body[open:at], attr)
	if from < 0 {
		return ""
	}
	rest := body[open+from+len(attr):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
