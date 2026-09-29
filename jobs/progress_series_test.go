package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/models"
	"math"
	"strings"
	"testing"
	"time"

	"mahresources/models/types"
)

func f64(v float64) *float64 { return &v }

var seriesEpoch = time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC)

func at(seconds float64) time.Time {
	return seriesEpoch.Add(time.Duration(seconds * float64(time.Second)))
}

func bytesProgress(completed int64, metrics ...Metric) Progress {
	return Progress{Completed: int64Ptr(completed), Unit: "bytes", Metrics: metrics}
}

func TestAdvanceSeriesAppendsAtMostOnePointPerInterval(t *testing.T) {
	var series ProgressSeries
	var changed bool
	series, changed = advanceSeries(series, at(0), bytesProgress(0), false)
	if !changed || len(series.Points) != 1 {
		t.Fatalf("first tick: changed=%v points=%d; want one point", changed, len(series.Points))
	}
	series, _ = advanceSeries(series, at(0.5), bytesProgress(500), false)
	if len(series.Points) != 1 {
		t.Fatalf("a tick inside the interval added a point: %d points", len(series.Points))
	}
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	if len(series.Points) != 2 {
		t.Fatalf("a tick one interval later did not add a point: %d points", len(series.Points))
	}
	last := series.Points[1]
	if last.Rate == nil || math.Abs(*last.Rate-1000) > 1e-9 {
		t.Fatalf("point rate = %v; want 1000 bytes/s", last.Rate)
	}
	if last.At != at(1).UnixMilli() || last.Completed == nil || *last.Completed != 1000 {
		t.Fatalf("point = %+v; want t=%d c=1000", last, at(1).UnixMilli())
	}
}

func TestAdvanceSeriesSmoothsTheCurrentRate(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	if got := series.CurrentRate(at(1)); got == nil || *got != 1000 {
		t.Fatalf("rate after one second = %v; want 1000", got)
	}
	series, _ = advanceSeries(series, at(2), bytesProgress(4000), false)
	// Halfway between the previous 1000/s and the new 3000/s.
	if got := series.CurrentRate(at(2)); got == nil || *got != 2000 {
		t.Fatalf("smoothed rate = %v; want 2000", got)
	}
	if got := series.CurrentRate(at(2).Add(rateStaleAfter + time.Second)); got != nil {
		t.Fatalf("a rate nobody refreshed for longer than %v is still reported: %v", rateStaleAfter, *got)
	}
}

func TestAdvanceSeriesDoesNotChargeAPauseToTheRate(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	// Ten minutes paused, then one more second of transfer.
	series, _ = advanceSeries(series, at(601), bytesProgress(2000), false)
	last := series.Points[len(series.Points)-1]
	if last.Rate != nil {
		t.Fatalf("the point after a pause averaged the pause into its rate: %v", *last.Rate)
	}
	if got := series.CurrentRate(at(601)); got != nil {
		t.Fatalf("the current rate survived a pause: %v", *got)
	}
	series, _ = advanceSeries(series, at(602), bytesProgress(3000), false)
	if got := series.CurrentRate(at(602)); got == nil || *got != 1000 {
		t.Fatalf("rate after resuming = %v; want 1000", got)
	}
}

func TestUnchangedCountDoesNotRebaseItsRateAnchor(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	for second := 2; second <= 10; second++ {
		series, _ = advanceSeries(series, at(float64(second)), bytesProgress(1000), false)
	}
	if series.Anchor == nil || series.Anchor.At != at(1).UnixMilli() {
		t.Fatalf("unchanged count moved the rate anchor: %+v; want the last count change at %d", series.Anchor, at(1).UnixMilli())
	}
	if got := series.CurrentRate(at(11.001)); got != nil {
		t.Fatalf("unchanged count kept the old rate fresh past its count deadline: %v", *got)
	}
	series, _ = advanceSeries(series, at(11.001), bytesProgress(1000), false)
	for second := 12; second <= 15; second++ {
		series, _ = advanceSeries(series, at(float64(second)), bytesProgress(1000), false)
	}
	if got := series.CurrentRate(at(15)); got != nil {
		t.Fatalf("same-count heartbeats recreated a rate after its stale window: %v", *got)
	}
	if series.Anchor == nil || series.Anchor.At != at(11.001).UnixMilli() {
		t.Fatalf("same-count heartbeats repeatedly moved the empty anchor: %+v", series.Anchor)
	}
}

func TestHLSActivityKeepsSegmentRateFreshBetweenBatches(t *testing.T) {
	var series ProgressSeries
	segmentProgress := func(completed int64, activity bool) Progress {
		return Progress{Completed: int64Ptr(completed), Total: int64Ptr(24), Unit: "items", Activity: activity}
	}
	series, _ = advanceSeries(series, at(0), segmentProgress(0, false), false)
	for second := 1; second < 15; second++ {
		series, _ = advanceSeries(series, at(float64(second)), segmentProgress(0, true), false)
	}
	series, _ = advanceSeries(series, at(15), segmentProgress(4, true), false)
	for second := 16; second < 30; second++ {
		series, _ = advanceSeries(series, at(float64(second)), segmentProgress(4, true), false)
	}
	series, _ = advanceSeries(series, at(30), segmentProgress(8, true), false)

	rate := series.CurrentRate(at(30))
	if rate == nil || math.Abs(*rate-4.0/15.0) > 1e-9 {
		t.Fatalf("rate after two slow segment batches = %v; want about 0.267 segments/s", rate)
	}
	if eta := EstimateETA(segmentProgress(8, true), rate, at(30)); eta == nil || !eta.After(at(30)) {
		t.Fatalf("ETA after two active batches = %v; want a future finish estimate", eta)
	}
	for i, point := range series.Points {
		if point.At >= at(15).UnixMilli() && (point.Rate == nil || *point.Rate <= 0) {
			t.Fatalf("point %d lost the active segment rate: %+v", i, point)
		}
	}
	if got := series.CurrentRate(at(40)); got == nil {
		t.Fatal("activity at t=30 was stale exactly 10 seconds later")
	}
	if got := series.CurrentRate(at(40.001)); got != nil {
		t.Fatalf("activity kept the rate fresh after its stale window: %v", *got)
	}

	// The next bytes arrive after an 11-second stall. They establish a new
	// anchor and cannot bridge that idle interval into a plausible-looking rate.
	series, _ = advanceSeries(series, at(41), segmentProgress(12, true), false)
	if got := series.CurrentRate(at(41)); got != nil {
		t.Fatalf("rate bridged an unobserved stall: %v", *got)
	}
}

