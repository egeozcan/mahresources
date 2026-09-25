package template_context_providers

import (
	"testing"
	"time"

	"mahresources/jobs"
)

func TestJobRowStatsFollowsTheJobsState(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	completed, total := int64(1<<20), int64(3<<20)
	rate := float64(512 << 10)
	start, end := 0.0, float64(1<<20)
	series := jobs.ProgressSeries{
		Unit: "bytes", Rate: &rate,
		Anchor: &jobs.RateAnchor{At: now.UnixMilli(), Completed: end},
		Points: []jobs.SeriesPoint{{At: now.Add(-4 * time.Second).UnixMilli(), Completed: &start}, {At: now.UnixMilli(), Completed: &end}},
	}
	running := jobs.Snapshot{
		State:          jobs.StateRunning,
		Progress:       jobs.Progress{Completed: &completed, Total: &total, Unit: "bytes"},
		ProgressSeries: series,
	}
	if got := jobRowStats(running, now); got != "512 KB/s · about 4 s left" {
		t.Fatalf("running stats = %q", got)
	}

	done := running
	done.State = jobs.StateSucceeded
	if got := jobRowStats(done, now); got != "average 256 KB/s" {
		t.Fatalf("finished stats = %q", got)
	}

	percent := running
	percent.Progress.Unit = "percent"
	percent.ProgressSeries.Unit = "percent"
	if got := jobRowStats(percent, now); got != "about 4 s left" {
		t.Fatalf("percent stats = %q; want no speed in percent", got)
	}
}
