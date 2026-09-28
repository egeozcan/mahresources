package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"mahresources/models/jobmetrics"
	"mahresources/models/types"
)

// Metric ceilings, re-exported from jobmetrics so the Service's own callers
// need not import it.
const (
	MaxProgressMetrics  = jobmetrics.MaxMetrics
	MaxGraphedMetrics   = jobmetrics.MaxGraphed
	MaxMetricKeyBytes   = jobmetrics.MaxKeyBytes
	MaxMetricLabelBytes = jobmetrics.MaxLabelBytes

	// MaxSeriesPoints bounds a Job's stored history. When a series would grow
	// past it, adjacent points are merged and the interval doubles, so the
	// whole lifetime of a Job of any length fits in the same bound.
	MaxSeriesPoints = 120

	seriesBaseIntervalMs = int64(1000)
	rateAnchorIntervalMs = int64(1000)
	// ProgressRateFreshFor is how long a current rate stays true without new
	// evidence. Producers that coalesce activity callbacks must preserve the
	// source time so a delayed mirror cannot restart this window.
	ProgressRateFreshFor = 10 * time.Second
	rateStaleAfter       = ProgressRateFreshFor
)

// Metric is one named figure reported beside a Job's primary measure. See
// jobmetrics.Metric.
type Metric = jobmetrics.Metric

// SeriesPoint is one sample of a Job's progress history. The JSON names are
// short because a full series is stored on the Job row and sent to the Jobs
// drawer for every row it shows.
//
// At is Unix milliseconds. Rate is the sampled progress per second. Ordinary
// progress uses comparable neighboring counts; an executor with a coarse
// count may carry its last measured rate across fresh activity reports that do
// not change that count. RateNeutral is internal stored-history metadata that
// distinguishes such no-new-sample endpoints from actual measurement gaps.
// Values holds the graphed metrics.
type SeriesPoint struct {
	At          int64              `json:"t"`
	Completed   *float64           `json:"c,omitempty"`
	Rate        *float64           `json:"r,omitempty"`
	RateNeutral bool               `json:"rateNeutral,omitempty"`
	Values      map[string]float64 `json:"v,omitempty"`
}

// RateAnchor is the count and instant the current rate is measured from.
type RateAnchor struct {
	At        int64   `json:"t"`
	Completed float64 `json:"c"`
}

// ProgressSeries is the bounded history of one Job's progress, kept on the Job
// row so a graph survives a reload, another process and the Job finishing.
//
// It carries two different rates. Each point's Rate is the sampled speed for
// that part of the graph. For ordinary progress it is measured between
// comparable counts; a coarse reporter can carry its last measured count rate
// while fresh activity proves work is still arriving. Rate on the series is
// the current speed, measured once a second and smoothed, which is what
// "2.1 MB/s" beside a running transfer wants: after compaction a point can span
// minutes, and a speed that old would be wrong.
type ProgressSeries struct {
	IntervalMs int64         `json:"intervalMs"`
	Unit       string        `json:"unit,omitempty"`
	Points     []SeriesPoint `json:"points"`
	Rate       *float64      `json:"rate,omitempty"`
	Anchor     *RateAnchor   `json:"anchor,omitempty"`
	// ActivityAt is the last time an executor proved fresh work while its
	// primary count stayed coarse. It keeps an existing count-based rate fresh
	// without letting byte heartbeats become count samples.
	ActivityAt *int64 `json:"activityAt,omitempty"`
	// Units is each graphed metric's unit, so a key reused in another unit
	// starts a fresh history instead of joining numbers that do not compare.
	Units map[string]string `json:"units,omitempty"`
}

// CurrentRate is the Job's current speed in units of Completed per second, or
// nil when there is none or nothing has refreshed it recently.
func (s ProgressSeries) CurrentRate(now time.Time) *float64 {
	if s.Rate == nil || s.Anchor == nil {
		return nil
	}
	lastActivity := s.Anchor.At
	if s.ActivityAt != nil {
		lastActivity = *s.ActivityAt
	}
	age := now.UnixMilli() - lastActivity
	if age < 0 || age > rateStaleAfter.Milliseconds() {
		return nil
	}
	rate := *s.Rate
	return &rate
}

// AverageRate is Completed's change per second across the whole series. It is
// nil when the series has no two comparable counts, or when nothing was counted
// between them: an average of zero says less than the amount beside it does.
// Snapshot.AverageRate, which measures over the Job's running time, is what a
// finished Job reports; this is its measure for a Job with no running time
// banked.
func (s ProgressSeries) AverageRate() *float64 {
	first, last := s.countedEnds()
	if first == nil || last == first || last.At <= first.At || *last.Completed <= *first.Completed {
		return nil
	}
	rate := (*last.Completed - *first.Completed) / (float64(last.At-first.At) / 1000)
	return &rate
}