func TestHLSActivityHeartbeatsKeepServiceRateETAAndGraphAliveAcrossSlowBatches(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-hls-activity")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-hls-activity"}
	total := int64(24)
	var snap Snapshot
	for second := int64(0); second <= 30; second++ {
		clock = at(float64(second))
		completed := int64(0)
		if second >= 15 {
			completed = 4
		}
		if second == 30 {
			completed = 8
		}
		_, err := svc.UpdateProgress(deps, ref, Progress{
			Phase: "downloading segments", Completed: &completed, Total: &total,
			Unit: "items", Activity: second > 0,
			Metrics: []Metric{{Key: "downloaded", Label: "Downloaded", Value: float64(second) * 1024, Unit: "bytes", Graph: true}},
		})
		if err != nil {
			t.Fatalf("UpdateProgress at %ds: %v", second, err)
		}
		snap, err = svc.Get(deps, Access{Administrator: true}, job.ID)
		if err != nil {
			t.Fatalf("Get at %ds: %v", second, err)
		}
	}
	if snap.Progress.Completed == nil || *snap.Progress.Completed != 8 {
		t.Fatalf("service progress completed = %v; want 8 segments", snap.Progress.Completed)
	}
	if rate := snap.LiveRate(clock); rate == nil || *rate <= 0 {
		t.Fatalf("service live rate after active 15-second batches = %v", rate)
	}
	if eta, estimated := snap.ExpectedFinish(clock); eta == nil || !estimated {
		t.Fatalf("service ETA after active batches = %v estimated=%v; want an estimate", eta, estimated)
	}
	positive := false
	for _, point := range snap.ProgressSeries.Points {
		if point.At >= at(15).UnixMilli() && point.Rate != nil && *point.Rate > 0 {
			positive = true
		}
	}
	if !positive {
		t.Fatalf("service graph has no positive rate after a segment batch: %+v", snap.ProgressSeries.Points)
	}

	if rate := snap.LiveRate(at(41)); rate != nil {
		t.Fatalf("service live rate survived 11 seconds without HLS activity: %v", *rate)
	}
	clock = at(41)
	completed := int64(12)
	if _, err := svc.UpdateProgress(deps, ref, Progress{
		Phase: "downloading segments", Completed: &completed, Total: &total,
		Unit: "items", Activity: true,
	}); err != nil {
		t.Fatalf("UpdateProgress after the stall: %v", err)
	}
	stalled, err := svc.Get(deps, Access{Administrator: true}, job.ID)
	if err != nil {
		t.Fatalf("Get after the stall: %v", err)
	}
	if rate := stalled.LiveRate(clock); rate != nil {
		t.Fatalf("first bytes after an 11-second stall bridged the old count window: %v", *rate)
	}
	last := stalled.ProgressSeries.Points[len(stalled.ProgressSeries.Points)-1]
	if last.Rate != nil {
		t.Fatalf("first post-stall graph point reused the old rate: %+v", last)
	}
}

func TestHLSActivityRetainsNewCountMeasuredAtAssembly(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-hls-assembly-count")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-hls-assembly-count"}
	total := int64(4)
	for second := int64(0); second < 15; second++ {
		clock = at(float64(second))
		completed := int64(0)
		_, err := svc.UpdateProgress(deps, ref, Progress{
			Phase: "downloading segments", Completed: &completed, Total: &total,
			Unit: "items", Activity: second > 0,
		})
		if err != nil {
			t.Fatalf("UpdateProgress at %ds: %v", second, err)
		}
	}
	clock = at(15)
	completed := int64(4)
	snap, err := svc.UpdateProgress(deps, ref, Progress{
		Phase: "downloading segments", Message: "assembling video",
		Completed: &completed, Total: &total, Unit: "items",
	})
	if err != nil {
		t.Fatalf("UpdateProgress at assembly: %v", err)
	}
	if rate := snap.LiveRate(clock); rate == nil || math.Abs(*rate-4.0/15.0) > 1e-9 {
		t.Fatalf("live rate at assembly = %v; want 4/15 items/s", rate)
	}
	last := snap.ProgressSeries.Points[len(snap.ProgressSeries.Points)-1]
	if last.Completed == nil || *last.Completed != float64(completed) || last.Rate == nil || *last.Rate <= 0 {
		t.Fatalf("assembly graph point = %+v; want the measured 4-of-4 count and positive rate", last)
	}
	if snap.ProgressSeries.ActivityAt != nil {
		t.Fatalf("assembly kept a segment activity lease: %d", *snap.ProgressSeries.ActivityAt)
	}
}

func TestDelayedHLSActivityMirrorUsesTheOriginalReadTime(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
	series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	readAt := at(2)
	series, _ = advanceSeries(series, at(2), Progress{
		Completed: int64Ptr(1), Unit: "items", Activity: true, ActivityAt: &readAt,
	}, false)
	if got := series.CurrentRate(at(12)); got == nil {
		t.Fatal("the source read was not fresh at the ten-second boundary")
	}

	// A throttled completion or key-phase callback can arrive long after the
	// last media bytes. Replaying the old activity flag must not restart its
	// freshness window from the callback time.
	series, _ = advanceSeries(series, at(13), Progress{
		Completed: int64Ptr(1), Unit: "items", Activity: true, ActivityAt: &readAt,
	}, false)
	if got := series.CurrentRate(at(13)); got != nil {
		t.Fatalf("a delayed mirror revived stale segment activity: %v", *got)
	}
	if series.ActivityAt != nil {
		t.Fatalf("stale source activity remained recorded at %d", *series.ActivityAt)
	}
}

func TestHLSActivityMovementSurvivesFinalReplacementAndCompaction(t *testing.T) {
	segmentProgress := func(completed int64, activity bool) Progress {
		return Progress{Completed: int64Ptr(completed), Total: int64Ptr(4), Unit: "items", Activity: activity}
	}

	t.Run("final replacement", func(t *testing.T) {
		var series ProgressSeries
		series, _ = advanceSeries(series, at(0), segmentProgress(0, false), false)
		series, _ = advanceSeries(series, at(1), segmentProgress(1, true), false)
		pointsBeforeFinal := len(series.Points)
		series, _ = advanceSeries(series, at(1.2), segmentProgress(1, false), true)
		if len(series.Points) != pointsBeforeFinal {
			t.Fatalf("final point at 1.2s appended %d points, want replacement of the point at 1s", len(series.Points))
		}
		last := series.Points[len(series.Points)-1]
		if last.At != at(1.2).UnixMilli() || last.Completed == nil || *last.Completed != 1 || last.Rate == nil || *last.Rate <= 0 {
			t.Fatalf("final point = %+v; want the real 0-to-1 movement retained in the sub-interval replacement", last)
		}
		if series.Rate != nil || series.Anchor != nil {
			t.Fatalf("final series still exposes a live measurement: %+v", series)
		}
	})

	t.Run("compaction", func(t *testing.T) {
		const nowSecond = MaxSeriesPoints
		series := ProgressSeries{IntervalMs: 1000, Unit: "items", Points: make([]SeriesPoint, MaxSeriesPoints)}
		for i := range series.Points {
			completed := 0.0
			series.Points[i] = SeriesPoint{At: at(float64(i)).UnixMilli(), Completed: &completed}
		}
		anchorAt := at(float64(nowSecond - 15)).UnixMilli()
		anchorCompleted := 0.0
		series.Anchor = &RateAnchor{At: anchorAt, Completed: anchorCompleted}
		activityAt := at(float64(nowSecond - 1)).UnixMilli()
		series.ActivityAt = &activityAt
		completed := int64(4)
		series, _ = advanceSeries(series, at(float64(nowSecond)), Progress{
			Completed: &completed, Unit: "items",
		}, false)
		if len(series.Points) > MaxSeriesPoints {
			t.Fatalf("compaction left %d points, over the %d point bound", len(series.Points), MaxSeriesPoints)
		}
		last := series.Points[len(series.Points)-1]
		if last.Completed == nil || *last.Completed != 4 || last.Rate == nil || *last.Rate <= 0 {
			t.Fatalf("compacted last point = %+v; want the new count movement retained", last)
		}
	})
}

