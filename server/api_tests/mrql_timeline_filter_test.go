package api_tests

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models/query_models"
)

// TestMRQLTimelineFilter_NarrowsAndReusesSubquery verifies the timeline count
// methods honour the MRQL filter across multiple bucket queries (the id-membership
// subquery is built once and reused per bucket).
func TestMRQLTimelineFilter_NarrowsAndReusesSubquery(t *testing.T) {
	tc := SetupTestEnv(t)

	createTaggedResource(t, tc, "v1", nil, "vacation")
	createTaggedResource(t, tc, "v2", nil, "vacation")
	createTaggedResource(t, tc, "w1", nil, "work")

	now := time.Now().UTC()
	boundaries := application_context.GenerateBucketBoundaries("yearly", now, 3)

	filtered, err := tc.AppCtx.GetResourceTimelineCounts(
		&query_models.ResourceSearchQuery{MRQL: `tags = "vacation"`}, boundaries)
	if err != nil {
		t.Fatalf("filtered timeline: %v", err)
	}
	var filteredCreated int64
	for _, b := range filtered {
		filteredCreated += b.Created
	}
	if filteredCreated != 2 {
		t.Fatalf("expected 2 vacation resources across buckets, got %d", filteredCreated)
	}

	unfiltered, err := tc.AppCtx.GetResourceTimelineCounts(&query_models.ResourceSearchQuery{}, boundaries)
	if err != nil {
		t.Fatalf("unfiltered timeline: %v", err)
	}
	var total int64
	for _, b := range unfiltered {
		total += b.Created
	}
	if total != 3 {
		t.Fatalf("expected 3 resources unfiltered, got %d", total)
	}
}

// TestMRQLTimelineFilter_InvalidExpressionIs400 verifies the timeline endpoints
// treat a bad MRQL filter as caller error (400), matching the list endpoints.
func TestMRQLTimelineFilter_InvalidExpressionIs400(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/v1/resources/timeline", "/v1/notes/timeline", "/v1/groups/timeline"} {
		q := url.Values{}
		q.Set("mrql", `tags = "unterminated`)
		rr := tc.MakeRequest(http.MethodGet, path+"?"+q.Encode(), nil)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400 for invalid mrql, got %d: %s", path, rr.Code, rr.Body.String())
		}
	}
}
