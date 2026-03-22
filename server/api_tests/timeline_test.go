package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"
	"time"
)

func TestTimelineAPI_EmptyDatabase_ReturnsZeroBuckets(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := tc.MakeRequest(http.MethodGet, "/v1/resources/timeline?granularity=monthly&columns=3", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(resp.Buckets))
	}

	for i, b := range resp.Buckets {
		if b.Created != 0 {
			t.Errorf("bucket[%d].Created = %d, want 0", i, b.Created)
		}
		if b.Updated != 0 {
			t.Errorf("bucket[%d].Updated = %d, want 0", i, b.Updated)
		}
	}
}

func TestTimelineAPI_WithEntities_CorrectBucketing(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag (lightweight entity that doesn't need associations)
	tag := &models.Tag{Name: "timeline-tag-1"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	// Use the tag's creation time as the anchor
	now := time.Now().UTC()
	anchor := now.Format("2006-01-02")

	rr := tc.MakeRequest(http.MethodGet, "/v1/tags/timeline?granularity=monthly&columns=3&anchor="+anchor, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(resp.Buckets))
	}

	// The rightmost bucket (current month) should have the tag we created
	lastBucket := resp.Buckets[len(resp.Buckets)-1]
	if lastBucket.Created < 1 {
		t.Errorf("last bucket created count = %d, want >= 1", lastBucket.Created)
	}
}

func TestTimelineAPI_UpdatedEntity_UpdatedCountCorrect(t *testing.T) {
	tc := SetupTestEnv(t)

	now := time.Now().UTC()
	createdTime := now.Add(-1 * time.Hour)
	updatedTime := now

	// Create a tag with explicit timestamps far enough apart to avoid precision issues
	tag := &models.Tag{Name: "timeline-updated-tag"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	// Set created_at to 1 hour ago and updated_at to now, ensuring they differ
	if err := tc.DB.Model(tag).UpdateColumns(map[string]interface{}{
		"created_at": createdTime,
		"updated_at": updatedTime,
	}).Error; err != nil {
		t.Fatalf("failed to set tag timestamps: %v", err)
	}

	anchor := updatedTime.Format("2006-01-02")
	rr := tc.MakeRequest(http.MethodGet, "/v1/tags/timeline?granularity=monthly&columns=3&anchor="+anchor, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	lastBucket := resp.Buckets[len(resp.Buckets)-1]
	if lastBucket.Updated < 1 {
		t.Errorf("last bucket updated count = %d, want >= 1 (entity was updated after creation)", lastBucket.Updated)
	}
}

func TestTimelineAPI_EntityWithSameCreatedUpdated_ExcludedFromUpdatedCount(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag (created_at == updated_at by default in GORM)
	tag := &models.Tag{Name: "timeline-same-time-tag"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	// Ensure updated_at == created_at
	if err := tc.DB.Model(tag).UpdateColumn("updated_at", tag.CreatedAt).Error; err != nil {
		t.Fatalf("failed to set updated_at equal to created_at: %v", err)
	}

	now := time.Now().UTC()
	anchor := now.Format("2006-01-02")
	rr := tc.MakeRequest(http.MethodGet, "/v1/tags/timeline?granularity=monthly&columns=3&anchor="+anchor, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	lastBucket := resp.Buckets[len(resp.Buckets)-1]
	if lastBucket.Updated != 0 {
		t.Errorf("last bucket updated count = %d, want 0 (entity with updated_at == created_at should be excluded)", lastBucket.Updated)
	}
}

func TestTimelineAPI_AllEntityTypes_Return200(t *testing.T) {
	tc := SetupTestEnv(t)

	endpoints := []string{
		"/v1/resources/timeline?granularity=monthly&columns=3",
		"/v1/notes/timeline?granularity=monthly&columns=3",
		"/v1/groups/timeline?granularity=monthly&columns=3",
		"/v1/tags/timeline?granularity=monthly&columns=3",
		"/v1/categories/timeline?granularity=monthly&columns=3",
		"/v1/queries/timeline?granularity=monthly&columns=3",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			rr := tc.MakeRequest(http.MethodGet, endpoint, nil)
			if rr.Code != http.StatusOK {
				t.Errorf("GET %s returned %d, want 200: %s", endpoint, rr.Code, rr.Body.String())
			}

			var resp models.TimelineResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Errorf("GET %s response is not valid JSON: %v", endpoint, err)
			}

			if len(resp.Buckets) != 3 {
				t.Errorf("GET %s returned %d buckets, want 3", endpoint, len(resp.Buckets))
			}
		})
	}
}

func TestTimelineAPI_GranularityOptions(t *testing.T) {
	tc := SetupTestEnv(t)

	granularities := []struct {
		name     string
		param    string
		wantCols int
	}{
		{"yearly", "yearly", 3},
		{"monthly", "monthly", 5},
		{"weekly", "weekly", 4},
	}

	for _, g := range granularities {
		t.Run(g.name, func(t *testing.T) {
			url := "/v1/tags/timeline?granularity=" + g.param + "&columns=" + itoa(g.wantCols)
			rr := tc.MakeRequest(http.MethodGet, url, nil)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp models.TimelineResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if len(resp.Buckets) != g.wantCols {
				t.Errorf("granularity=%s: got %d buckets, want %d", g.param, len(resp.Buckets), g.wantCols)
			}
		})
	}
}

func TestTimelineAPI_HasMoreIndicators(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag so there's some data
	tag := &models.Tag{Name: "hasmore-tag"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	// Anchor far in the future — the tag we created should be to the left
	rr := tc.MakeRequest(http.MethodGet, "/v1/tags/timeline?granularity=yearly&columns=2&anchor=2099-01-01", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// hasMore.left should be true because data exists before the visible window
	if !resp.HasMore.Left {
		t.Error("expected hasMore.left to be true when data exists before the visible window")
	}
}

func TestTimelineAPI_ResponseStructure(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := tc.MakeRequest(http.MethodGet, "/v1/resources/timeline?granularity=monthly&columns=2", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse as generic map to validate structure
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not valid JSON object: %v", err)
	}

	if _, ok := raw["buckets"]; !ok {
		t.Error("response missing 'buckets' field")
	}
	if _, ok := raw["hasMore"]; !ok {
		t.Error("response missing 'hasMore' field")
	}

	// Verify bucket fields
	var resp models.TimelineResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal to TimelineResponse: %v", err)
	}

	for i, b := range resp.Buckets {
		if b.Label == "" {
			t.Errorf("bucket[%d].Label is empty", i)
		}
		if b.Start.IsZero() {
			t.Errorf("bucket[%d].Start is zero", i)
		}
		if b.End.IsZero() {
			t.Errorf("bucket[%d].End is zero", i)
		}
		if !b.End.After(b.Start) {
			t.Errorf("bucket[%d].End (%v) is not after Start (%v)", i, b.End, b.Start)
		}
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