func TestFinishRetainsMovementOnASameCountFinalReplacementForEveryOutcome(t *testing.T) {
	tests := []struct {
		name    string
		outcome State
		failure *Failure
	}{
		{name: "cancelled", outcome: StateCancelled},
		{name: "succeeded", outcome: StateSucceeded},
		{name: "failed", outcome: StateFailed, failure: &Failure{Code: "test-failed", Class: FailureClassInternal}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			svc := NewService()
			clock := at(0)
			deps.Now = func() time.Time { return clock }
			job := seededExecution(t, deps, StateRunning, "claim-final-replacement")
			ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-final-replacement"}
			if _, err := svc.UpdateProgress(deps, ref, Progress{
				Completed: int64Ptr(0), Total: int64Ptr(4), Unit: "items",
			}); err != nil {
				t.Fatalf("seed zero count: %v", err)
			}

			clock = at(1)
			moving, err := svc.UpdateProgress(deps, ref, Progress{
				Completed: int64Ptr(1), Total: int64Ptr(4), Unit: "items", Activity: true,
			})
			if err != nil {
				t.Fatalf("report the measured count: %v", err)
			}
			if positiveRatePointCount(moving.ProgressSeries) == 0 {
				t.Fatal("setup did not record the actual 0-to-1 movement")
			}

			clock = at(1.2)
			ended, err := svc.Finish(deps, FinishRequest{
				ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: tt.outcome, Failure: tt.failure,
			})
			if err != nil {
				t.Fatalf("Finish %s: %v", tt.outcome, err)
			}
			if ended.State != tt.outcome || len(ended.ProgressSeries.Points) != len(moving.ProgressSeries.Points) {
				t.Fatalf("terminal result = %s with %d points, want %s and same-count replacement of %d points",
					ended.State, len(ended.ProgressSeries.Points), tt.outcome, len(moving.ProgressSeries.Points))
			}
			last := ended.ProgressSeries.Points[len(ended.ProgressSeries.Points)-1]
			if last.At != at(1.2).UnixMilli() || last.Completed == nil || *last.Completed != 1 || last.Rate == nil || *last.Rate <= 0 {
				t.Fatalf("final point = %+v; want the original measured movement retained at the replacement timestamp", last)
			}
			if positiveRatePointCount(ended.ProgressSeries) == 0 {
				t.Fatalf("Finish(%s) erased the only measured movement", tt.outcome)
			}
		})
	}
}

func TestCompactionKeepsMovementAcrossTerminalAndActivityBoundaries(t *testing.T) {
	t.Run("ordinary succeeded finish at the cap", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		clock := at(0)
		deps.Now = func() time.Time { return clock }
		job := seededExecution(t, deps, StateRunning, "claim-terminal-compaction")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-terminal-compaction"}
		for second := 0; second < MaxSeriesPoints-1; second++ {
			clock = at(float64(second))
			if _, err := svc.UpdateProgress(deps, ref, Progress{
				Completed: int64Ptr(0), Total: int64Ptr(1), Unit: "items",
			}); err != nil {
				t.Fatalf("unchanged count at %ds: %v", second, err)
			}
		}
		clock = at(float64(MaxSeriesPoints - 1))
		moving, err := svc.UpdateProgress(deps, ref, Progress{
			Completed: int64Ptr(1), Total: int64Ptr(1), Unit: "items",
		})
		if err != nil {
			t.Fatalf("report final count: %v", err)
		}
		if len(moving.ProgressSeries.Points) != MaxSeriesPoints || positiveRatePointCount(moving.ProgressSeries) == 0 {
			t.Fatalf("pre-finish series has %d points and %d positive rates; want the full cap and measured movement",
				len(moving.ProgressSeries.Points), positiveRatePointCount(moving.ProgressSeries))
		}
		clock = at(float64(MaxSeriesPoints))
		ended, err := svc.Finish(deps, FinishRequest{
			ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded,
		})
		if err != nil {
			t.Fatalf("Finish succeeded: %v", err)
		}
		if positiveRatePointCount(ended.ProgressSeries) == 0 {
			t.Fatal("final no-movement replacement and compaction erased the last measured count movement")
		}
		assertProgressSeriesAnchoredAndBounded(t, ended.ProgressSeries, at(0), at(float64(MaxSeriesPoints)).UnixMilli())
	})

	t.Run("activity-ending metadata at the cap", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		clock := at(0)
		deps.Now = func() time.Time { return clock }
		job := seededExecution(t, deps, StateRunning, "claim-activity-compaction")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-activity-compaction"}
		for second := 0; second < MaxSeriesPoints-1; second++ {
			clock = at(float64(second))
			if _, err := svc.UpdateProgress(deps, ref, Progress{
				Completed: int64Ptr(0), Total: int64Ptr(4), Unit: "items", Activity: second > 0,
			}); err != nil {
				t.Fatalf("segment heartbeat at %ds: %v", second, err)
			}
		}
		clock = at(float64(MaxSeriesPoints - 1))
		moving, err := svc.UpdateProgress(deps, ref, Progress{
			Completed: int64Ptr(1), Total: int64Ptr(4), Unit: "items", Activity: true,
		})
		if err != nil {
			t.Fatalf("report segment movement: %v", err)
		}
		if positiveRatePointCount(moving.ProgressSeries) == 0 {
			t.Fatal("setup did not record the actual segment movement")
		}
		clock = at(float64(MaxSeriesPoints))
		endedActivity, err := svc.UpdateProgress(deps, ref, Progress{
			Completed: int64Ptr(1), Total: int64Ptr(4), Unit: "items", Message: "assembling video",
		})
		if err != nil {
			t.Fatalf("end segment activity: %v", err)
		}
		if positiveRatePointCount(endedActivity.ProgressSeries) == 0 {
			t.Fatal("same-count phase report at the compaction boundary erased the last measured segment movement")
		}
		if endedActivity.ProgressSeries.Anchor == nil || endedActivity.ProgressSeries.Anchor.At != at(float64(MaxSeriesPoints-1)).UnixMilli() {
			t.Fatalf("metadata report rebased the count anchor: %+v", endedActivity.ProgressSeries.Anchor)
		}
		assertProgressSeriesAnchoredAndBounded(t, endedActivity.ProgressSeries, at(0), at(float64(MaxSeriesPoints)).UnixMilli())
	})
}

func TestProgressSeriesCompactionPreservesMovementAcrossCapacityBoundaries(t *testing.T) {
	for _, pointCount := range []int{MaxSeriesPoints - 1, MaxSeriesPoints, MaxSeriesPoints + 1, MaxSeriesPoints + 2, 241, 481} {
		t.Run(fmt.Sprintf("points_%d", pointCount), func(t *testing.T) {
			series := ProgressSeries{IntervalMs: seriesBaseIntervalMs, Unit: "items", Points: make([]SeriesPoint, pointCount)}
			for i := range series.Points {
				completed := 0.0
				if i > 0 {
					completed = 1
				}
				series.Points[i] = SeriesPoint{At: at(float64(i)).UnixMilli(), Completed: &completed}
				if i == 1 {
					rate := 1.0
					series.Points[i].Rate = &rate
				}
				if i > 1 {
					series.Points[i].RateNeutral = true
				}
			}
			latestAt := series.Points[len(series.Points)-1].At
			completed := int64(1)
			series, _ = advanceSeries(series, at(float64(pointCount)), Progress{
				Completed: &completed, Total: int64Ptr(2), Unit: "items",
			}, false)
			assertProgressSeriesAnchoredAndBounded(t, series, at(0), at(float64(pointCount)).UnixMilli())
			if latestAt >= series.Points[len(series.Points)-1].At {
				t.Fatalf("latest timestamp %d did not advance past %d", series.Points[len(series.Points)-1].At, latestAt)
			}
			if positiveRatePointCount(series) == 0 {
				t.Fatalf("compaction of %d seeded points erased the only actual movement", pointCount)
			}
		})
	}
}

