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

// The scope answers a term no stored text can contain by matching nothing, and
// the live rows merged onto the same page must answer the same. Go's substring
// search is happy to match a NUL or invalid UTF-8 in a job's own name, which
// would put a live row on a page whose stored half is empty by construction.
func TestUnmatchableTermsDropLiveRowsToo(t *testing.T) {
	created := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	row := downloadRow{JobID: "l1", Name: "a\x00b", URL: "http://example.com/a\xffb", CreatedAt: created, Live: true}
	for _, term := range []string{"\x00", "\xff"} {
		if liveRowMatchesFilter(&query_models.DownloadHistoryQuery{URL: term}, row) {
			t.Fatalf("URL=%q matched a live row; the scope answers it with no rows at all", term)
		}
	}
}

// The finish-time window and the two failure filters exclude live rows as a
// class, exactly as the status filter does — and for a reason about the row
// rather than about its values. An in-flight download has no completion time,
// and buildDownloadRows clears Error on every live row, so asking either
// question of one could only ever answer no. This is what keeps the reason
// classification in the SQL scope alone: liveRowMatchesFilter never runs it.
func TestFinishAndFailureFiltersExcludeLiveRows(t *testing.T) {
	for name, query := range map[string]query_models.DownloadHistoryQuery{
		"finished after":  {CompletedAfter: "2026-03-01"},
		"finished before": {CompletedBefore: "2026-03-01"},
		"reason":          {Reason: query_models.DownloadReasonHTTP},
		"error text":      {Error: "404"},
	} {
		if !downloadFilterExcludesLive(&query) {
			t.Errorf("%s must exclude live rows: a running download has no finish time and no stored error", name)
		}
	}

	// A stored failure whose retry is running is relabelled by the merge, and
	// must be dropped by these filters too — printing it would answer "which
	// downloads failed with a 404?" with one that says "downloading".
	entries := []models.DownloadHistoryEntry{{
		ID: 1, JobID: "j1", URL: "http://example.com/x", Error: "HTTP 404: 404 Not Found",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: time.Now(),
	}}
	live := map[string]*download_queue.DownloadJob{"j1": {ID: "j1", Status: download_queue.JobStatusDownloading}}
	query := query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonHTTP}
	if rows := buildDownloadRows(entries, live, &query, true); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0: a relabelled live row survived a reason filter", len(rows))
	}
	if rows := buildDownloadRows(entries, map[string]*download_queue.DownloadJob{}, &query, true); len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: the stored failure itself must still be listed", len(rows))
	}
}

