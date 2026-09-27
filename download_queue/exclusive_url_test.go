package download_queue

import (
	"errors"
	"strconv"
	"strings"
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

// Arbitration compares the request a fetch sends, not the URL's spelling: a
// second spelling of a URL in flight waits for it on every path that asks.
func TestAnotherSpellingOfAURLInFlightIsTheSameTransfer(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	server := stallingServer(t)
	url := server.URL + "/spelled.bin#first"
	respelled := strings.Replace(server.URL, "http://", "HTTP://", 1) + "/spelled.bin#second"

	running, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-running", ExecutionToken: "t1"}})
	if err != nil {
		t.Fatalf("submit the running transfer: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return running.GetStatus() == JobStatusDownloading })

	_, err = dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: respelled}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-respelled", ExecutionToken: "t2"}, ExclusiveURL: true})
	var busy *URLActiveError
	if !errors.As(err, &busy) || busy.JobID != running.ID {
		t.Fatalf("another spelling of a URL in flight answered %v, want the running entry %s", err, running.ID)
	}
	if got := dm.OtherActiveTransfer(respelled, "job-respelled"); got != running.ID {
		t.Fatalf("OtherActiveTransfer(%q) = %q, want %s", respelled, got, running.ID)
	}
	if got, active := ActiveDownloadForURL(dm, respelled); !active || got != running.ID {
		t.Fatalf("ActiveDownloadForURL(%q) = %q, %v, want %s", respelled, got, active, running.ID)
	}
	if err := dm.Cancel(running.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitForCanonical(t, "the transfer to end", func() bool { return downloadQueueStatusTerminal(running.GetStatus()) })
}

// A held download fetches nothing, and may wait for a person indefinitely, so it
// does not make other work for its URL wait. Resuming it is what is arbitrated:
// a resume is refused while another entry is fetching the URL.
func TestAHeldDownloadDoesNotHoldItsURL(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	server := stallingServer(t)
	url := server.URL + "/held-url.bin"

	held, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-held", ExecutionToken: "t1"}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return held.GetStatus() == JobStatusDownloading })
	if err := dm.Pause(held.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitForCanonical(t, "the pause", func() bool { return held.GetStatus() == JobStatusPaused })

	if got := dm.OtherActiveTransfer(url, "job-other"); got != "" {
		t.Fatalf("a held download was reported as fetching its URL: %s", got)
	}
	if got, active := ActiveDownloadForURL(dm, url); active {
		t.Fatalf("a held download was reported as fetching its URL to the legacy paths: %s", got)
	}
	other, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "",
		SubmissionOptions{Canonical: &CanonicalRef{JobID: "job-other", ExecutionToken: "t2"}, ExclusiveURL: true})
	if err != nil {
		t.Fatalf("an exclusive submission was refused while the URL was only held: %v", err)
	}
	waitForCanonical(t, "the other transfer to start", func() bool { return other.GetStatus() == JobStatusDownloading })

	var busy *URLActiveError
	if err := dm.ResumeExclusive(held.ID); !errors.As(err, &busy) || busy.JobID != other.ID {
		t.Fatalf("resuming the held download while its URL downloads answered %v, want the other entry", err)
	}
	if status := held.GetStatus(); status != JobStatusPaused {
		t.Fatalf("the refused resume moved the held download to %s", status)
	}
	if err := dm.Cancel(other.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitForCanonical(t, "the other transfer to end", func() bool { return downloadQueueStatusTerminal(other.GetStatus()) })
	if err := dm.ResumeExclusive(held.ID); err != nil {
		t.Fatalf("resuming the held download once its URL is free: %v", err)
	}
}

// A legacy retry is arbitrated like every other start: the check and the start
// are one step, so two retries of one URL that race each other start one
// transfer, and a retry of a URL in flight is refused.
func TestTwoRetriesOfOneURLStartOneTransfer(t *testing.T) {
	for round := 0; round < 25; round++ {
		dm := createTestManager()
		dm.resourceCtx = &capturingResourceCreator{}
		// Every slot held, so a retried worker parks before it can finish and the
		// retried entry stays pending while the other retry asks.
		for i := 0; i < cap(dm.semaphore); i++ {
			dm.semaphore <- struct{}{}
		}
		url := "http://example.com/retried-" + strconv.Itoa(round) + ".bin"
		first := addTestJob(dm, "first", JobStatusFailed)
		second := addTestJob(dm, "second", JobStatusFailed)
		for _, job := range []*DownloadJob{first, second} {
			job.URL, job.Source = url, JobSourceDownload
			job.creator = &query_models.ResourceFromRemoteCreator{URL: url}
		}

		start := make(chan struct{})
		results := make(chan error, 2)
		for _, id := range []string{"first", "second"} {
			go func(id string) {
				<-start
				results <- dm.RetryExclusive(id)
			}(id)
		}
		close(start)
		var started, refused int
		for i := 0; i < 2; i++ {
			var busy *URLActiveError
			switch err := <-results; {
			case err == nil:
				started++
			case errors.As(err, &busy):
				refused++
			default:
				t.Fatalf("round %d: a retry failed: %v", round, err)
			}
		}
		if started != 1 || refused != 1 {
			t.Fatalf("round %d: %d retries started and %d were refused, want one of each", round, started, refused)
		}
		for _, job := range []*DownloadJob{first, second} {
			_ = dm.Cancel(job.ID)
		}
		for i := 0; i < cap(dm.semaphore); i++ {
			<-dm.semaphore
		}
		dm.workers.Wait()
	}
}