func TestCompactionDoesNotCarryRateAcrossAStaleGap(t *testing.T) {
	one := 1.0
	movement := SeriesPoint{At: at(1).UnixMilli(), Completed: &one, Rate: f64(1)}
	stale := SeriesPoint{At: at(12).UnixMilli(), Completed: &one}
	merged := mergePoints(movement, stale)
	if merged.Rate != nil || merged.RateNeutral {
		t.Fatalf("compacted stale gap = %+v; a true gap must remain empty and non-neutral", merged)
	}
}

func TestNeutralEndpointMarkerPersistsAndLegacyNilRatesStayHardGaps(t *testing.T) {
	completed := 1.0
	series := ProgressSeries{Points: []SeriesPoint{{At: at(1).UnixMilli(), Completed: &completed, RateNeutral: true}}}
	cloned := cloneSeries(series)
	if !cloned.Points[0].RateNeutral {
		t.Fatal("cloneSeries dropped the internal neutral endpoint marker")
	}
	raw, err := json.Marshal(series)
	if err != nil {
		t.Fatalf("encode progress history: %v", err)
	}
	loaded := decodeSeries(types.JSON(raw))
	if !loaded.Points[0].RateNeutral {
		t.Fatalf("stored neutral endpoint loaded as %+v", loaded.Points[0])
	}

	// Older stored points have no marker. Their nil rate remains a hard gap, so
	// adding this metadata does not backfill uncertain historical intervals.
	legacy := decodeSeries(types.JSON([]byte(`{"points":[{"t":1746493445000,"c":1}]}`)))
	if len(legacy.Points) != 1 || legacy.Points[0].RateNeutral {
		t.Fatalf("legacy point became neutral after decoding: %+v", legacy.Points)
	}
	merged := mergePoints(SeriesPoint{At: at(0).UnixMilli(), Completed: &completed, Rate: f64(2)}, legacy.Points[0])
	if merged.Rate != nil || merged.RateNeutral {
		t.Fatalf("legacy nil rate merged across a hard gap: %+v", merged)
	}
}

func TestActivityEndingCountDecreaseStaysHardAcrossPlacementAndCompaction(t *testing.T) {
	newSeries := func() ProgressSeries {
		var series ProgressSeries
		series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
		series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(1), Unit: "items", Activity: true}, false)
		return series
	}
	assertHardDecrease := func(t *testing.T, series ProgressSeries) {
		t.Helper()
		last := series.Points[len(series.Points)-1]
		if last.Rate != nil || last.RateNeutral {
			t.Fatalf("decreasing Activity-ending point = %+v; want no rate and a hard boundary", last)
		}
		merged := mergePoints(series.Points[len(series.Points)-2], last)
		if merged.Rate != nil || merged.RateNeutral {
			t.Fatalf("compaction crossed the decreasing-count boundary: %+v", merged)
		}
	}

	t.Run("append decrease accepted by service", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		clock := at(0)
		deps.Now = func() time.Time { return clock }
		job := seededExecution(t, deps, StateRunning, "claim-decreasing-activity")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-decreasing-activity"}
		if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(0), Unit: "items"}); err != nil {
			t.Fatalf("initial count: %v", err)
		}
		clock = at(1)
		if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(1), Unit: "items", Activity: true}); err != nil {
			t.Fatalf("active count increase: %v", err)
		}
		clock = at(2)
		snap, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(0), Unit: "items"})
		if err != nil {
			t.Fatalf("service rejected the executor's decreasing restart count: %v", err)
		}
		assertHardDecrease(t, snap.ProgressSeries)
	})

	t.Run("final replacement falls from replaced point", func(t *testing.T) {
		series := newSeries()
		series, _ = advanceSeries(series, at(1.2), Progress{Completed: int64Ptr(0), Unit: "items"}, true)
		assertHardDecrease(t, series)
	})

	t.Run("final replacement cannot measure from the penultimate point across a decrease", func(t *testing.T) {
		series := newSeries()
		series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(3), Unit: "items", Activity: true}, false)
		// pointRate's required baseline is the penultimate 1-count point, so
		// 1->2 would produce a positive rate. The final count fell from the point
		// being replaced (3), which is a restart boundary and must override it.
		series, _ = advanceSeries(series, at(2.2), Progress{Completed: int64Ptr(2), Unit: "items"}, true)
		assertHardDecrease(t, series)
	})

	t.Run("cap compaction and final trim keep the gap hard", func(t *testing.T) {
		series := newSeries()
		series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
		gapAt := series.Points[len(series.Points)-1].At
		if last := series.Points[len(series.Points)-1]; last.Rate != nil || last.RateNeutral {
			t.Fatalf("setup count-decrease point = %+v; want a hard boundary", last)
		}
		for second := 3; second <= 400; second++ {
			series, _ = advanceSeries(series, at(float64(second)), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
		}
		series, _ = advanceSeries(series, at(401), Progress{Completed: int64Ptr(0), Unit: "items"}, true)
		assertProgressSeriesAnchoredAndBounded(t, series, at(0), at(401).UnixMilli())
		for _, point := range series.Points {
			if point.At >= gapAt && point.Rate != nil && *point.Rate > 0 {
				t.Fatalf("compacted/finalized count decrease inherited positive earlier rate: %+v", point)
			}
		}
	})
}

func TestNoMeasureCanBeNeutralButUnitChangeWithoutCountIsHard(t *testing.T) {
	t.Run("phase-only completion omission", func(t *testing.T) {
		var series ProgressSeries
		series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
		series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(1), Unit: "items", Activity: true}, false)
		series, _ = advanceSeries(series, at(2), Progress{Phase: "assembling", Message: "muxing"}, false)
		last := series.Points[len(series.Points)-1]
		if last.Completed != nil || last.Rate != nil || !last.RateNeutral {
			t.Fatalf("no-measure phase endpoint = %+v; want no count/rate and an explicit neutral marker", last)
		}
		merged := mergePoints(series.Points[len(series.Points)-2], last)
		if merged.Completed != nil || merged.Rate == nil || *merged.Rate <= 0 {
			t.Fatalf("neutral no-measure endpoint failed to retain the prior graph sample without inventing a count: %+v", merged)
		}
	})

	t.Run("unit change without a count", func(t *testing.T) {
		var series ProgressSeries
		series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
		series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(1), Unit: "items", Activity: true}, false)
		series, _ = advanceSeries(series, at(2), Progress{Unit: "bytes"}, false)
		last := series.Points[len(series.Points)-1]
		if last.Completed != nil || last.Rate != nil || last.RateNeutral {
			t.Fatalf("unit-change endpoint = %+v; want no count/rate and a hard boundary", last)
		}
	})
}

