package application_context

import (
	"slices"
	"testing"

	"mahresources/download_queue"
	"mahresources/jobs"
)

// legacyDownloadStatuses is every status the queue reports.
var legacyDownloadStatuses = []download_queue.JobStatus{
	download_queue.JobStatusPending, download_queue.JobStatusDownloading, download_queue.JobStatusProcessing,
	download_queue.JobStatusPaused, download_queue.JobStatusCompleted, download_queue.JobStatusFailed,
	download_queue.JobStatusCancelled,
}

// Each canonical state is named by the legacy status the projection reads it as,
// and by no other, so a filter translated from the legacy vocabulary lists what a
// legacy client saw under that status: an interrupted download is failed there.
func TestEveryLegacyDownloadStatusNamesTheStatesProjectedAsIt(t *testing.T) {
	for _, state := range jobs.AllStates {
		projected := downloadStatusFromState(state)
		for _, status := range legacyDownloadStatuses {
			named := slices.Contains(LegacyDownloadStatusStates(status), state)
			readsAs := status == projected ||
				(status == download_queue.JobStatusProcessing && projected == download_queue.JobStatusDownloading)
			if named != readsAs {
				t.Errorf("status %s names %s: %v; the projection reads %s as %s", status, state, named, state, projected)
			}
		}
	}
	if got := LegacyDownloadStatusStates(download_queue.JobStatusFailed); !slices.Equal(got, []jobs.State{jobs.StateFailed, jobs.StateInterrupted}) {
		t.Fatalf("failed names %v, want failed and interrupted", got)
	}
	if got := LegacyDownloadStatusStates("unknown"); len(got) != 0 {
		t.Fatalf("an unknown status names %v, want none", got)
	}
}
