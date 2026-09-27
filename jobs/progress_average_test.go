package jobs

import (
	"math"
	"testing"
	"time"
)

// A Job whose first report carried no count did everything it counts after
// that report, so its average is its whole count over the time it ran, not the
// change between its first and last counts.
func TestAFinishedJobAveragesItsCountOverItsRunningTime(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Message: "starting"}, false)
	series, _ = advanceSeries(series, at(3), Progress{Completed: int64Ptr(1), Total: int64Ptr(3), Unit: "shares"}, false)
	series, _ = advanceSeries(series, at(3.4), Progress{Completed: int64Ptr(1), Total: int64Ptr(3), Unit: "shares"}, true)

	snap := Snapshot{State: StateSucceeded, ProgressSeries: series, RunningDuration: 3500 * time.Millisecond}
	avg := snap.AverageRate()
	if avg == nil || math.Abs(*avg-1/3.5) > 1e-9 {
		t.Fatalf("average rate = %v; want 1 share over 3.5 s of running", avg)
	}
}

// A first report that already carried a count is where the Job began: a
// Continue that picks up at 120 of 500 did not do those 120 itself.
func TestAnAverageStartsFromTheCountTheFirstReportCarried(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(120), Total: int64Ptr(500), Unit: "items"}, false)
	series, _ = advanceSeries(series, at(10), Progress{Completed: int64Ptr(500), Total: int64Ptr(500), Unit: "items"}, true)

	snap := Snapshot{State: StateSucceeded, ProgressSeries: series, RunningDuration: 20 * time.Second}
	avg := snap.AverageRate()
	if avg == nil || math.Abs(*avg-380.0/20) > 1e-9 {
		t.Fatalf("average rate = %v; want 380 items over 20 s", avg)
	}
}

// Time the Job spent waiting, paused or blocked is not part of its average:
// only running time is, and that is what RunningDuration banks.
func TestAnAverageLeavesOutTimeTheJobWasNotRunning(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Phase: "queued"}, false)
	series, _ = advanceSeries(series, at(60), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(64), bytesProgress(4000), true)

	snap := Snapshot{State: StateSucceeded, ProgressSeries: series, RunningDuration: 4 * time.Second, QueueDuration: 60 * time.Second}
	avg := snap.AverageRate()
	if avg == nil || math.Abs(*avg-1000) > 1e-9 {
		t.Fatalf("average rate = %v; want 4000 bytes over 4 s of running, not over the minute it waited", avg)
	}
}

// A Job that counted nothing has no average worth showing: "average 0 passes/s"
// says less than the amount beside it already does.
func TestAJobThatCountedNothingHasNoAverage(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Total: int64Ptr(4), Unit: "passes"}, false)
	series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(0), Total: int64Ptr(4), Unit: "passes"}, true)
	snap := Snapshot{State: StateFailed, ProgressSeries: series, RunningDuration: 2 * time.Second}
	if avg := snap.AverageRate(); avg != nil {
		t.Fatalf("average rate = %v; want none for a Job that counted nothing", *avg)
	}
	if avg := series.AverageRate(); avg != nil {
		t.Fatalf("series average = %v; want none across two equal counts", *avg)
	}
}

// Without banked running time (a Job an earlier release finished, or one that
// ran in no recorded state) the average is measured across the series, as it
// was before running time was used.
func TestAnAverageWithoutRunningTimeFallsBackToTheSeries(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(2), bytesProgress(2000), true)
	snap := Snapshot{State: StateSucceeded, ProgressSeries: series}
	if avg := snap.AverageRate(); avg == nil || math.Abs(*avg-1000) > 1e-9 {
		t.Fatalf("average rate = %v; want 1000 bytes/s across the series", avg)
	}
}

// The point that closes a finished Job's series records no speed when nothing
// was counted since the one before it: the Job ended there, it did not slow to
// zero, and a graph that plunged to 0 at the end would say it had.
func TestAClosingPointWithNoNewCountRecordsNoSpeed(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Total: int64Ptr(10), Unit: "chunks"}, false)
	series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(3), Total: int64Ptr(10), Unit: "chunks"}, false)
	series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(5), Total: int64Ptr(10), Unit: "chunks"}, false)
	series, _ = advanceSeries(series, at(5), Progress{Completed: int64Ptr(5), Total: int64Ptr(10), Unit: "chunks"}, true)

	last := series.Points[len(series.Points)-1]
	if last.Completed == nil || *last.Completed != 5 {
		t.Fatalf("closing point = %+v; want the final count", last)
	}
	if last.Rate != nil {
		t.Fatalf("closing point rate = %v; want none for an end with nothing new counted", *last.Rate)
	}
	if previous := series.Points[len(series.Points)-2]; previous.Rate == nil || *previous.Rate != 2 {
		t.Fatalf("last measured point = %+v; want its 2 chunks/s kept", previous)
	}

	// A stall while the Job is still running is a real zero, and stays one.
	var running ProgressSeries
	running, _ = advanceSeries(running, at(0), Progress{Completed: int64Ptr(2), Unit: "chunks"}, false)
	running, _ = advanceSeries(running, at(1), Progress{Completed: int64Ptr(2), Unit: "chunks"}, false)
	if stalled := running.Points[1]; stalled.Rate == nil || *stalled.Rate != 0 {
		t.Fatalf("stalled point = %+v; want a rate of 0 while running", stalled)
	}
}

// Through the Service: a Job finished by Finish reports the running-time
// average, and the closing point it gets from its stored snapshot is not a
// plunge.
func TestAFinishedJobReportsItsRunningTimeAverage(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}

	clock = at(1)
	if _, err := svc.UpdateProgress(deps, ref, Progress{Message: "starting"}); err != nil {
		t.Fatal(err)
	}
	clock = at(3)
	if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(1), Total: int64Ptr(3), Unit: "shares"}); err != nil {
		t.Fatal(err)
	}
	clock = at(4)
	finished, err := svc.Finish(deps, FinishRequest{ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if finished.RunningDuration != 4*time.Second {
		t.Fatalf("running duration = %s; the test assumes the Job ran from its seeding", finished.RunningDuration)
	}
	if avg := finished.AverageRate(); avg == nil || math.Abs(*avg-0.25) > 1e-9 {
		t.Fatalf("average rate = %v; want 1 share over 4 s", avg)
	}
	points := finished.ProgressSeries.Points
	if last := points[len(points)-1]; last.Rate != nil {
		t.Fatalf("closing point = %+v; want no speed recorded for the end", last)
	}
}

// A running Job's current stint is banked only when it ends, so an average taken
// while it runs would divide by too little time.
func TestARunningJobReportsNoAverage(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(2), bytesProgress(2000), false)
	snap := Snapshot{State: StateRunning, ProgressSeries: series, RunningDuration: time.Second}
	if avg := snap.AverageRate(); avg != nil {
		t.Fatalf("average rate = %v; want none while running", *avg)
	}
}