func positiveRatePointCount(series ProgressSeries) int {
	count := 0
	for _, point := range series.Points {
		if point.Rate != nil && *point.Rate > 0 {
			count++
		}
	}
	return count
}

func assertProgressSeriesAnchoredAndBounded(t *testing.T, series ProgressSeries, first time.Time, latestAt int64) {
	t.Helper()
	if len(series.Points) == 0 || len(series.Points) > MaxSeriesPoints {
		t.Fatalf("series has %d points; want 1..%d", len(series.Points), MaxSeriesPoints)
	}
	if series.Points[0].At != first.UnixMilli() {
		t.Fatalf("compaction moved the first point to %d; want first Job timestamp %d", series.Points[0].At, first.UnixMilli())
	}
	if last := series.Points[len(series.Points)-1].At; last != latestAt {
		t.Fatalf("compaction ended at %d; want latest timestamp %d", last, latestAt)
	}
}

func TestHLSActivityClearsAtPauseRestartAndAssemblyBoundaries(t *testing.T) {
	segmentProgress := func(completed int64, activity bool) Progress {
		return Progress{Completed: int64Ptr(completed), Total: int64Ptr(8), Unit: "items", Activity: activity}
	}
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), segmentProgress(0, false), false)
	series, _ = advanceSeries(series, at(1), segmentProgress(1, true), false)
	series, _ = advanceSeries(series, at(2), segmentProgress(1, true), false)
	if got := series.CurrentRate(at(2)); got == nil || *got != 1 {
		t.Fatalf("same-count byte activity lost the measured rate: %v", got)
	}

	// A paused snapshot ends the activity lease. StatePaused also suppresses a
	// live rate even while the last measured count anchor is young.
	series, _ = advanceSeries(series, at(3), segmentProgress(1, false), false)
	pausedPoint := series.Points[len(series.Points)-1]
	if pausedPoint.Rate != nil || !pausedPoint.RateNeutral {
		t.Fatalf("same-count pause endpoint = %+v; want a neutral endpoint without a sample", pausedPoint)
	}
	if series.ActivityAt != nil {
		t.Fatalf("pause kept HLS activity live at %d", *series.ActivityAt)
	}
	paused := Snapshot{State: StatePaused, ProgressSeries: series}
	if got := paused.LiveRate(at(3)); got != nil {
		t.Fatalf("paused snapshot exposed a live rate: %v", *got)
	}

	// Resume restarts the segment count. Its decrease resets the old rate, and
	// assembly leaves no segment-byte activity timestamp behind.
	series, _ = advanceSeries(series, at(4), segmentProgress(0, false), false)
	restartPoint := series.Points[len(series.Points)-1]
	if restartPoint.Rate != nil || restartPoint.RateNeutral {
		t.Fatalf("decreasing restart endpoint = %+v; want a hard restart boundary", restartPoint)
	}
	if got := series.CurrentRate(at(4)); got != nil {
		t.Fatalf("restarted segment count kept its earlier rate: %v", *got)
	}
	series, _ = advanceSeries(series, at(5), segmentProgress(1, false), false)
	if series.ActivityAt != nil {
		t.Fatalf("assembly kept an HLS byte activity timestamp: %d", *series.ActivityAt)
	}

	// A phase-only assembly report with an unchanged count must not refresh an
	// old segment anchor or manufacture a zero-speed graph sample.
	var phaseOnly ProgressSeries
	phaseOnly, _ = advanceSeries(phaseOnly, at(0), segmentProgress(0, false), false)
	phaseOnly, _ = advanceSeries(phaseOnly, at(1), segmentProgress(1, true), false)
	for second := 2; second <= 11; second++ {
		phaseOnly, _ = advanceSeries(phaseOnly, at(float64(second)), segmentProgress(1, true), false)
	}
	phaseOnly, _ = advanceSeries(phaseOnly, at(12), Progress{
		Phase: "assembling video", Completed: int64Ptr(1), Total: int64Ptr(8), Unit: "items",
	}, false)
	if phaseOnly.ActivityAt != nil || phaseOnly.Anchor == nil || phaseOnly.Anchor.At != at(1).UnixMilli() {
		t.Fatalf("phase-only report changed activity or count anchor: %+v", phaseOnly)
	}
	if got := phaseOnly.CurrentRate(at(12)); got != nil {
		t.Fatalf("phase-only report refreshed stale segment rate: %v", *got)
	}
	last := phaseOnly.Points[len(phaseOnly.Points)-1]
	if last.Rate != nil || last.Completed == nil || *last.Completed != 1 {
		t.Fatalf("phase-only assembly point = %+v; want unchanged count without a sample", last)
	}
}

func TestAdvanceSeriesRecordsNoRateWhenCompletedGoesBackwards(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(5000), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(100), false)
	if last := series.Points[1]; last.Rate != nil {
		t.Fatalf("a restarted transfer recorded a negative rate: %v", *last.Rate)
	}
	if got := series.CurrentRate(at(1)); got != nil {
		t.Fatalf("a restarted transfer kept its old rate: %v", *got)
	}
}

func TestAdvanceSeriesDropsRatesWhenTheUnitChanges(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(3), Unit: "items"}, false)
	if series.Unit != "items" {
		t.Fatalf("series unit = %q; want items", series.Unit)
	}
	for i, point := range series.Points {
		if point.Rate != nil {
			t.Fatalf("point %d kept a %v rate measured in another unit", i, *point.Rate)
		}
	}
	if last := series.Points[len(series.Points)-1]; last.RateNeutral {
		t.Fatalf("unit-change boundary was marked neutral: %+v", last)
	}
	if got := series.CurrentRate(at(2)); got != nil {
		t.Fatalf("current rate crossed a unit change: %v", *got)
	}
}

