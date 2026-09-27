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

func TestParseFilterReadsBothEndsOfARelationship(t *testing.T) {
	filter, err := ParseFilter(url.Values{
		"relationship": {"repeat-of"}, "inboundRelationship": {"parent-child"}, "noInboundRelationship": {"retry-of"},
	})
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if filter.Relationship != "repeat-of" || filter.InboundRelationship != "parent-child" || filter.NoInboundRelationship != "retry-of" {
		t.Fatalf("filter = %+v", filter)
	}
}

func TestParseFilterReadsADeletedOwner(t *testing.T) {
	filter, err := ParseFilter(url.Values{"ownerDeleted": {"true"}})
	if err != nil || !filter.OwnerDeleted {
		t.Fatalf("ParseFilter(ownerDeleted=true) = %+v, %v", filter, err)
	}
	if filter, err := ParseFilter(url.Values{"ownerDeleted": {"false"}}); err != nil || filter.OwnerDeleted {
		t.Fatalf("ParseFilter(ownerDeleted=false) = %+v, %v; want not asked", filter, err)
	}
	if _, err := ParseFilter(url.Values{"ownerDeleted": {"yes"}}); err == nil {
		t.Fatal("ownerDeleted=yes was accepted")
	}
}

func TestParseFilterReadsTheViewerAsOwner(t *testing.T) {
	filter, err := ParseFilter(url.Values{"owner": {"me"}})
	if err != nil || !filter.OwnedByViewer {
		t.Fatalf("ParseFilter(owner=me) = %+v, %v", filter, err)
	}
	if _, err := ParseFilter(url.Values{"owner": {"7"}}); err == nil {
		t.Fatal("owner=7 was accepted; an id belongs in ownerId")
	}
}

