package template_context_providers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"mahresources/jobs"
)

// jobRowStats is a card's stats line, formatted the way every Job surface
// formats it (jobStatsText in src/components/jobCenter.js) so they read alike:
// the amount counted, then the live rate and time left while the Job runs or
// the average rate once it has ended. A rate in percent says nothing a reader
// can use and is left out.
func jobRowStats(snapshot jobs.Snapshot, now time.Time) string {
	unit := snapshot.Progress.Unit
	var parts []string
	if amount := formatJobAmount(snapshot.Progress, snapshot.State.Terminal()); amount != "" {
		parts = append(parts, amount)
	}
	if snapshot.State == jobs.StateRunning {
		if rate := formatJobRate(snapshot.LiveRate(now), unit); rate != "" {
			parts = append(parts, rate)
		}
		if eta, estimated := snapshot.ExpectedFinish(now); eta != nil {
			if left := formatJobEta(eta.Sub(now), estimated); left != "" {
				parts = append(parts, left)
			}
		}
	} else if snapshot.State.Terminal() {
		if rate := formatJobRate(snapshot.AverageRate(), unit); rate != "" {
			parts = append(parts, "average "+rate)
		}
	}
	return strings.Join(parts, " · ")
}

// formatJobEta is formatEta in jobProgress.js: "about 14 s left" for an
// estimate, "14 s left" for an executor's own ETA, and "almost done" for an
// estimate under a second.
func formatJobEta(left time.Duration, estimated bool) string {
	switch {
	case estimated && left < time.Second:
		return "almost done"
	case left <= 0:
		return ""
	case estimated:
		return "about " + formatJobDuration(left) + " left"
	default:
		return formatJobDuration(left) + " left"
	}
}

// formatJobRate is formatRate in jobProgress.js: a rate in its unit, per second,
// or per minute or per hour when it is too slow to show per second.
func formatJobRate(rate *float64, unit string) string {
	if rate == nil || *rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0) || unit == "percent" {
		return ""
	}
	value, per := *rate, "s"
	if !jobRateShowsPerSecond(value, unit) {
		value, per = value*60, "min"
		if !jobRateShowsPerSecond(value, unit) {
			value, per = value*60, "h"
		}
	}
	switch unit {
	case "bytes":
		return formatJobBytes(value) + "/" + per
	case "":
		return formatJobNumber(value) + "/" + per
	default:
		return formatJobNumber(value) + " " + unit + "/" + per
	}
}

// jobRateShowsPerSecond is showsPerSecond in jobProgress.js: the smallest rate a
// figure per second can show is a byte, or a tenth of a count.
func jobRateShowsPerSecond(rate float64, unit string) bool {
	if unit == "bytes" {
		return rate == 0 || rate >= 1
	}
	return rate == 0 || rate >= 0.1
}

// formatJobAmount is formatAmount in jobProgress.js: "12.3 MB of 40 MB", "3 of
// 12 items", or the completed amount alone. A finished Job's amount that reached
// its total is the amount alone.
func formatJobAmount(progress jobs.Progress, finished bool) string {
	if progress.Completed == nil || progress.Unit == "percent" {
		return ""
	}
	completed := float64(*progress.Completed)
	if progress.Total != nil && *progress.Total > 0 && !(finished && *progress.Completed >= *progress.Total) {
		total := float64(*progress.Total)
		if progress.Unit == "bytes" {
			return formatJobByteAmount(completed, total)
		}
		suffix := ""
		if progress.Unit != "" {
			suffix = " " + progress.Unit
		}
		return formatJobNumber(completed) + " of " + formatJobNumber(total) + suffix
	}
	if progress.Unit == "items" {
		return formatJobNumber(completed) + " items"
	}
	return formatJobQuantity(completed, progress.Unit)
}

var jobByteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

func jobByteUnit(value float64) int {
	unit := 0
	for value >= 1024 && unit < len(jobByteUnits)-1 {
		value /= 1024
		unit++
	}
	return unit
}