func TestFinalTrimAndCompactionKeepStaleAndRestartBoundaries(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
	series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(1), Unit: "items", Activity: true}, false)
	series, _ = advanceSeries(series, at(2), Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	if last := series.Points[len(series.Points)-1]; last.Rate != nil || !last.RateNeutral {
		t.Fatalf("activity-ending pause endpoint = %+v; want neutral without a count sample", last)
	}

	// No progress arrives during the real stale interval. The next same-count
	// snapshot is a hard boundary, even though terminal trimming later sees only
	// unchanged counts at the end of the series.
	staleAt := at(13)
	series, _ = advanceSeries(series, staleAt, Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	staleIndex := len(series.Points) - 1
	if stalePoint := series.Points[staleIndex]; stalePoint.Rate != nil || stalePoint.RateNeutral {
		t.Fatalf("stale pause endpoint = %+v; want a hard gap", stalePoint)
	}
	for second := 14; second < 20; second++ {
		series, _ = advanceSeries(series, at(float64(second)), Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	}
	series, _ = advanceSeries(series, at(20), Progress{Completed: int64Ptr(0), Unit: "items"}, false)
	if restartPoint := series.Points[len(series.Points)-1]; restartPoint.Rate != nil || restartPoint.RateNeutral {
		t.Fatalf("count-decrease restart = %+v; want a hard restart boundary", restartPoint)
	}
	series, _ = advanceSeries(series, at(21), Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	for second := 22; second <= MaxSeriesPoints+4; second++ {
		series, _ = advanceSeries(series, at(float64(second)), Progress{Completed: int64Ptr(1), Unit: "items"}, false)
	}
	series, _ = advanceSeries(series, at(float64(MaxSeriesPoints+5)), Progress{Completed: int64Ptr(1), Unit: "items"}, true)
	if len(series.Points) > MaxSeriesPoints {
		t.Fatalf("terminal series has %d points, over %d", len(series.Points), MaxSeriesPoints)
	}
	if positiveRatePointCount(series) == 0 {
		t.Fatal("compaction erased the post-restart count measurement")
	}
	if series.Points[0].At != at(0).UnixMilli() {
		t.Fatalf("compaction moved first Job timestamp to %d", series.Points[0].At)
	}
	for _, point := range series.Points {
		if point.At >= staleAt.UnixMilli() && point.At < at(21).UnixMilli() && point.Rate != nil && *point.Rate > 0 {
			t.Fatalf("stale/restart interval inherited a positive rate: %+v", point)
		}
	}
}

func TestServicePauseResumeKeepsRestartGapHardThroughCompaction(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	registerTestAdapter(t, svc, testDefinition())
	accepted := acceptQueued(t, svc, deps, nil)
	first, ok := claimOnce(t, svc, deps, "pause-resume-runtime-a")
	if !ok {
		t.Fatal("the accepted Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: first.ExecutionToken}
	zero, one := int64(0), int64(1)
	if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: &zero, Unit: "items"}); err != nil {
		t.Fatalf("initial count: %v", err)
	}
	clock = at(1)
	moving, err := svc.UpdateProgress(deps, ref, Progress{Completed: &one, Unit: "items", Activity: true})
	if err != nil {
		t.Fatalf("first attempt count movement: %v", err)
	}
	if positiveRatePointCount(moving.ProgressSeries) == 0 {
		t.Fatal("setup did not record the first attempt's measured rate")
	}
	clock = at(2)
	paused, err := svc.Transition(deps, Transition{
		JobID: accepted.ID, ExpectedVersion: moving.Version, ExecutionToken: first.ExecutionToken, To: StatePaused,
	})
	if err != nil {
		t.Fatalf("running -> paused: %v", err)
	}
	if paused.State != StatePaused || paused.LiveRate(clock) != nil {
		t.Fatalf("paused Job = %s with live rate %v", paused.State, paused.LiveRate(clock))
	}

	clock = at(20)
	queued, err := svc.Transition(deps, Transition{JobID: accepted.ID, ExpectedVersion: paused.Version, To: StateQueued})
	if err != nil {
		t.Fatalf("paused -> queued: %v", err)
	}
	if queued.State != StateQueued {
		t.Fatalf("resume queued the Job as %s", queued.State)
	}
	resumedExecution, ok := claimOnce(t, svc, deps, "pause-resume-runtime-b")
	if !ok {
		t.Fatal("the resumed Job was not claimed")
	}
	resumedRef := ExecutionRef{JobID: accepted.ID, ExecutionToken: resumedExecution.ExecutionToken}
	clock = at(20.5)
	reset, err := svc.UpdateProgress(deps, resumedRef, Progress{Completed: &zero, Unit: "items"})
	if err != nil {
		t.Fatalf("resumed executor's reset count: %v", err)
	}
	if reset.ProgressSeries.Points[len(reset.ProgressSeries.Points)-1].RateNeutral {
		t.Fatalf("the real pause/restart count reset was marked neutral: %+v", reset.ProgressSeries.Points[len(reset.ProgressSeries.Points)-1])
	}
	if reset.LiveRate(clock) != nil {
		t.Fatalf("resumed executor reused the pre-pause rate: %v", reset.LiveRate(clock))
	}
	clock = at(21.5)
	resumedMovement, err := svc.UpdateProgress(deps, resumedRef, Progress{Completed: &one, Unit: "items"})
	if err != nil {
		t.Fatalf("resumed count movement: %v", err)
	}
	if rate := resumedMovement.LiveRate(clock); rate == nil || *rate <= 0 {
		t.Fatalf("fresh resumed count movement has live rate %v", rate)
	}

	// Feed the actual service-produced lifecycle history through repeated
	// sampling and compaction, then trim its unchanged terminal tail. The
	// pre-pause sample must stop at the restart gap; only the resumed movement
	// may remain on the far side of that boundary.
	series := resumedMovement.ProgressSeries
	for second := 23; second <= 500; second++ {
		series, _ = advanceSeries(series, at(float64(second)), Progress{Completed: &one, Unit: "items"}, false)
	}
	series, _ = advanceSeries(series, at(501), Progress{Completed: &one, Unit: "items"}, true)
	assertProgressSeriesAnchoredAndBounded(t, series, at(0), at(501).UnixMilli())
	if positiveRatePointCount(series) == 0 {
		t.Fatal("pause/resume compaction erased the resumed count movement")
	}
	for _, point := range series.Points {
		if point.At >= at(20).UnixMilli() && point.At < at(21.5).UnixMilli() && point.Rate != nil && *point.Rate > 0 {
			t.Fatalf("pre-pause movement crossed the actual pause/restart gap: %+v", point)
		}
	}
}

func TestAdvanceSeriesRecordsOnlyGraphedMetrics(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Metrics: []Metric{
		{Key: "rows", Label: "Rows", Value: 12, Graph: true},
		{Key: "errors", Label: "Errors", Value: 1},
	}}, false)
	point := series.Points[0]
	if len(point.Values) != 1 || point.Values["rows"] != 12 {
		t.Fatalf("point values = %v; want only the graphed rows metric", point.Values)
	}
	if point.Completed != nil || point.Rate != nil {
		t.Fatalf("a Job without Completed recorded a count or a rate: %+v", point)
	}
}

func TestAdvanceSeriesCompactsTheWholeLifetimeIntoItsBound(t *testing.T) {
	var series ProgressSeries
	for second := 0; second <= 1000; second++ {
		series, _ = advanceSeries(series, at(float64(second)), bytesProgress(int64(second)*100), false)
		if len(series.Points) > MaxSeriesPoints {
			t.Fatalf("second %d: %d points, over the %d bound", second, len(series.Points), MaxSeriesPoints)
		}
	}
	if first := series.Points[0].At; first != seriesEpoch.UnixMilli() {
		t.Fatalf("compaction moved the Job's first point to +%dms; the graph must start where the Job did", first-seriesEpoch.UnixMilli())
	}
	// Between points the latest tick lives in the snapshot, not the series, so
	// the newest point is at most one interval old.
	if last := series.Points[len(series.Points)-1].At; at(1000).UnixMilli()-last >= series.IntervalMs {
		t.Fatalf("compaction lost the latest point: last at +%dms with a %dms interval", last-seriesEpoch.UnixMilli(), series.IntervalMs)
	}
	if series.IntervalMs <= 1000 {
		t.Fatalf("interval = %dms; want it doubled by compaction", series.IntervalMs)
	}
	for i, point := range series.Points {
		if i > 0 && point.At <= series.Points[i-1].At {
			t.Fatalf("points out of order at %d", i)
		}
		if point.Rate != nil && math.Abs(*point.Rate-100) > 1e-6 {
			t.Fatalf("point %d rate = %v after compaction; want 100", i, *point.Rate)
		}
	}
}

func TestAdvanceSeriesFinalAlwaysRecordsTheLastValue(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), bytesProgress(0), false)
	series, _ = advanceSeries(series, at(1), bytesProgress(1000), false)
	series, changed := advanceSeries(series, at(1.2), bytesProgress(1500), true)
	if !changed {
		t.Fatal("a final sample reported no change")
	}
	last := series.Points[len(series.Points)-1]
	if last.Completed == nil || *last.Completed != 1500 {
		t.Fatalf("final point = %+v; want completed 1500", last)
	}
	if series.Anchor != nil || series.Rate != nil {
		t.Fatalf("a finished series still reports a live rate: %+v", series)
	}
	if avg := series.AverageRate(); avg == nil || math.Abs(*avg-1250) > 1e-9 {
		t.Fatalf("average rate = %v; want 1250", avg)
	}
}