// countedEnds is the first and the last point that hold a count.
func (s ProgressSeries) countedEnds() (first, last *SeriesPoint) {
	for i := range s.Points {
		if s.Points[i].Completed == nil {
			continue
		}
		if first == nil {
			first = &s.Points[i]
		}
		last = &s.Points[i]
	}
	return first, last
}

// EstimateETA estimates when a Job whose total is known finishes at the given
// rate. It answers nil rather than guessing when either is missing.
func EstimateETA(progress Progress, rate *float64, now time.Time) *time.Time {
	if rate == nil || *rate <= 0 || progress.Completed == nil || progress.Total == nil {
		return nil
	}
	remaining := float64(*progress.Total - *progress.Completed)
	if remaining < 0 {
		return nil
	}
	eta := now.Add(time.Duration(remaining / *rate * float64(time.Second))).UTC()
	return &eta
}

// advanceSeries folds one progress snapshot into a series and reports whether
// the series changed, so a tick that added nothing writes nothing.
//
// A point is appended at most once per interval. A final snapshot always lands:
// it replaces the latest point when that one is younger than the interval, and
// it ends the current rate, because a finished Job has no speed.
func advanceSeries(series ProgressSeries, now time.Time, progress Progress, final bool) (ProgressSeries, bool) {
	series = cloneSeries(series)
	nowMs := now.UnixMilli()
	changed := false
	if series.IntervalMs <= 0 {
		series.IntervalMs = seriesBaseIntervalMs
		changed = true
	}

	var completed *float64
	if progress.Completed != nil {
		value := float64(*progress.Completed)
		completed = &value
	}

	// A rate is only meaningful within one unit. A change of unit ends every
	// rate measured in the old one, and keeps the graphed metrics, which carry
	// their own units.
	// A tick that reports no measure at all (an HLS stream muxing, say, between
	// counting segments and finishing) says nothing about the unit, and must
	// not erase the history measured in it.
	noMeasure := progress.Completed == nil && progress.Unit == ""
	unitChanged := false
	if progress.Unit != series.Unit && !noMeasure {
		if len(series.Points) > 0 || series.Anchor != nil {
			for i := range series.Points {
				series.Points[i].Rate = nil
				series.Points[i].Completed = nil
			}
			series.Anchor, series.Rate = nil, nil
		}
		series.ActivityAt = nil
		series.Unit = progress.Unit
		unitChanged = true
		changed = true
	}

	stale := rateStaleAfter.Milliseconds()
	activityAt := series.ActivityAt
	activityWasFresh := activityAt != nil && nowMs-*activityAt >= 0 && nowMs-*activityAt <= stale
	reportedActivityAt := nowMs
	if progress.ActivityAt != nil && progress.ActivityAt.UnixMilli() <= nowMs {
		reportedActivityAt = progress.ActivityAt.UnixMilli()
	}
	reportedActivityAge := nowMs - reportedActivityAt
	activityEvidenceFresh := progress.Activity && reportedActivityAge >= 0 && reportedActivityAge <= stale
	activityCanBridge := activityWasFresh
	if activityEvidenceFresh && !activityWasFresh && activityAt == nil && series.Anchor != nil {
		anchorAge := nowMs - series.Anchor.At
		activityCanBridge = anchorAge >= 0 && anchorAge <= stale
	}
	if activityEvidenceFresh && activityAt != nil && !activityWasFresh {
		// A byte report after an unobserved gap starts a new measurement window;
		// it cannot make the old count delta current again.
		if series.Anchor != nil || series.Rate != nil {
			series.Anchor, series.Rate = nil, nil
			changed = true
		}
	}
	if activityEvidenceFresh {
		if series.ActivityAt == nil || reportedActivityAt > *series.ActivityAt {
			series.ActivityAt = &reportedActivityAt
			changed = true
		}
	}
	switch {
	case final:
		if series.Anchor != nil || series.Rate != nil || series.ActivityAt != nil {
			series.Anchor, series.Rate = nil, nil
			series.ActivityAt = nil
			changed = true
		}
	case completed == nil:
		if series.Anchor != nil || series.Rate != nil || series.ActivityAt != nil {
			series.Anchor, series.Rate = nil, nil
			series.ActivityAt = nil
			changed = true
		}
	case series.Anchor == nil || *completed < series.Anchor.Completed || (nowMs-series.Anchor.At > stale && !activityCanBridge):
		// A first count, a restart that went backwards, or a gap long enough to
		// be a pause: measure afresh from here rather than across it.
		series.Anchor = &RateAnchor{At: nowMs, Completed: *completed}
		series.Rate = nil
		changed = true
	case *completed == series.Anchor.Completed:
		// An unchanged count is never a new count measurement. Fresh activity
		// can keep this anchor's rate visible, but metadata/phase reports cannot
		// rebase it with a zero delta and extend the stale window.
	case nowMs-series.Anchor.At >= rateAnchorIntervalMs:
		elapsed := float64(nowMs-series.Anchor.At) / 1000
		instant := (*completed - series.Anchor.Completed) / elapsed
		if series.Rate != nil {
			instant = (instant + *series.Rate) / 2
		}
		series.Rate = &instant
		series.Anchor = &RateAnchor{At: nowMs, Completed: *completed}
		changed = true
	}
	if (!progress.Activity && series.ActivityAt != nil) || (progress.Activity && !activityEvidenceFresh && !activityWasFresh && series.ActivityAt != nil) {
		series.ActivityAt = nil
		changed = true
	}

	if reuniteMetricUnits(&series, progress.Metrics) {
		changed = true
	}

	point := SeriesPoint{At: nowMs, Completed: completed, RateNeutral: completed == nil && !unitChanged, Values: graphedValues(progress.Metrics)}
	count := len(series.Points)
	switch {
	case count == 0:
		series.Points = append(series.Points, point)
		changed = true
	case nowMs-series.Points[count-1].At >= series.IntervalMs:
		previous := series.Points[count-1]
		point.Rate = pointRate(previous, point, max(stale, 3*series.IntervalMs))
		if pointCountDecreased(previous, point) {
			point.Rate = nil
		}
		setActivityPointRate(&point, series.Rate, progress.Activity, !progress.Activity && activityWasFresh,
			pointCountAdvanced(previous, point), neutralActivityEndpoint(previous, point, unitChanged))
		series.Points = append(series.Points, point)
		changed = true
	case final:
		// Younger than the interval: the final value replaces the latest point,
		// measured against the one before it.
		if count > 1 {
			point.Rate = pointRate(series.Points[count-2], point, max(stale, 3*series.IntervalMs))
		}
		latest := series.Points[count-1]
		decreasedFromLatest := pointCountDecreased(latest, point)
		if decreasedFromLatest {
			// The final rate baseline is the penultimate sample, but a value that
			// fell from the point being replaced signals a restart within that
			// interval. Do not publish a plausible rate across the decrease.
			point.Rate = nil
		}
		movementBaseline := count - 1
		if count > 1 {
			movementBaseline = count - 2
		}
		countAdvanced := count > 1 && pointCountAdvanced(series.Points[movementBaseline], point)
		setActivityPointRate(&point, series.Rate, progress.Activity, !progress.Activity && activityWasFresh,
			countAdvanced, neutralActivityEndpoint(latest, point, unitChanged))
		series.Points[count-1] = point
		changed = true
	}
	if final {
		endAtLastMovement(series.Points)
	}

	for len(series.Points) > MaxSeriesPoints {
		series.Points = compactPoints(series.Points)
		series.IntervalMs *= 2
	}
	pruneMetricUnits(&series)
	return series, changed
}