// TestParseFilterReadsAnyAsNoPreference pins the one spelling of "either way"
// both preference filters share across the Job Center page, the API and the
// CLI, so a query string written on one surface means the same on the others.
func TestParseFilterReadsAnyAsNoPreference(t *testing.T) {
	for _, name := range []string{"dismissed", "pinned"} {
		filter, err := ParseFilter(url.Values{name: {"any"}})
		if err != nil {
			t.Fatalf("%s=any: %v", name, err)
		}
		if filter.Dismissed != nil || filter.Pinned != nil {
			t.Fatalf("%s=any asked a preference: %+v", name, filter)
		}
		if _, err := ParseFilter(url.Values{name: {"sometimes"}}); err == nil || err.Error() != name+" must be true, false or any" {
			t.Fatalf("%s=sometimes answered %v", name, err)
		}
	}
	filter, err := ParseFilter(url.Values{"dismissed": {"false"}, "pinned": {"true"}})
	if err != nil || filter.Dismissed == nil || *filter.Dismissed || filter.Pinned == nil || !*filter.Pinned {
		t.Fatalf("explicit preferences read as %+v, %v", filter, err)
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

// A cursor carries the order it was issued in, and a token from before there
// was a second order continues acceptance order.
func TestCursorRoundTripsInEitherOrder(t *testing.T) {
	stateEntered := jobs.Cursor{Order: jobs.OrderStateEntered, StateEnteredAt: time.Date(2026, 9, 1, 8, 0, 0, 5, time.UTC), ID: "abc"}
	token, err := EncodeCursor(stateEntered)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	if decoded, err := DecodeCursor(token); err != nil || decoded != stateEntered {
		t.Fatalf("round trip = %+v, %v; want %+v", decoded, err, stateEntered)
	}
	old, err := EncodeCursor(jobs.Cursor{AcceptedAt: stateEntered.StateEnteredAt, ID: "abc"})
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	if decoded, err := DecodeCursor(old); err != nil || decoded.Order != jobs.OrderAccepted {
		t.Fatalf("an acceptance token decoded as %+v, %v", decoded, err)
	}
}

func TestApplyListOrder(t *testing.T) {
	first, err := ApplyListOrder(url.Values{"order": {"stateEntered"}}, jobs.Cursor{})
	if err != nil || first.Order != jobs.OrderStateEntered || first.ID != "" {
		t.Fatalf("order=stateEntered on the first page = %+v, %v", first, err)
	}
	if cursor, err := ApplyListOrder(url.Values{}, first); err != nil || cursor != first {
		t.Fatalf("no order parameter changed the cursor: %+v, %v", cursor, err)
	}
	if cursor, err := ApplyListOrder(url.Values{"order": {"accepted"}}, jobs.Cursor{}); err != nil || cursor.Order != jobs.OrderAccepted {
		t.Fatalf("order=accepted = %+v, %v", cursor, err)
	}
	next := jobs.Cursor{Order: jobs.OrderStateEntered, StateEnteredAt: time.Now(), ID: "abc"}
	for _, values := range []url.Values{
		{"order": {"finishedAt"}},
		{"order": {"accepted", "stateEntered"}},
	} {
		if _, err := ApplyListOrder(values, jobs.Cursor{}); err == nil {
			t.Errorf("%v was accepted", values)
		}
	}
	if _, err := ApplyListOrder(url.Values{"order": {"accepted"}}, next); err == nil {
		t.Fatal("a state-entered cursor was read in acceptance order")
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

// TestResultLinkForNeverOffersTheResourceADuplicateCollidedWith: a download Job
// that once failed as a duplicate keeps that output when a reconciled replay of
// it succeeds, since an output cannot be withdrawn. What the Job made is its
// result; the resource it once collided with is not, and it sorts first.
func TestResultLinkForNeverOffersTheResourceADuplicateCollidedWith(t *testing.T) {
	job := jobs.Snapshot{ID: "j1", Kind: application_context.JobKindRemoteDownload, Title: "cat.jpg", State: jobs.StateSucceeded}
	existing := jobs.Output{Key: application_context.JobDownloadExistingResourceOutput, Type: jobs.OutputTypeEntity,
		Label: "Existing resource", Availability: jobs.OutputAvailable}
	created := jobs.Output{Key: "resource", Type: jobs.OutputTypeEntity, Label: "Created resource", Availability: jobs.OutputAvailable}

	if link := ResultLinkFor(job, []jobs.Output{existing, created}); link.URL != "/v1/jobs/j1/outputs?key=resource" {
		t.Fatalf("the result link is %+v, want the created resource", link)
	}
	if link := ResultLinkFor(job, []jobs.Output{existing}); link != (ResultLink{}) {
		t.Fatalf("the resource a duplicate collided with was offered as the result: %+v", link)
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

// TestResultLinkForOffersAFinishedExportsFile: what an export made is its file, so
// a finished export's row and card offer it for download the way a download's
// offer the resource it created. A file that has expired, or is no longer
// available, is not offered, and a Job that made an entity offers that first.
func TestResultLinkForOffersAFinishedExportsFile(t *testing.T) {
	job := jobs.Snapshot{ID: "j3", Kind: "group-export", Title: "Export of one group", State: jobs.StateSucceeded}
	later := time.Now().Add(time.Hour)
	archive := jobs.Output{Key: "artifact", Type: jobs.OutputTypeArtifact, Label: "Exported archive",
		Availability: jobs.OutputAvailable, ExpiresAt: &later}
	link := ResultLinkFor(job, []jobs.Output{archive})
	if link.URL != "/v1/jobs/j3/outputs?key=artifact" || link.Label != "Download exported archive" ||
		link.AccessibleLabel != "Download exported archive for Export of one group" {
		t.Fatalf("link = %+v", link)
	}

	earlier := time.Now().Add(-time.Minute)
	expired := archive
	expired.ExpiresAt = &earlier
	if link := ResultLinkFor(job, []jobs.Output{expired}); link != (ResultLink{}) {
		t.Fatalf("an expired file was offered: %+v", link)
	}
	removed := archive
	removed.Availability = jobs.OutputAvailability("removed")
	if link := ResultLinkFor(job, []jobs.Output{removed}); link != (ResultLink{}) {
		t.Fatalf("a removed file was offered: %+v", link)
	}
	entity := jobs.Output{Key: "entity", Type: jobs.OutputTypeEntity, Label: "Created group", Availability: jobs.OutputAvailable}
	if link := ResultLinkFor(job, []jobs.Output{archive, entity}); link.URL != "/v1/jobs/j3/outputs?key=entity" {
		t.Fatalf("the file was preferred over the entity the Job made: %+v", link)
	}
	job.State = jobs.StateFailed
	if link := ResultLinkFor(job, []jobs.Output{archive}); link != (ResultLink{}) {
		t.Fatalf("an unsuccessful export offered its file: %+v", link)
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
