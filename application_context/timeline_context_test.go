package application_context

import (
	"testing"
	"time"
)

func TestGenerateBucketBoundaries_Monthly(t *testing.T) {
	// Anchor: 2025-03-15, 3 columns
	// Rightmost bucket should contain the anchor date
	anchor := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("monthly", anchor, 3)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}

	// Buckets ordered oldest-first: Jan, Feb, Mar
	expectedLabels := []string{"2025-01", "2025-02", "2025-03"}
	for i, b := range buckets {
		if b.Label != expectedLabels[i] {
			t.Errorf("bucket[%d].Label = %q, want %q", i, b.Label, expectedLabels[i])
		}
	}

	// Verify start/end dates
	// Bucket 0: Jan 2025
	wantStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	if !buckets[0].Start.Equal(wantStart) {
		t.Errorf("bucket[0].Start = %v, want %v", buckets[0].Start, wantStart)
	}
	if !buckets[0].End.Equal(wantEnd) {
		t.Errorf("bucket[0].End = %v, want %v", buckets[0].End, wantEnd)
	}

	// Bucket 2: Mar 2025
	wantStart = time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	wantEnd = time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	if !buckets[2].Start.Equal(wantStart) {
		t.Errorf("bucket[2].Start = %v, want %v", buckets[2].Start, wantStart)
	}
	if !buckets[2].End.Equal(wantEnd) {
		t.Errorf("bucket[2].End = %v, want %v", buckets[2].End, wantEnd)
	}
}

func TestGenerateBucketBoundaries_Yearly(t *testing.T) {
	anchor := time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("yearly", anchor, 3)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}

	expectedLabels := []string{"2023", "2024", "2025"}
	for i, b := range buckets {
		if b.Label != expectedLabels[i] {
			t.Errorf("bucket[%d].Label = %q, want %q", i, b.Label, expectedLabels[i])
		}
	}

	// Bucket 0: 2023
	wantStart := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !buckets[0].Start.Equal(wantStart) {
		t.Errorf("bucket[0].Start = %v, want %v", buckets[0].Start, wantStart)
	}
	if !buckets[0].End.Equal(wantEnd) {
		t.Errorf("bucket[0].End = %v, want %v", buckets[0].End, wantEnd)
	}
}

func TestGenerateBucketBoundaries_Weekly(t *testing.T) {
	// 2025-03-12 is a Wednesday. The Monday of that week is 2025-03-10.
	anchor := time.Date(2025, 3, 12, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("weekly", anchor, 2)

	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	// Bucket 1 (rightmost) should start on Monday 2025-03-10
	wantStart := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2025, 3, 17, 0, 0, 0, 0, time.UTC)
	if !buckets[1].Start.Equal(wantStart) {
		t.Errorf("bucket[1].Start = %v, want %v", buckets[1].Start, wantStart)
	}
	if !buckets[1].End.Equal(wantEnd) {
		t.Errorf("bucket[1].End = %v, want %v", buckets[1].End, wantEnd)
	}

	// Bucket 0: previous week, starting Monday 2025-03-03
	wantStart = time.Date(2025, 3, 3, 0, 0, 0, 0, time.UTC)
	wantEnd = time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	if !buckets[0].Start.Equal(wantStart) {
		t.Errorf("bucket[0].Start = %v, want %v", buckets[0].Start, wantStart)
	}
	if !buckets[0].End.Equal(wantEnd) {
		t.Errorf("bucket[0].End = %v, want %v", buckets[0].End, wantEnd)
	}

	// Weekly label format: "Mar 10" (short month + day of Monday)
	if buckets[1].Label != "Mar 10" {
		t.Errorf("bucket[1].Label = %q, want %q", buckets[1].Label, "Mar 10")
	}
	if buckets[0].Label != "Mar 03" {
		t.Errorf("bucket[0].Label = %q, want %q", buckets[0].Label, "Mar 03")
	}
}

func TestGenerateBucketBoundaries_InvalidGranularity(t *testing.T) {
	anchor := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("invalid", anchor, 2)

	// Invalid granularity defaults to monthly
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets for invalid granularity (monthly fallback), got %d", len(buckets))
	}

	expectedLabels := []string{"2025-04", "2025-05"}
	for i, b := range buckets {
		if b.Label != expectedLabels[i] {
			t.Errorf("bucket[%d].Label = %q, want %q", i, b.Label, expectedLabels[i])
		}
	}
}

func TestGenerateBucketBoundaries_ZeroColumns(t *testing.T) {
	anchor := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("monthly", anchor, 0)

	// columns=0 defaults to 15
	if len(buckets) != 15 {
		t.Fatalf("expected 15 buckets for columns=0, got %d", len(buckets))
	}

	// Rightmost bucket should contain December 2025
	lastBucket := buckets[len(buckets)-1]
	if lastBucket.Label != "2025-12" {
		t.Errorf("last bucket label = %q, want %q", lastBucket.Label, "2025-12")
	}
}

func TestGenerateBucketBoundaries_WeeklyAcrossYearBoundary(t *testing.T) {
	// 2025-01-01 is a Wednesday. Monday of that week is 2024-12-30.
	anchor := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("weekly", anchor, 2)

	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	// Rightmost bucket: week of Dec 30 (Monday)
	wantStart := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	if !buckets[1].Start.Equal(wantStart) {
		t.Errorf("bucket[1].Start = %v, want %v", buckets[1].Start, wantStart)
	}
	if !buckets[1].End.Equal(wantEnd) {
		t.Errorf("bucket[1].End = %v, want %v", buckets[1].End, wantEnd)
	}

	// Label uses the Monday date
	if buckets[1].Label != "Dec 30" {
		t.Errorf("bucket[1].Label = %q, want %q", buckets[1].Label, "Dec 30")
	}
}

func TestGenerateBucketBoundaries_MonthlyAcrossYearBoundary(t *testing.T) {
	anchor := time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("monthly", anchor, 4)

	if len(buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(buckets))
	}

	expectedLabels := []string{"2024-11", "2024-12", "2025-01", "2025-02"}
	for i, b := range buckets {
		if b.Label != expectedLabels[i] {
			t.Errorf("bucket[%d].Label = %q, want %q", i, b.Label, expectedLabels[i])
		}
	}
}
