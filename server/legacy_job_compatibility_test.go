package server

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/download_queue"
	"mahresources/server/jobview"
)

func TestLegacyDownloadsLocationCarriesOnlyEquivalentFilters(t *testing.T) {
	values := url.Values{
		"Status":         {"failed", "completed", "unknown"},
		"URL":            {"  example.test/a b  "},
		"CreatedAfter":   {"2026-09-23"},
		"CreatedBefore":  {"2026-09-24T12:30:00+02:00"},
		"Reason":         {"http"},
		"CompletedAfter": {"2026-09-20"},
	}
	location := legacyDownloadsLocation(values)
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse translated location %q: %v", location, err)
	}
	if parsed.Path != "/jobs" {
		t.Fatalf("translated path = %q, want /jobs", parsed.Path)
	}
	query := parsed.Query()
	if got := query["kind"]; len(got) != 2 || got[0] != "remote-download" || got[1] != "deferred-download" {
		t.Fatalf("kind = %v, want both download Kinds: the history listed downloads scheduled for later too", got)
	}
	if got := query.Get("dismissed"); got != "false" {
		t.Fatalf("dismissed = %q, want the Job Center's default written out", got)
	}
	if got := query["state"]; !slices.Equal(got, []string{"failed", "interrupted", "succeeded"}) {
		t.Fatalf("state filters = %v, want [failed interrupted succeeded]", got)
	}
	if got := query.Get("search"); got != "example.test/a b" {
		t.Fatalf("search = %q, want trimmed URL", got)
	}
	if got := query.Get("acceptedAfter"); got != "2026-09-23" {
		t.Fatalf("acceptedAfter = %q, want the bare date, which the Job Center reads as the whole local day", got)
	}
	if got := query.Get("acceptedBefore"); got != "2026-09-24T12:30:00+02:00" {
		t.Fatalf("acceptedBefore = %q, want original RFC3339 instant", got)
	}
	for _, unsupported := range []string{"reason", "completedAfter"} {
		if _, present := query[unsupported]; present {
			t.Errorf("unsupported legacy filter %s was translated", unsupported)
		}
	}
}

// TestLegacyDownloadStatusesNameTheStatesTheyWereProjectedFrom: each legacy status
// is translated to the canonical states the projection reads it from
// (application_context.LegacyDownloadStatusStates), so a legacy link lists the
// Jobs a legacy client would have seen under that status.
func TestLegacyDownloadStatusesNameTheStatesTheyWereProjectedFrom(t *testing.T) {
	for _, status := range []download_queue.JobStatus{
		download_queue.JobStatusPending, download_queue.JobStatusDownloading, download_queue.JobStatusProcessing,
		download_queue.JobStatusPaused, download_queue.JobStatusCompleted, download_queue.JobStatusFailed,
		download_queue.JobStatusCancelled,
	} {
		parsed, err := url.Parse(legacyDownloadsLocation(url.Values{"Status": {" " + strings.ToUpper(string(status)) + " "}}))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var want []string
		for _, state := range application_context.LegacyDownloadStatusStates(status) {
			want = append(want, string(state))
		}
		if got := parsed.Query()["state"]; len(want) == 0 || !slices.Equal(got, want) {
			t.Fatalf("Status=%s = %v, want the projected states %v", status, got, want)
		}
	}
	for status, want := range map[string][]string{
		"pending": {"scheduled", "queued"},
		"paused":  {"paused", "blocked"},
		"failed":  {"failed", "interrupted"},
	} {
		parsed, _ := url.Parse(legacyDownloadsLocation(url.Values{"Status": {status}}))
		if got := parsed.Query()["state"]; !slices.Equal(got, want) {
			t.Fatalf("Status=%s = %v, want %v", status, got, want)
		}
	}
}

// TestALegacyDateRangeOfOneDayListsThatDay pins the reading of an old bookmark for
// "the downloads of that day": a bare date is passed through as a date, which the
// Job Center reads as the whole local day at either end, rather than turned into
// UTC midnight, which made a same-day range a zero-width window.
func TestALegacyDateRangeOfOneDayListsThatDay(t *testing.T) {
	parsed, err := url.Parse(legacyDownloadsLocation(url.Values{"CreatedAfter": {"2026-09-26"}, "CreatedBefore": {"2026-09-26"}}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	filter, err := jobview.ParseFilter(parsed.Query())
	if err != nil {
		t.Fatalf("the translated filter is unreadable: %v", err)
	}
	wantStart := time.Date(2026, 9, 26, 0, 0, 0, 0, time.Local).UTC()
	wantEnd := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).UTC()
	if filter.AcceptedAfter == nil || !filter.AcceptedAfter.Equal(wantStart) ||
		filter.AcceptedBefore == nil || !filter.AcceptedBefore.Equal(wantEnd) {
		t.Fatalf("a one-day range reads %v .. %v, want %v .. %v", filter.AcceptedAfter, filter.AcceptedBefore, wantStart, wantEnd)
	}
}
