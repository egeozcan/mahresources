package jobs

import (
	"errors"
	"fmt"
	"mahresources/models"
	"math"
	"strings"
	"testing"
	"time"
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
	if got := series.CurrentRate(at(2)); got != nil {
		t.Fatalf("current rate crossed a unit change: %v", *got)
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

	mine, err := svc.LiveProgress(deps, Access{UserID: 7}, at(0), 0)
	if err != nil {
		t.Fatalf("LiveProgress: %v", err)
	}
	if got := ids(mine); len(got) != 1 || got[0] != owned.ID {
		t.Fatalf("owner's live progress = %v; want only their own Job %s", got, owned.ID)
	}
	if other, _ := svc.LiveProgress(deps, Access{UserID: 9}, at(0), 0); len(other) != 0 {
		t.Fatalf("a user who owns neither Job saw live progress for %v", ids(other))
	}
	all, err := svc.LiveProgress(deps, Access{UserID: 1, Administrator: true}, at(0), 0)
	if err != nil {
		t.Fatalf("admin LiveProgress: %v", err)
	}
	if got := ids(all); len(got) != 2 || got[0] != theirs.ID || got[1] != owned.ID {
		t.Fatalf("admin live progress = %v; want both, newest change first", got)
	}

	// The watermark is exclusive: a reader that saw the change at +1s is not
	// sent it again, and sees the next one.
	after, err := svc.LiveProgress(deps, Access{UserID: 7}, at(1), 0)
	if err != nil || len(after) != 0 {
		t.Fatalf("after the watermark: %v, %v; want nothing", ids(after), err)
	}
	report(owned, "claim-mine", 3, 200)
	after, err = svc.LiveProgress(deps, Access{UserID: 7}, at(1), 0)
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
	page, err := svc.LiveProgress(deps, Access{UserID: 1, Administrator: true}, at(0), 1)
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