func TestEstimateETA(t *testing.T) {
	progress := Progress{Completed: int64Ptr(250), Total: int64Ptr(1250), Unit: "bytes"}
	eta := EstimateETA(progress, f64(100), at(0))
	if eta == nil || !eta.Equal(at(10)) {
		t.Fatalf("eta = %v; want +10s", eta)
	}
	if EstimateETA(progress, nil, at(0)) != nil || EstimateETA(progress, f64(0), at(0)) != nil {
		t.Fatal("an ETA was estimated without a positive rate")
	}
	if EstimateETA(Progress{Completed: int64Ptr(250), Unit: "bytes"}, f64(100), at(0)) != nil {
		t.Fatal("an ETA was estimated without a total")
	}
}

func TestValidateProgressMetrics(t *testing.T) {
	valid := Metric{Key: "rows", Label: "Rows", Value: 1}
	many := func(n int, graph bool) []Metric {
		out := make([]Metric, n)
		for i := range out {
			out[i] = Metric{Key: "m" + strings.Repeat("x", i), Label: "M", Value: 1, Graph: graph}
		}
		return out
	}
	tests := []struct {
		name    string
		metrics []Metric
		wantErr bool
	}{
		{"one valid metric", []Metric{valid}, false},
		{"the bound", many(MaxProgressMetrics, false), false},
		{"too many", many(MaxProgressMetrics+1, false), true},
		{"graph bound", many(MaxGraphedMetrics, true), false},
		{"too many graphed", many(MaxGraphedMetrics+1, true), true},
		{"empty key", []Metric{{Label: "x"}}, true},
		{"key charset", []Metric{{Key: "Rows!", Label: "x"}}, true},
		{"key too long", []Metric{{Key: strings.Repeat("k", MaxMetricKeyBytes+1), Label: "x"}}, true},
		{"duplicate key", []Metric{valid, valid}, true},
		{"label too long", []Metric{{Key: "k", Label: strings.Repeat("l", MaxMetricLabelBytes+1)}}, true},
		{"unit too long", []Metric{{Key: "k", Label: "x", Unit: strings.Repeat("u", MaxProgressUnitBytes+1)}}, true},
		{"negative value", []Metric{{Key: "k", Label: "x", Value: -1}}, true},
		{"NaN value", []Metric{{Key: "k", Label: "x", Value: math.NaN()}}, true},
		{"infinite total", []Metric{{Key: "k", Label: "x", Total: f64(math.Inf(1))}}, true},
		{"negative total", []Metric{{Key: "k", Label: "x", Total: f64(-1)}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProgress(Progress{Metrics: tt.metrics})
			if tt.wantErr != (err != nil) {
				t.Fatalf("validateProgress error = %v; wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidProgress) {
				t.Fatalf("error %v is not ErrInvalidProgress", err)
			}
		})
	}
}

func TestUpdateProgressStoresMetricsAndHistory(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}

	tick := func(seconds float64, completed int64) Snapshot {
		t.Helper()
		clock = at(seconds)
		snap, err := svc.UpdateProgress(deps, ref, Progress{
			Completed: int64Ptr(completed), Total: int64Ptr(10000), Unit: "bytes",
			Metrics: []Metric{
				{Key: "segments", Label: "Segments", Value: float64(completed / 1000), Total: f64(10), Unit: "items", Graph: true},
			},
		})
		if err != nil {
			t.Fatalf("UpdateProgress at +%vs: %v", seconds, err)
		}
		return snap
	}
	tick(0, 0)
	tick(1, 1000)
	snap := tick(2, 3000)

	if got := snap.Progress.Metrics; len(got) != 1 || got[0].Key != "segments" || got[0].Value != 3 || got[0].Total == nil || *got[0].Total != 10 {
		t.Fatalf("snapshot metrics = %+v; want segments 3 of 10", got)
	}
	if n := len(snap.ProgressSeries.Points); n != 3 {
		t.Fatalf("snapshot series has %d points; want 3", n)
	}
	if snap.ProgressUpdatedAt == nil || !snap.ProgressUpdatedAt.Equal(at(2)) {
		t.Fatalf("progress updated at = %v; want +2s", snap.ProgressUpdatedAt)
	}

	stored, err := svc.Get(deps, Access{Administrator: true}, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := stored.Progress.Metrics; len(got) != 1 || got[0].Value != 3 || !got[0].Graph || got[0].Unit != "items" {
		t.Fatalf("stored metrics = %+v", got)
	}
	series := stored.ProgressSeries
	if len(series.Points) != 3 || series.Points[2].Values["segments"] != 3 {
		t.Fatalf("stored series = %+v; want three points with the graphed metric", series)
	}
	if rate := series.CurrentRate(at(2)); rate == nil || *rate != 1500 {
		t.Fatalf("stored current rate = %v; want the smoothed 1500", rate)
	}

	// A tick that clears the metrics clears the column: each snapshot replaces
	// the whole set.
	clock = at(2.5)
	if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(3500), Unit: "bytes"}); err != nil {
		t.Fatalf("clearing tick: %v", err)
	}
	if row := jobRow(t, deps, job.ID); len(row.ProgressMetrics) != 0 {
		t.Fatalf("metrics column = %s after a tick that reported none", row.ProgressMetrics)
	}
}

