package template_context_providers

import (
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A live download is a row of this table too, so the filters have to be asked of
// it rather than used as a reason to hide it. Dropping every live row whenever
// any filter was set meant that searching for a download by name hid the copy of
// it that was still running — the one the user was most likely looking for.
func TestLiveRowsAreFilteredNotDropped(t *testing.T) {
	created := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	row := downloadRow{JobID: "live1", Name: "holiday.pdf", URL: "http://example.com/holiday.pdf", CreatedAt: created, Live: true}

	cases := []struct {
		name  string
		query query_models.DownloadHistoryQuery
		want  bool
	}{
		{"no filters", query_models.DownloadHistoryQuery{}, true},
		{"matching term", query_models.DownloadHistoryQuery{URL: "holiday"}, true},
		{"term matches the name in any case", query_models.DownloadHistoryQuery{URL: "HOLIDAY"}, true},
		{"non-matching term", query_models.DownloadHistoryQuery{URL: "invoice"}, false},
		{"inside the date range", query_models.DownloadHistoryQuery{CreatedAfter: "2026-03-01", CreatedBefore: "2026-03-31"}, true},
		{"before the range", query_models.DownloadHistoryQuery{CreatedAfter: "2026-04-01"}, false},
		{"after the range", query_models.DownloadHistoryQuery{CreatedBefore: "2026-03-01"}, false},
		{"malformed date drops the row", query_models.DownloadHistoryQuery{CreatedAfter: "last tuesday"}, false},
		// A live-only row has no history row behind it, so it has been run once
		// and never retried: it belongs under "never retried", and a page asking
		// for retried downloads must not print it.
		{"never retried keeps a live row", query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedNo}, true},
		{"retried drops a live row", query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedYes}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := liveRowMatchesFilter(&tc.query, row); got != tc.want {
				t.Fatalf("liveRowMatchesFilter = %v, want %v", got, tc.want)
			}
		})
	}
}

// A status filter names terminal states, so it does exclude live rows as a class
// — a running download is in none of them, and showing it would contradict the
// filter the user set.
func TestStatusFilterStillExcludesLiveRows(t *testing.T) {
	if !downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Status: []string{models.DownloadHistoryStatusFailed}}) {
		t.Fatal("a status filter must exclude live rows")
	}
	if downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{URL: "holiday"}) {
		t.Fatal("a search term must not exclude live rows wholesale; it is asked of each one")
	}
	if downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Status: []string{""}}) {
		t.Fatal("the empty \"all statuses\" option is not a filter")
	}
}

// A stored failure that a live job has moved past is relabelled with the live
// status by the merge. On a page filtered to failures that would print a row
// saying "downloading" — the filter names terminal states, and the merge must not
// smuggle a live row past it.
func TestStoredRowsRelabelledLiveAreExcludedByAStatusFilter(t *testing.T) {
	query := query_models.DownloadHistoryQuery{Status: []string{models.DownloadHistoryStatusFailed}}
	entries := []models.DownloadHistoryEntry{{
		ID: 1, JobID: "j1", URL: "http://example.com/x",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: time.Now(),
	}}

	// No live job: the stored row stands.
	if rows := buildDownloadRows(entries, map[string]*download_queue.DownloadJob{}, &query, true); len(rows) != 1 {
		t.Fatalf("stored failure rows = %d, want 1", len(rows))
	}

	// The same row, retried in place and downloading again.
	live := &download_queue.DownloadJob{ID: "j1", Status: download_queue.JobStatusDownloading}
	rows := buildDownloadRows(entries, map[string]*download_queue.DownloadJob{"j1": live}, &query, true)
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0: a live row was shown on a page filtered to %q", len(rows), models.DownloadHistoryStatusFailed)
	}
}

// A resubmission is a new job with a new id, so a row whose retry is running is
// not matched by its own job id. Without following the link the old failure keeps
// its Retry and Delete buttons while the attempt it started runs, and both answer
// 409 when pressed.
func TestRowsFollowTheirRunningRetry(t *testing.T) {
	entries := []models.DownloadHistoryEntry{{
		ID: 1, JobID: "old", LastRetryJobID: "new", URL: "http://example.com/x",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: time.Now(),
	}}
	live := map[string]*download_queue.DownloadJob{
		"new": {ID: "new", Status: download_queue.JobStatusDownloading, URL: "http://example.com/x"},
	}

	rows := buildDownloadRows(entries, live, &query_models.DownloadHistoryQuery{}, true)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: the running retry was listed as a row of its own", len(rows))
	}
	if !rows[0].Live {
		t.Fatal("the row is not marked live although the attempt it started is running")
	}
	if rows[0].Retryable || rows[0].Deletable {
		t.Fatal("the row still offers Retry/Delete while its retry runs; both would answer 409")
	}
	if rows[0].Status != string(download_queue.JobStatusDownloading) {
		t.Fatalf("status = %q, want downloading", rows[0].Status)
	}
}

// The retries filter is not a status-class exclusion: a running download can
// itself be a retry, so live rows are asked the question rather than dropped.
func TestRetriedFilterDoesNotExcludeLiveRowsWholesale(t *testing.T) {
	for _, value := range []string{query_models.DownloadRetriedYes, query_models.DownloadRetriedNo} {
		if downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Retried: value}) {
			t.Fatalf("Retried=%q must not exclude live rows as a class", value)
		}
	}

	created := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	// A live row that *is* a retry: the stored row it came from carries the
	// counters, and the merge keeps them.
	retriedLive := downloadRow{JobID: "old", Attempts: 2, CreatedAt: created, Live: true}
	if !liveRowMatchesFilter(&query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedYes}, retriedLive) {
		t.Fatal("a live row with a retry behind it must survive Retried=yes")
	}
	linkedLive := downloadRow{JobID: "old", LastRetryJobID: "new", CreatedAt: created, Live: true}
	if liveRowMatchesFilter(&query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedNo}, linkedLive) {
		t.Fatal("a row whose retry is running must not appear under Retried=no")
	}
}
