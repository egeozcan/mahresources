// Package jobmetrics is the value type a Job's progress metrics share across
// every surface that reports them: the Job Service stores them, and the plugin
// runtime and the command runner accept them from plugin code. It is a leaf
// under models/ so that those callers can validate a metric where it was made
// without importing the control plane: plugin_system may not import jobs, and
// plugin_commands may import nothing above models/.
package jobmetrics

import (
	"fmt"
	"math"
	"regexp"
)

// Ceilings. A Job's metrics render as a row of labelled values in the Jobs
// drawer and on the detail page, and every graphed one draws its own chart, so
// both counts are bounded where they are accepted rather than trimmed where
// they are drawn.
const (
	MaxMetrics    = 8
	MaxGraphed    = 3
	MaxKeyBytes   = 40
	MaxLabelBytes = 60
	MaxUnitBytes  = 20
)

var keyPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Metric is one named figure an executor reports beside its primary measure:
// segments fetched next to the bytes they held, rows scanned by a plugin, a
// queue depth. It is part of a progress snapshot, so each report replaces the
// whole set.
//
// Graph asks for the metric's history to be kept in the Job's progress series
// and drawn. It is opt-in because a graph is only worth its space for a figure
// that moves.
type Metric struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value float64  `json:"value"`
	Total *float64 `json:"total,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Graph bool     `json:"graph,omitempty"`
}

// Validate checks metrics against their ceilings. A value that is not a
// finite, non-negative number cannot be drawn or compared, so it is refused
// rather than stored. The message names the metric, because the reader of it
// is a plugin author.
func Validate(metrics []Metric) error {
	if len(metrics) > MaxMetrics {
		return fmt.Errorf("%d metrics, over the %d-metric ceiling", len(metrics), MaxMetrics)
	}
	seen := make(map[string]bool, len(metrics))
	graphed := 0
	for i, metric := range metrics {
		switch {
		case metric.Key == "":
			return fmt.Errorf("metric %d has no key", i+1)
		case len(metric.Key) > MaxKeyBytes:
			return fmt.Errorf("metric key %q is over the %d-byte ceiling", metric.Key, MaxKeyBytes)
		case !keyPattern.MatchString(metric.Key):
			return fmt.Errorf("metric key %q may only use a-z, 0-9, _ and -", metric.Key)
		case seen[metric.Key]:
			return fmt.Errorf("metric key %q is repeated", metric.Key)
		case len(metric.Label) > MaxLabelBytes:
			return fmt.Errorf("metric %q label is over the %d-byte ceiling", metric.Key, MaxLabelBytes)
		case len(metric.Unit) > MaxUnitBytes:
			return fmt.Errorf("metric %q unit is over the %d-byte ceiling", metric.Key, MaxUnitBytes)
		case !finiteNonNegative(metric.Value):
			return fmt.Errorf("metric %q value must be a finite number of at least zero", metric.Key)
		case metric.Total != nil && !finiteNonNegative(*metric.Total):
			return fmt.Errorf("metric %q total must be a finite number of at least zero", metric.Key)
		}
		seen[metric.Key] = true
		if metric.Graph {
			graphed++
		}
	}
	if graphed > MaxGraphed {
		return fmt.Errorf("%d graphed metrics, over the %d-graph ceiling", graphed, MaxGraphed)
	}
	return nil
}

// Clone copies a metric set so a holder never shares a Total pointer with the
// caller that reported it.
func Clone(metrics []Metric) []Metric {
	if len(metrics) == 0 {
		return nil
	}
	out := make([]Metric, len(metrics))
	for i, metric := range metrics {
		out[i] = metric
		if metric.Total != nil {
			total := *metric.Total
			out[i].Total = &total
		}
	}
	return out
}

func finiteNonNegative(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}
