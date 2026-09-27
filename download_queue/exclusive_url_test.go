package download_queue

import (
	"errors"
	"testing"

	"mahresources/models/query_models"
)

// An exclusive submission is refused while another entry fetches the same URL,
// and the check is made under the lock the entry is added under. An entry of the
// same durable Job is that Job's own earlier attempt, not another transfer.
func TestAnExclusiveSubmissionWaitsForTheURLToBeFree(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	server := stallingServer(t)
	url := server.URL + "/one-at-a-time.bin"

	running, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-running", ExecutionToken: "t1"}})
	if err != nil {
		t.Fatalf("submit the running transfer: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return running.GetStatus() == JobStatusDownloading })

	_, err = dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-waiting", ExecutionToken: "t2"}, ExclusiveURL: true})
	var busy *URLActiveError
	if !errors.As(err, &busy) || busy.JobID != running.ID {
		t.Fatalf("an exclusive submission of a URL in flight answered %v, want the running entry %s", err, running.ID)
	}
	if got := dm.OtherActiveTransfer(url, "job-running"); got != "" {
		t.Fatalf("the running Job's own entry was reported as another transfer: %s", got)
	}
	if got := dm.OtherActiveTransfer(url, "job-waiting"); got != running.ID {
		t.Fatalf("OtherActiveTransfer = %q, want %s", got, running.ID)
	}

	if err := dm.Cancel(running.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitForCanonical(t, "the transfer to end", func() bool { return downloadQueueStatusTerminal(running.GetStatus()) })
	if _, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-waiting", ExecutionToken: "t2"}, ExclusiveURL: true}); err != nil {
		t.Fatalf("an exclusive submission of a free URL was refused: %v", err)
	}
}
