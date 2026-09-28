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

// A first report that already carries a count is the Job's own work, done
// before it reported: a fresh plugin Job that says "1 of 3 shares" and then
// finishes did that share, and is averaged over the time it ran.
func TestAnAverageCountsTheFirstReportedCountAsTheJobsOwn(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(3), Progress{Completed: int64Ptr(1), Total: int64Ptr(3), Unit: "shares"}, false)
	series, _ = advanceSeries(series, at(3.5), Progress{Completed: int64Ptr(1), Total: int64Ptr(3), Unit: "shares"}, true)

	snap := Snapshot{State: StateSucceeded, ProgressSeries: series, RunningDuration: 4 * time.Second}
	avg := snap.AverageRate()
	if avg == nil || math.Abs(*avg-0.25) > 1e-9 {
		t.Fatalf("average rate = %v; want 1 share over 4 s", avg)
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

// A finished Job's series ends at the last speed it measured. The points at
// its end that counted nothing since the one before are the Job no longer
// counting before it ended, not a slowdown to zero, so they record no speed,
// whichever way the closing point is placed: appended after an interval, or in
// place of a point younger than one. A stall the Job moved on from stays zero.
func TestAFinishedSeriesEndsAtItsLastMeasuredSpeed(t *testing.T) {
	counts := func(series ProgressSeries, final float64, want []any) {
		t.Helper()
		if len(series.Points) != len(want) {
			t.Fatalf("points = %+v; want %d", series.Points, len(want))
		}
		for i, point := range series.Points {
			switch rate := want[i].(type) {
			case nil:
				if point.Rate != nil {
					t.Fatalf("point %d rate = %v; want none", i, *point.Rate)
				}
			case float64:
				if point.Rate == nil || *point.Rate != rate {
					t.Fatalf("point %d rate = %v; want %v", i, point.Rate, rate)
				}
			}
		}
		if last := series.Points[len(series.Points)-1]; last.Completed == nil || *last.Completed != final {
			t.Fatalf("closing point = %+v; want the final count %v", last, final)
		}
	}
	chunks := func(n int64) Progress { return Progress{Completed: int64Ptr(n), Total: int64Ptr(10), Unit: "chunks"} }

	// Appended: 0, 3, 5 counted, then nothing for three seconds before the end.
	var appended ProgressSeries
	appended, _ = advanceSeries(appended, at(0), chunks(0), false)
	appended, _ = advanceSeries(appended, at(1), chunks(3), false)
	appended, _ = advanceSeries(appended, at(2), chunks(5), false)
	appended, _ = advanceSeries(appended, at(5), chunks(5), true)
	counts(appended, 5, []any{nil, 3.0, 2.0, nil})

	// Reported again at the same count while running, then finished within the
	// interval: the replaced tail is no plunge either.
	var replaced ProgressSeries
	replaced, _ = advanceSeries(replaced, at(0), chunks(0), false)
	replaced, _ = advanceSeries(replaced, at(1), chunks(5), false)
	replaced, _ = advanceSeries(replaced, at(2), chunks(5), false)
	replaced, _ = advanceSeries(replaced, at(3), chunks(5), false)
	replaced, _ = advanceSeries(replaced, at(3.4), chunks(5), true)
	counts(replaced, 5, []any{nil, 5.0, nil, nil})

	// A stall the Job moved on from is a real zero, and stays one.
	var resumed ProgressSeries
	resumed, _ = advanceSeries(resumed, at(0), chunks(2), false)
	resumed, _ = advanceSeries(resumed, at(1), chunks(2), false)
	resumed, _ = advanceSeries(resumed, at(2), chunks(4), false)
	resumed, _ = advanceSeries(resumed, at(3), chunks(4), true)
	counts(resumed, 4, []any{nil, 0.0, 2.0, nil})

	// While the Job runs, a stall is drawn as the zero it is.
	var running ProgressSeries
	running, _ = advanceSeries(running, at(0), chunks(2), false)
	running, _ = advanceSeries(running, at(1), chunks(2), false)
	counts(running, 2, []any{nil, 0.0})
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
