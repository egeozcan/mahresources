package template_context_providers

import (
	"net/http/httptest"
	"testing"
)

func TestSavedSearchTimelinePresentation(t *testing.T) {
	r := httptest.NewRequest("GET", "/notes/timeline?timelineMode=updated&timelineGranularity=week&timelineAnchor=2026-09-01", nil)
	if hasActiveFilter(r) {
		t.Fatal("timeline presentation counted as an active filter")
	}
	r.URL.RawQuery += "&Name=needle&page=4&SortBy=Name"
	if !hasActiveFilter(r) {
		t.Fatal("name filter was ignored")
	}
	want := "/notes/timeline?SortBy=Name&timelineAnchor=2026-09-01&timelineGranularity=week&timelineMode=updated"
	if got := clearFiltersURL(r); got != want {
		t.Fatalf("clear filters = %q, want %q", got, want)
	}
	if !hasActiveFilter(httptest.NewRequest("GET", "/downloads?Error=timeout", nil)) {
		t.Fatal("Downloads error filter was ignored")
	}
}
