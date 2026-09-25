package template_context_providers

import (
	"fmt"
	"math"
	"strings"
	"time"

	"mahresources/jobs"
)

// jobRowStats is a card's speed line, formatted the way the Jobs drawer
// formats it (src/components/jobProgress.js) so the two read alike: the live
// rate and time left while a Job runs, the average rate once it has finished.
// A rate in percent says nothing a reader can use and is left out.
func jobRowStats(snapshot jobs.Snapshot, now time.Time) string {
	unit := snapshot.Progress.Unit
	var parts []string
	if snapshot.State == jobs.StateRunning {
		if rate := formatJobRate(snapshot.LiveRate(now), unit); rate != "" {
			parts = append(parts, rate)
		}
		if eta, estimated := snapshot.ExpectedFinish(now); eta != nil {
			if left := eta.Sub(now); left > 0 {
				text := formatJobDuration(left) + " left"
				if estimated {
					text = "about " + text
				}
				parts = append(parts, text)
			}
		}
	} else if snapshot.State.Terminal() {
		if rate := formatJobRate(snapshot.ProgressSeries.AverageRate(), unit); rate != "" {
			parts = append(parts, "average "+rate)
		}
	}
	return strings.Join(parts, " · ")
}

func formatJobRate(rate *float64, unit string) string {
	if rate == nil || *rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0) || unit == "percent" {
		return ""
	}
	switch unit {
	case "bytes":
		return formatJobBytes(*rate) + "/s"
	case "", "items":
		return formatJobNumber(*rate) + "/s"
	default:
		return formatJobNumber(*rate) + " " + unit + "/s"
	}
}

func formatJobBytes(value float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 || value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatJobNumber(value float64) string {
	if math.Abs(value) >= 100 || value == math.Trunc(value) {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.1f", value)
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
