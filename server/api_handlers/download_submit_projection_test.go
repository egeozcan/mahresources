package api_handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/download_queue"
	"mahresources/models/query_models"
)

// The submission endpoint's answer is a contract with every client that has ever
// posted to it: `{"queued": true, "jobs": [{...}]}` with each row's `id` being the
// download identifier that client will poll and control.
//
// A deployment whose concurrency budget is full has no queue entry to report — the
// durable Job waits for a slot rather than being refused — so the row has to come
// from the Job itself. What this pins is that the endpoint answers the same shape
// either way, with the stable handle the Job already carries, instead of
// dereferencing an entry that does not exist yet.
//
// The row is built by the application layer (it is the same projection the
// compatibility route answers with), so what this test exercises is the handler's own
// contract: an accepted submission with no live entry still gets a row, and a refused
// one still gets the refusal path.

// projectionSubmitter answers one accepted URL the way a deployment with a full
// budget does: a durable Job, its stable handle, and no queue entry.
type projectionSubmitter struct {
	submission download_queue.RemoteDownloadSubmission
}

func (p projectionSubmitter) SubmitRemoteDownloads(_ *query_models.ResourceFromRemoteCreator, _ *uint, _, _ string) []download_queue.RemoteDownloadSubmission {
	return []download_queue.RemoteDownloadSubmission{p.submission}
}

func (p projectionSubmitter) DownloadManager() *download_queue.DownloadManager {
	panic("the submission handler must not reach for the queue manager for this request")
}

func (p projectionSubmitter) GroupVisible(uint) bool { return true }

func (p projectionSubmitter) NoteVisible(uint) bool { return true }

// TestADownloadSubmissionWithoutAQueueEntryStillAnswersARow is the capacity case seen
// from the HTTP seam.
func TestADownloadSubmissionWithoutAQueueEntryStillAnswersARow(t *testing.T) {
	row := &download_queue.DownloadJob{
		ID:              "handle-1",
		URL:             "https://example.invalid/held.bin",
		Status:          download_queue.JobStatusPending,
		ProgressPercent: -1,
		TotalSize:       -1,
		Source:          download_queue.JobSourceDownload,
		CanonicalJobID:  "job-1",
	}
	submitter := projectionSubmitter{submission: download_queue.RemoteDownloadSubmission{
		URL:            "https://example.invalid/held.bin",
		Row:            row,
		CanonicalJobID: "job-1",
	}}

	request := httptest.NewRequest(http.MethodPost, "/v1/download/submit",
		strings.NewReader(`{"URL":"https://example.invalid/held.bin"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	GetDownloadSubmitHandler(submitter)(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("the endpoint answered %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Queued bool `json:"queued"`
		Jobs   []struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			CanonicalJobID string `json:"canonicalJobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the answer: %v (%s)", err, recorder.Body.String())
	}
	if !body.Queued {
		t.Fatalf("the answer does not report the submission as queued: %s", recorder.Body.String())
	}
	if len(body.Jobs) != 1 {
		t.Fatalf("%d jobs in the answer, want one: %s", len(body.Jobs), recorder.Body.String())
	}
	if body.Jobs[0].ID != "handle-1" {
		t.Fatalf("the answered id is %q, want the handle the job carries", body.Jobs[0].ID)
	}
	if body.Jobs[0].Status != string(download_queue.JobStatusPending) {
		t.Fatalf("the answered status is %q, want %q", body.Jobs[0].Status, download_queue.JobStatusPending)
	}
	if body.Jobs[0].CanonicalJobID != "job-1" {
		t.Fatalf("the answered row names job %q, want job-1", body.Jobs[0].CanonicalJobID)
	}
}