func setActivityPointRate(point *SeriesPoint, measuredRate *float64, active, activityEnded, countAdvanced, neutralEndpoint bool) {
	switch {
	case activityEnded && !countAdvanced:
		// A phase-only report can end a byte-activity lease, but it is not a
		// count measurement. Mark only this endpoint neutral; any earlier stale,
		// pause, restart, or unit boundary remains on its own point.
		point.Rate = nil
		point.RateNeutral = neutralEndpoint
	case (active || activityEnded) && measuredRate != nil:
		// When activity ends alongside real count movement, retain the
		// measured count rate. Activity describes freshness; Completed remains
		// the graph's measurement.
		rate := *measuredRate
		point.Rate = &rate
	}
}

func pointCountAdvanced(previous, point SeriesPoint) bool {
	return previous.Completed != nil && point.Completed != nil && *point.Completed > *previous.Completed
}

func pointCountDecreased(previous, point SeriesPoint) bool {
	return previous.Completed != nil && point.Completed != nil && *point.Completed < *previous.Completed
}

func neutralActivityEndpoint(previous, point SeriesPoint, unitChanged bool) bool {
	if unitChanged {
		return false
	}
	if point.Completed == nil {
		return true
	}
	return previous.Completed != nil && *previous.Completed == *point.Completed
}