func TestFinishFinalProgressLandsInTheHistory(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(0), Total: int64Ptr(100), Unit: "items"}); err != nil {
		t.Fatalf("seed progress: %v", err)
	}
	clock = at(4)
	final := Progress{Completed: int64Ptr(100), Total: int64Ptr(100), Unit: "items"}
	finished, err := svc.Finish(deps, FinishRequest{
		ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded, FinalProgress: &final,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	points := finished.ProgressSeries.Points
	if len(points) != 2 || points[1].Completed == nil || *points[1].Completed != 100 {
		t.Fatalf("finished series = %+v; want the final count as its last point", points)
	}
	if avg := finished.ProgressSeries.AverageRate(); avg == nil || *avg != 25 {
		t.Fatalf("average rate = %v; want 25 items/s", avg)
	}
	if finished.ProgressSeries.CurrentRate(at(4)) != nil {
		t.Fatal("a finished Job still reports a current rate")
	}
}

func TestLiveProgressReturnsOnlyVisibleChangesAfterTheWatermark(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }

	owned := seededExecution(t, deps, StateRunning, "claim-mine")
	theirs := seededExecution(t, deps, StateRunning, "claim-theirs")
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", owned.ID).Update("owner_user_id", 7).Error; err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", theirs.ID).Update("owner_user_id", 8).Error; err != nil {
		t.Fatal(err)
	}
	report := func(job models.Job, token string, seconds float64, completed int64) {
		t.Helper()
		clock = at(seconds)
		if _, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: token},
			Progress{Completed: int64Ptr(completed), Unit: "bytes"}); err != nil {
			t.Fatalf("UpdateProgress: %v", err)
		}
	}
	report(owned, "claim-mine", 1, 100)
	report(theirs, "claim-theirs", 2, 100)

	ids := func(snaps []Snapshot) []string {
		out := make([]string, 0, len(snaps))
		for _, snap := range snaps {
			out = append(out, snap.ID)
		}
		return out
	}

	mine, err := svc.LiveProgress(deps, Access{UserID: 7}, EventFilter{}, at(0), 0)
	if err != nil {
		t.Fatalf("LiveProgress: %v", err)
	}
	if got := ids(mine); len(got) != 1 || got[0] != owned.ID {
		t.Fatalf("owner's live progress = %v; want only their own Job %s", got, owned.ID)
	}
	if other, _ := svc.LiveProgress(deps, Access{UserID: 9}, EventFilter{}, at(0), 0); len(other) != 0 {
		t.Fatalf("a user who owns neither Job saw live progress for %v", ids(other))
	}
	all, err := svc.LiveProgress(deps, Access{UserID: 1, Administrator: true}, EventFilter{}, at(0), 0)
	if err != nil {
		t.Fatalf("admin LiveProgress: %v", err)
	}
	if got := ids(all); len(got) != 2 || got[0] != theirs.ID || got[1] != owned.ID {
		t.Fatalf("admin live progress = %v; want both, newest change first", got)
	}
	// An administrator's stream limited to their own work gets its own frames.
	own, err := svc.LiveProgress(deps, Access{UserID: 7, Administrator: true}, EventFilter{OwnedByViewer: true}, at(0), 0)
	if err != nil {
		t.Fatalf("owner-filtered LiveProgress: %v", err)
	}
	if got := ids(own); len(got) != 1 || got[0] != owned.ID {
		t.Fatalf("owner-filtered live progress = %v; want only %s", got, owned.ID)
	}

	// The watermark is exclusive: a reader that saw the change at +1s is not
	// sent it again, and sees the next one.
	after, err := svc.LiveProgress(deps, Access{UserID: 7}, EventFilter{}, at(1), 0)
	if err != nil || len(after) != 0 {
		t.Fatalf("after the watermark: %v, %v; want nothing", ids(after), err)
	}
	report(owned, "claim-mine", 3, 200)
	after, err = svc.LiveProgress(deps, Access{UserID: 7}, EventFilter{}, at(1), 0)
	if err != nil || len(after) != 1 || after[0].Progress.Completed == nil || *after[0].Progress.Completed != 200 {
		t.Fatalf("next change = %+v, %v; want the owned Job at 200", after, err)
	}
}

func TestAdvanceSeriesStaysBoundedWhenTheGraphedKeysKeepChanging(t *testing.T) {
	var series ProgressSeries
	for second := 0; second <= 5000; second++ {
		key := fmt.Sprintf("m%d", second%1000)
		series, _ = advanceSeries(series, at(float64(second)), Progress{Metrics: []Metric{
			{Key: key, Label: key, Value: 1, Graph: true},
		}}, false)
	}
	for i, point := range series.Points {
		if len(point.Values) > MaxGraphedMetrics {
			t.Fatalf("point %d holds %d graphed values; a merge must not accumulate keys", i, len(point.Values))
		}
	}
}

func TestLiveProgressKeepsTheNewestChangesWhenOverItsLimit(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(5)
	deps.Now = func() time.Time { return clock }
	older := seededExecution(t, deps, StateRunning, "claim-1")
	newer := seededExecution(t, deps, StateRunning, "claim-2")
	for i, job := range []models.Job{older, newer} {
		clock = at(float64(5 + i))
		if _, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: job.ExecutionToken},
			Progress{Completed: int64Ptr(1), Unit: "items"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := svc.LiveProgress(deps, Access{UserID: 1, Administrator: true}, EventFilter{}, at(0), 1)
	if err != nil || len(page) != 1 || page[0].ID != newer.ID {
		t.Fatalf("limited read = %+v, %v; want the most recent change", page, err)
	}
}

func TestCompactionKeepsAPauseGap(t *testing.T) {
	rate := 100.0
	c := func(v float64) *float64 { return &v }
	merged := mergePoints(SeriesPoint{At: 1000, Completed: c(100), Rate: &rate}, SeriesPoint{At: 601000, Completed: c(200)})
	if merged.Rate != nil {
		t.Fatalf("merged rate = %v; a pause ending at the later point must stay a gap", *merged.Rate)
	}
}

func TestFinishWithoutFinalProgressClosesTheSeries(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := at(0)
	deps.Now = func() time.Time { return clock }
	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	for _, tick := range []struct {
		at        float64
		completed int64
	}{{0, 0}, {1, 100}, {1.4, 180}} {
		clock = at(tick.at)
		if _, err := svc.UpdateProgress(deps, ref, Progress{Completed: int64Ptr(tick.completed), Unit: "bytes"}); err != nil {
			t.Fatal(err)
		}
	}
	clock = at(1.5)
	finished, err := svc.Finish(deps, FinishRequest{ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	points := finished.ProgressSeries.Points
	if last := points[len(points)-1]; last.Completed == nil || *last.Completed != 180 {
		t.Fatalf("last point = %+v; the tick inside the interval must close the series", last)
	}
	if finished.ProgressSeries.Anchor != nil {
		t.Fatal("a finished Job still holds a live rate anchor")
	}
}

func TestAGraphedKeyReusedInAnotherUnitStartsAFreshHistory(t *testing.T) {
	var series ProgressSeries
	metric := func(unit string, value float64) Progress {
		return Progress{Metrics: []Metric{{Key: "size", Label: "Size", Value: value, Unit: unit, Graph: true}}}
	}
	series, _ = advanceSeries(series, at(0), metric("bytes", 1024), false)
	series, _ = advanceSeries(series, at(1), metric("bytes", 2048), false)
	series, _ = advanceSeries(series, at(2), metric("items", 3), false)
	held := 0
	for _, point := range series.Points {
		if _, ok := point.Values["size"]; ok {
			held++
		}
	}
	if held != 1 || series.Units["size"] != "items" {
		t.Fatalf("points holding size = %d, unit %q; want only the items sample", held, series.Units["size"])
	}
}

func TestATickWithNoMeasureKeepsTheHistory(t *testing.T) {
	var series ProgressSeries
	series, _ = advanceSeries(series, at(0), Progress{Completed: int64Ptr(0), Total: int64Ptr(10), Unit: "items"}, false)
	series, _ = advanceSeries(series, at(1), Progress{Completed: int64Ptr(10), Total: int64Ptr(10), Unit: "items"}, false)
	series, _ = advanceSeries(series, at(2), Progress{Phase: "muxing"}, false)
	if series.Unit != "items" || series.Points[1].Completed == nil || series.Points[1].Rate == nil {
		t.Fatalf("series = %+v; a tick with no measure must not erase the items history", series)
	}
	if avg := series.AverageRate(); avg == nil || *avg != 10 {
		t.Fatalf("average rate = %v; want 10 items/s", avg)
	}
}
