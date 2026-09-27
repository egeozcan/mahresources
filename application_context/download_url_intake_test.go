package application_context

import (
	"errors"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A batch with one line that is not a download refuses that line and accepts the
// rest, and the refused line never becomes a Job.
func TestASubmissionRefusesALineThatIsNotADownloadAndKeepsTheRest(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	server := plainContentServer(t, "intake body")

	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: "b.txt\n" + server.URL + "/kept.bin\nftp://example.com/c.bin",
	}, nil, "", "api")
	if len(submissions) != 3 {
		t.Fatalf("the batch answered %d submissions, want one per line", len(submissions))
	}
	for _, i := range []int{0, 2} {
		if !errors.Is(submissions[i].Err, download_queue.ErrInvalidDownloadURL) || submissions[i].CanonicalJobID != "" {
			t.Fatalf("line %d (%s) answered %+v, want a refusal with no Job", i, submissions[i].URL, submissions[i])
		}
	}
	if submissions[1].Err != nil || submissions[1].CanonicalJobID == "" {
		t.Fatalf("the valid line was not accepted: %+v", submissions[1])
	}
	page, err := ctx.ListJobs(jobs.Filter{Kinds: []string{JobKindRemoteDownload}}, jobs.Cursor{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Jobs) != 1 {
		t.Fatalf("%d download Jobs exist, want only the valid line's", len(page.Jobs))
	}
}

// A deferred download is refused the same way, when it is scheduled rather than
// when it comes due.
func TestADeferredDownloadRefusesALineThatIsNotADownload(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	actor, err := ctx.CreateUser(&UserInput{Username: "intake-deferrer", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "http:///no-host"}, time.Now().Add(time.Hour)); !errors.Is(err, download_queue.ErrInvalidDownloadURL) {
		t.Fatalf("a deferred download with no host answered %v", err)
	}
}