// reuniteMetricUnits drops a graphed key's history when the key comes back in a
// different unit, and records each graphed key's current unit.
func reuniteMetricUnits(series *ProgressSeries, metrics []Metric) bool {
	changed := false
	for _, metric := range metrics {
		if !metric.Graph {
			continue
		}
		previous, known := series.Units[metric.Key]
		if known && previous == metric.Unit {
			continue
		}
		if known {
			for i := range series.Points {
				delete(series.Points[i].Values, metric.Key)
			}
		}
		if series.Units == nil {
			series.Units = make(map[string]string, MaxGraphedMetrics)
		}
		series.Units[metric.Key] = metric.Unit
		changed = true
	}
	return changed
}

// pruneMetricUnits forgets the unit of a key no point holds any more, which
// bounds the map by the keys the series itself still carries.
func pruneMetricUnits(series *ProgressSeries) {
	for key := range series.Units {
		held := false
		for _, point := range series.Points {
			if _, ok := point.Values[key]; ok {
				held = true
				break
			}
		}
		if !held {
			delete(series.Units, key)
		}
	}
}

// pointRate is Completed's change per second between two points, or nil when
// they are not comparable: a count missing on either side, a count that went
// down, or a gap wider than a pause allows.
func pointRate(previous, point SeriesPoint, maxGapMs int64) *float64 {
	if previous.Completed == nil || point.Completed == nil || *point.Completed < *previous.Completed {
		return nil
	}
	gap := point.At - previous.At
	if gap <= 0 || gap > maxGapMs {
		return nil
	}
	rate := (*point.Completed - *previous.Completed) / (float64(gap) / 1000)
	return &rate
}

// compactPoints halves a series' resolution. The first point is kept as it is,
// so the graph always starts where the Job did; the rest are merged in pairs
// onto the later point of each pair, so the latest value is never averaged
// away.
func compactPoints(points []SeriesPoint) []SeriesPoint {
	if len(points) < 3 {
		return points
	}
	out := make([]SeriesPoint, 0, len(points)/2+2)
	out = append(out, points[0])
	rest := points[1:]
	if len(rest)%2 == 1 {
		// An odd count leaves one point unpaired. Pair from the end so the
		// unpaired one is the oldest, not the newest.
		out = append(out, rest[0])
		rest = rest[1:]
	}
	for i := 0; i+1 < len(rest); i += 2 {
		out = append(out, mergePoints(rest[i], rest[i+1]))
	}
	return out
}

func mergePoints(a, b SeriesPoint) SeriesPoint {
	merged := SeriesPoint{At: b.At, Completed: b.Completed}
	// A nil rate is ambiguous by itself: a neutral endpoint has no new count
	// sample but may carry an earlier measured rate through compaction, while an
	// unmarked nil rate is a hard boundary (including old stored points whose
	// meaning cannot be recovered). Never carry a rate through a hard boundary.
	switch {
	case a.Rate != nil && b.Rate != nil:
		rate := (*a.Rate + *b.Rate) / 2
		merged.Rate = &rate
		merged.RateNeutral = b.RateNeutral
	case b.Rate != nil:
		rate := *b.Rate
		merged.Rate = &rate
		merged.RateNeutral = b.RateNeutral
	case b.RateNeutral && a.Rate != nil:
		rate := *a.Rate
		merged.Rate = &rate
		merged.RateNeutral = true
	case b.RateNeutral && a.RateNeutral:
		merged.RateNeutral = true
	}
	// Only the later point's keys survive a merge. Keeping the union would let a
	// reporter that changes which metrics it graphs grow every merged point by
	// one set of keys per compaction, so the series would stay 120 points long
	// and still grow without bound. A metric that stopped being graphed loses
	// the one sample it had in the older half, which is all a merge can lose.
	if len(b.Values) > 0 {
		merged.Values = make(map[string]float64, len(b.Values))
		for key, value := range b.Values {
			if other, ok := a.Values[key]; ok {
				value = (value + other) / 2
			}
			merged.Values[key] = value
		}
	}
	return merged
}

// endAtLastMovement ends a finished series at the last speed it measured. The
// same-count tail is not a slowdown to zero, so sampled rates there are dropped
// and no-sample endpoints become neutral for compaction. A nil hard-gap point
// stops the trim; finalization cannot relabel a real pause, restart, or unit
// change as an endpoint that may inherit an earlier rate. A stall the Job
// moved on from is left as the zero it was.
func endAtLastMovement(points []SeriesPoint) {
	for i := len(points) - 1; i > 0; i-- {
		if points[i].Completed == nil || points[i-1].Completed == nil || !sameCount(points[i].Completed, points[i-1].Completed) {
			return
		}
		if points[i].Rate == nil && !points[i].RateNeutral {
			return
		}
		points[i].Rate = nil
		points[i].RateNeutral = true
	}
}