// The card reads FinishedAt, so an unfinished download must leave it empty: the
// pointer cannot be tested in the template, where a typed nil is truthy.
func TestFinishedAtIsEmptyUntilTheDownloadEnds(t *testing.T) {
	// Distinct from the submission time on purpose: with one timestamp for both,
	// a row that formatted CreatedAt into the finish field would pass.
	started := time.Date(2026, 3, 4, 9, 15, 0, 0, time.UTC)
	done := time.Date(2026, 3, 4, 12, 30, 0, 0, time.UTC)
	entries := []models.DownloadHistoryEntry{
		{ID: 1, JobID: "unfinished", Status: models.DownloadHistoryStatusFailed, CreatedAt: started},
		{ID: 2, JobID: "finished", Status: models.DownloadHistoryStatusCompleted, CreatedAt: started, CompletedAt: &done},
	}
	rows := buildDownloadRows(entries, map[string]*download_queue.DownloadJob{}, &query_models.DownloadHistoryQuery{}, true)
	byJob := map[string]downloadRow{}
	for _, row := range rows {
		byJob[row.JobID] = row
	}
	if got := byJob["unfinished"].FinishedAt; got != "" {
		t.Fatalf("FinishedAt = %q for a download that has not finished, want empty", got)
	}
	if got := byJob["finished"].FinishedAt; got != "2026-03-04 12:30" {
		t.Fatalf("FinishedAt = %q, want 2026-03-04 12:30", got)
	}
	if got := byJob["finished"].FinishedAtISO; got != "2026-03-04T12:30:00Z" {
		t.Fatalf("FinishedAtISO = %q, want an RFC 3339 instant for the <time> element", got)
	}
	// A stored row a live job has moved past has not finished either: the merge
	// relabels it as running, and printing the previous attempt's finish time
	// beside a "downloading" badge says the download it describes is over.
	relabelled := []models.DownloadHistoryEntry{{ID: 3, JobID: "j1", Status: models.DownloadHistoryStatusFailed, CreatedAt: started, CompletedAt: &done}}
	running := map[string]*download_queue.DownloadJob{"j1": {ID: "j1", Status: download_queue.JobStatusDownloading}}
	for _, row := range buildDownloadRows(relabelled, running, &query_models.DownloadHistoryQuery{}, true) {
		if row.FinishedAt != "" {
			t.Fatalf("relabelled row FinishedAt = %q, want empty — the attempt it now describes is still running", row.FinishedAt)
		}
	}

	// Only a nil CompletedAt means unfinished. The zero time.Time is a real
	// instant in a nullable column, and rejecting it as "not finished" dropped a
	// finish time the row carried.
	zero := time.Time{}
	rows = buildDownloadRows(
		[]models.DownloadHistoryEntry{{ID: 4, JobID: "epoch", Status: models.DownloadHistoryStatusCompleted, CreatedAt: started, CompletedAt: &zero}},
		map[string]*download_queue.DownloadJob{}, &query_models.DownloadHistoryQuery{}, true,
	)
	if rows[0].FinishedAt == "" {
		t.Fatal("a stored zero completion time is a completion, not a missing one")
	}

	// A live-only row has not finished either.
	live := map[string]*download_queue.DownloadJob{"l1": {ID: "l1", Status: download_queue.JobStatusDownloading, URL: "http://example.com/l"}}
	for _, row := range buildDownloadRows(nil, live, &query_models.DownloadHistoryQuery{}, true) {
		if row.FinishedAt != "" {
			t.Fatalf("live row FinishedAt = %q, want empty", row.FinishedAt)
		}
	}
}

// The select is built from the shared table, so it cannot offer a bucket the
// scope does not implement, and "Other" is appended rather than listed there.
func TestReasonOptionsMirrorTheClassification(t *testing.T) {
	options := buildDownloadReasonOptions()
	if len(options) != len(query_models.DownloadFailureReasons)+2 {
		t.Fatalf("options = %d, want every reason plus \"any\" and \"other\"", len(options))
	}
	if options[0].Link != "" || options[len(options)-1].Link != query_models.DownloadReasonOther {
		t.Fatalf("options = %+v, want \"any\" first and \"other\" last", options)
	}
	for i, reason := range query_models.DownloadFailureReasons {
		if options[i+1].Link != reason.Key || options[i+1].Title != reason.Title {
			t.Fatalf("option %d = %+v, want %s/%s", i+1, options[i+1], reason.Key, reason.Title)
		}
	}
}

// A reason the scope does not filter on must not hide live rows. The scope is
// deliberately lenient about an unrecognised key — it keeps every stored row —
// and a page that answered the same question differently for the two sources
// would show all of the history and none of what is running.
func TestAnUnrecognisedReasonDoesNotHideLiveRows(t *testing.T) {
	if downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Reason: "not-a-reason"}) {
		t.Fatal("an unrecognised reason is not a filter, so it must not exclude live rows")
	}
	if !downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonOther}) {
		t.Fatal("\"other\" is a bucket the scope filters on")
	}
	for _, bucket := range query_models.DownloadFailureReasons {
		if !downloadFilterExcludesLive(&query_models.DownloadHistoryQuery{Reason: bucket.Key}) {
			t.Fatalf("Reason=%s must exclude live rows", bucket.Key)
		}
	}
}
