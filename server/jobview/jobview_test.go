package jobview

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
)

// TestParseFilterReadsABareDateAsAWholeLocalDay pins what the sidebar's date
// inputs mean: both bounds are inclusive, so "before" a day covers the whole of
// it, and the day is the server's, as the other lists read theirs.
func TestParseFilterReadsABareDateAsAWholeLocalDay(t *testing.T) {
	filter, err := ParseFilter(url.Values{"acceptedAfter": {"2026-09-01"}, "acceptedBefore": {"2026-09-02"}})
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	wantAfter := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).UTC()
	wantBefore := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).UTC()
	if !filter.AcceptedAfter.Equal(wantAfter) || !filter.AcceptedBefore.Equal(wantBefore) {
		t.Fatalf("bounds = %v .. %v, want %v .. %v", filter.AcceptedAfter, filter.AcceptedBefore, wantAfter, wantBefore)
	}

	instant, err := ParseFilter(url.Values{"acceptedAfter": {"2026-09-01T10:00:00+02:00"}})
	if err != nil || !instant.AcceptedAfter.Equal(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("an RFC3339 bound = %v, %v", instant.AcceptedAfter, err)
	}
	if _, err := ParseFilter(url.Values{"acceptedAfter": {"yesterday"}}); err == nil {
		t.Fatal("an unreadable bound must be refused")
	}
}

func TestParseFilterReadsRepeatedAndCommaSeparatedValues(t *testing.T) {
	filter, err := ParseFilter(url.Values{"state": {"failed", "blocked,interrupted"}, "kind": {"remote-download"}})
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if len(filter.States) != 3 || filter.States[2] != "interrupted" || len(filter.Kinds) != 1 {
		t.Fatalf("filter = %+v", filter)
	}
}

func TestCursorRoundTrips(t *testing.T) {
	cursor := jobs.Cursor{AcceptedAt: time.Date(2026, 9, 1, 8, 0, 0, 5, time.UTC), ID: "abc"}
	token, err := EncodeCursor(cursor)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	decoded, err := DecodeCursor(token)
	if err != nil || decoded != cursor {
		t.Fatalf("round trip = %+v, %v", decoded, err)
	}
	if _, err := DecodeCursor("v2:12"); err == nil {
		t.Fatal("a stream cursor must not decode as a list cursor")
	}
}

func TestResultLinkForPrefersTheEntityOutput(t *testing.T) {
	job := jobs.Snapshot{ID: "j1", Kind: "remote-download", Title: "cat.jpg", State: jobs.StateSucceeded}
	outputs := []jobs.Output{
		{Key: "log", Type: jobs.OutputTypeLog, Availability: jobs.OutputAvailable},
		{Key: "entity", Type: jobs.OutputTypeEntity, Label: "Created resource", Availability: jobs.OutputAvailable},
	}
	link := ResultLinkFor(job, outputs)
	if link.URL != "/v1/jobs/j1/outputs?key=entity" || link.Label != "View created resource" ||
		link.AccessibleLabel != "View created resource for cat.jpg" {
		t.Fatalf("link = %+v", link)
	}

	job.State = jobs.StateFailed
	if link := ResultLinkFor(job, outputs); link != (ResultLink{}) {
		t.Fatalf("an unsuccessful job offered a result link: %+v", link)
	}
	job.State = jobs.StateSucceeded
	outputs[1].Availability = jobs.OutputAvailability("removed")
	if link := ResultLinkFor(job, outputs); link != (ResultLink{}) {
		t.Fatalf("an unavailable output offered a result link: %+v", link)
	}
}

func TestResultLinkForFallsBackToAPluginActionDestination(t *testing.T) {
	reference, _ := json.Marshal(map[string]string{"redirect": "/note?id=12"})
	output := jobs.Output{Key: "result", Type: jobs.OutputTypeSummary, Availability: jobs.OutputAvailable, Reference: reference}
	job := jobs.Snapshot{ID: "j2", Kind: application_context.JobKindPluginAction, Title: "Summarize", State: jobs.StateSucceeded}
	if link := ResultLinkFor(job, []jobs.Output{output}); link.URL != "/note?id=12" || link.Label != "View result" {
		t.Fatalf("link = %+v", link)
	}
	job.Kind = "group-export"
	if link := ResultLinkFor(job, []jobs.Output{output}); link != (ResultLink{}) {
		t.Fatalf("a non-plugin summary offered a destination: %+v", link)
	}
	unsafe, _ := json.Marshal(map[string]string{"redirect": "//evil.example/note?id=1"})
	output.Reference = unsafe
	job.Kind = application_context.JobKindPluginAction
	if link := ResultLinkFor(job, []jobs.Output{output}); link != (ResultLink{}) {
		t.Fatalf("an unsafe destination was offered: %+v", link)
	}
}

// TestParseFilterReadsALocalDateTimeToItsPrecision pins the datetime inputs: a
// bound written to the minute or the second names that whole minute or second,
// the way a bare date names the whole day, so "before 15:00" includes 15:00.
func TestParseFilterReadsALocalDateTimeToItsPrecision(t *testing.T) {
	filter, err := ParseFilter(url.Values{"acceptedAfter": {"2026-09-01T14:00"}, "acceptedBefore": {"2026-09-01T15:00"}})
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	wantAfter := time.Date(2026, 9, 1, 14, 0, 0, 0, time.Local).UTC()
	wantBefore := time.Date(2026, 9, 1, 15, 1, 0, 0, time.Local).Add(-time.Nanosecond).UTC()
	if !filter.AcceptedAfter.Equal(wantAfter) || !filter.AcceptedBefore.Equal(wantBefore) {
		t.Fatalf("minute bounds = %v .. %v, want %v .. %v", filter.AcceptedAfter, filter.AcceptedBefore, wantAfter, wantBefore)
	}
	seconds, err := ParseFilter(url.Values{"acceptedBefore": {"2026-09-01T15:00:30"}})
	if err != nil || !seconds.AcceptedBefore.Equal(time.Date(2026, 9, 1, 15, 0, 31, 0, time.Local).Add(-time.Nanosecond).UTC()) {
		t.Fatalf("second bound = %v, %v", seconds.AcceptedBefore, err)
	}
}