// formatJobBytesIn is a byte count in the given unit: two decimals below 1 of
// it, one below 100, none above; floor rounds down.
func formatJobBytesIn(value float64, unit int, floor bool) string {
	value /= math.Pow(1024, float64(unit))
	digits := 1
	switch {
	case unit == 0 || value >= 100:
		digits = 0
	case value < 1:
		digits = 2
	}
	scale := math.Pow(10, float64(digits))
	if floor {
		value = math.Floor(value*scale) / scale
	} else {
		value = math.Round(value*scale) / scale
	}
	return strconv.FormatFloat(value, 'f', digits, 64) + " " + jobByteUnits[unit]
}

func formatJobBytes(value float64) string {
	return formatJobBytesIn(value, jobByteUnit(value), false)
}

// formatJobByteAmount is formatByteAmount in jobProgress.js: a completed amount
// at least a tenth of its total is shown in the total's unit, and one short of
// its total is rounded down, so it never reads as the total before it is.
func formatJobByteAmount(completed, total float64) string {
	unit := jobByteUnit(completed)
	if completed >= total/10 && completed <= total {
		unit = jobByteUnit(total)
	}
	return formatJobBytesIn(completed, unit, completed < total) + " of " + formatJobBytes(total)
}

// formatJobNumber is formatNumber in jobProgress.js: whole above 100, one
// decimal below, and grouped by thousands as en-US groups them.
func formatJobNumber(value float64) string {
	if math.Abs(value) >= 100 || value == math.Trunc(value) {
		return groupThousands(strconv.FormatFloat(math.Round(value), 'f', 0, 64))
	}
	return strconv.FormatFloat(math.Round(value*10)/10, 'f', -1, 64)
}

func groupThousands(digits string) string {
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return sign + digits
}

func formatJobDuration(d time.Duration) string {
	seconds := d.Seconds()
	switch {
	case seconds < 1:
		return "under 1 s"
	case seconds < 60:
		return fmt.Sprintf("%.0f s", seconds)
	}
	minutes := int(math.Round(seconds / 60))
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours, rest := minutes/60, minutes%60
	if hours < 24 {
		if rest > 0 {
			return fmt.Sprintf("%d h %d min", hours, rest)
		}
		return fmt.Sprintf("%d h", hours)
	}
	days, hoursLeft := hours/24, hours%24
	if hoursLeft > 0 {
		return fmt.Sprintf("%d d %d h", days, hoursLeft)
	}
	return fmt.Sprintf("%d d", days)
}

// jobMetricSummary is one metric as a line of text, its label and its value, as
// the Jobs drawer and the detail page list it (metricSummary in jobProgress.js).
func jobMetricSummary(metric jobs.Metric) string {
	label := metric.Label
	if label == "" {
		label = metric.Key
	}
	return label + ": " + formatJobMetric(metric)
}

// formatJobMetric is formatMetric in jobProgress.js: the value in its unit, and
// its total when it has one.
func formatJobMetric(metric jobs.Metric) string {
	value := formatJobQuantity(metric.Value, metric.Unit)
	if metric.Total == nil {
		return value
	}
	if metric.Unit == "bytes" {
		return value + " of " + formatJobBytes(*metric.Total)
	}
	suffix := ""
	if metric.Unit != "" && metric.Unit != "items" {
		suffix = " " + metric.Unit
	}
	return formatJobNumber(metric.Value) + " of " + formatJobNumber(*metric.Total) + suffix
}

// formatJobQuantity is formatQuantity in jobProgress.js.
func formatJobQuantity(value float64, unit string) string {
	switch unit {
	case "bytes":
		return formatJobBytes(value)
	case "percent":
		return formatJobNumber(value) + "%"
	case "seconds":
		return formatJobDuration(time.Duration(value * float64(time.Second)))
	case "ms":
		return formatJobNumber(value) + " ms"
	case "", "items":
		return formatJobNumber(value)
	default:
		return formatJobNumber(value) + " " + unit
	}
}