// sameCount reports whether two points hold the same count, or neither holds one.
func sameCount(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func graphedValues(metrics []Metric) map[string]float64 {
	var values map[string]float64
	for _, metric := range metrics {
		if !metric.Graph {
			continue
		}
		if values == nil {
			values = make(map[string]float64, MaxGraphedMetrics)
		}
		values[metric.Key] = metric.Value
	}
	return values
}

func cloneSeries(series ProgressSeries) ProgressSeries {
	out := series
	out.Points = make([]SeriesPoint, len(series.Points), len(series.Points)+1)
	copy(out.Points, series.Points)
	if series.Anchor != nil {
		anchor := *series.Anchor
		out.Anchor = &anchor
	}
	if series.ActivityAt != nil {
		activityAt := *series.ActivityAt
		out.ActivityAt = &activityAt
	}
	if series.Rate != nil {
		rate := *series.Rate
		out.Rate = &rate
	}
	// Point value maps are replaced, never edited, except by a unit change,
	// which edits them; copy them so the caller's series is left alone.
	for i := range out.Points {
		if out.Points[i].Values != nil {
			values := make(map[string]float64, len(out.Points[i].Values))
			for key, value := range out.Points[i].Values {
				values[key] = value
			}
			out.Points[i].Values = values
		}
	}
	if series.Units != nil {
		out.Units = make(map[string]string, len(series.Units))
		for key, unit := range series.Units {
			out.Units[key] = unit
		}
	}
	return out
}

// ValidateMetrics checks a snapshot's metrics against their ceilings,
// answering ErrInvalidProgress.
func ValidateMetrics(metrics []Metric) error {
	if err := jobmetrics.Validate(metrics); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidProgress, err.Error())
	}
	return nil
}

// encodeMetrics is the stored form of a snapshot's metrics: nil for none, so a
// Job that never reported any keeps a NULL column rather than "[]".
func encodeMetrics(metrics []Metric) (types.JSON, error) {
	if len(metrics) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("jobs: encode progress metrics: %w", err)
	}
	return types.JSON(encoded), nil
}

// decodeMetrics and decodeSeries read what encodeMetrics and applyProgressHistory
// stored. A column that does not decode reads as empty rather than failing the
// snapshot: it is display history, and a Job must stay readable without it.
func decodeMetrics(raw types.JSON) []Metric {
	if len(raw) == 0 {
		return nil
	}
	var metrics []Metric
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return nil
	}
	return metrics
}

func decodeSeries(raw types.JSON) ProgressSeries {
	var series ProgressSeries
	if len(raw) == 0 {
		return series
	}
	if err := json.Unmarshal(raw, &series); err != nil {
		return ProgressSeries{}
	}
	return series
}

// AverageRate is what a Job that is not running reports in place of a speed:
// the amount it counted per second of running. Time spent queued, paused or
// blocked is left out, and work done before the first report is kept in: the
// count a Job reports is its own work, counted from zero. (A Job that reports
// counts carried over from earlier work, a Continue whose first report is "120
// of 500", is averaged as though it did them; nothing on the Job says which of
// its first count it did itself.) It is nil while the Job runs, since the
// running time of the current stint is banked only when it ends; nil when the
// Job counted nothing; and measured across the series
// (ProgressSeries.AverageRate) when no running time was banked.
func (s Snapshot) AverageRate() *float64 {
	if s.State == StateRunning {
		return nil
	}
	running := s.RunningDuration.Seconds()
	if running <= 0 {
		return s.ProgressSeries.AverageRate()
	}
	_, last := s.ProgressSeries.countedEnds()
	if last == nil || *last.Completed <= 0 {
		return nil
	}
	rate := *last.Completed / running
	return &rate
}

// LiveRate is the Job's current speed, answered only while it runs: a paused
// or finished Job has no speed, whatever its last measurement said.
func (s Snapshot) LiveRate(now time.Time) *float64 {
	if s.State != StateRunning {
		return nil
	}
	return s.ProgressSeries.CurrentRate(now)
}

// ExpectedFinish is the executor's own ETA when it gave one, otherwise an
// estimate from the live rate. Estimated reports which, so a reader can say
// "about".
func (s Snapshot) ExpectedFinish(now time.Time) (eta *time.Time, estimated bool) {
	if s.Progress.ETA != nil {
		reported := *s.Progress.ETA
		return &reported, false
	}
	if estimate := EstimateETA(s.Progress, s.LiveRate(now), now); estimate != nil {
		return estimate, true
	}
	return nil, false
}
