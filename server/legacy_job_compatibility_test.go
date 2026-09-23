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
	if got := query.Get("kind"); got != "remote-download" {
		t.Fatalf("kind = %q, want remote-download", got)
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
