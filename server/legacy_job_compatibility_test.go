package server

import (
	"net/url"
	"testing"
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
	if got := query["state"]; len(got) != 2 || got[0] != "failed" || got[1] != "succeeded" {
		t.Fatalf("state filters = %v, want [failed succeeded]", got)
	}
	if got := query.Get("search"); got != "example.test/a b" {
		t.Fatalf("search = %q, want trimmed URL", got)
	}
	if got := query.Get("acceptedAfter"); got != "2026-09-23T00:00:00Z" {
		t.Fatalf("acceptedAfter = %q, want UTC start of legacy date", got)
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
// is translated to the canonical states the projection reads it from, so a legacy
// link lists the Jobs a legacy client would have seen under that status.
func TestLegacyDownloadStatusesNameTheStatesTheyWereProjectedFrom(t *testing.T) {
	cases := map[string][]string{
		"pending": {"queued", "scheduled"},
		"paused":  {"paused", "blocked"},
	}
	for status, want := range cases {
		parsed, err := url.Parse(legacyDownloadsLocation(url.Values{"Status": {status}}))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := parsed.Query()["state"]
		if len(got) != len(want) {
			t.Fatalf("Status=%s = %v, want %v", status, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Status=%s = %v, want %v", status, got, want)
			}
		}
	}
}
