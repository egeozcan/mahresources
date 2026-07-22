package mrqlbench

import (
	"errors"
	"sort"
)

func CalculatePercentiles(samples []int64) (Percentiles, error) {
	if len(samples) == 0 {
		return Percentiles{}, errors.New("at least one sample is required")
	}
	ordered := append([]int64(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	result := Percentiles{
		Samples: len(ordered),
		P50:     nearestRank(ordered, 50),
	}
	if len(ordered) >= 20 {
		result.P95 = nearestRank(ordered, 95)
	}
	if len(ordered) >= 100 {
		p99 := nearestRank(ordered, 99)
		result.P99 = &p99
	}
	return result, nil
}

func SummarizeMetric(values []int64) (MetricSummary, error) {
	if len(values) == 0 {
		return MetricSummary{}, errors.New("at least one metric value is required")
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return MetricSummary{Samples: len(ordered), Minimum: ordered[0], Maximum: ordered[len(ordered)-1], P50: nearestRank(ordered, 50)}, nil
}

func nearestRank(ordered []int64, percentile int) int64 {
	rank := (percentile*len(ordered) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(ordered) {
		rank = len(ordered)
	}
	return ordered[rank-1]
}
