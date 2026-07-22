package mrqlbench

import (
	"fmt"
	"reflect"
	"sort"
)

func Compare(reference, candidate RunArtifact) Comparison {
	comparison := Comparison{Compatible: true, Deltas: []ScenarioDelta{}}
	if reference.SchemaVersion != candidate.SchemaVersion || reference.CatalogVersion != candidate.CatalogVersion {
		comparison.Compatible = false
		comparison.Errors = append(comparison.Errors, "artifact schema or scenario catalog versions differ")
	}
	if reference.Status != candidate.Status {
		comparison.Compatible = false
		comparison.Errors = append(comparison.Errors, "artifact status differs")
	}
	if reference.Warmups != candidate.Warmups || reference.MeasuredSamples != candidate.MeasuredSamples || !reflect.DeepEqual(reference.Configuration, candidate.Configuration) {
		comparison.Compatible = false
		comparison.Errors = append(comparison.Errors, "warmup or measured sample counts differ")
	}
	if reference.Fixture.LogicalChecksum != candidate.Fixture.LogicalChecksum ||
		reference.Fixture.Dialect != candidate.Fixture.Dialect ||
		reference.Fixture.GeneratorVersion != candidate.Fixture.GeneratorVersion ||
		reference.Fixture.SchemaRevision != candidate.Fixture.SchemaRevision ||
		!reflect.DeepEqual(reference.Fixture.Profile, candidate.Fixture.Profile) {
		comparison.Compatible = false
		comparison.Errors = append(comparison.Errors, "fixture profile, generator, schema, checksum, or dialect differs")
	}
	if !compatibleEnvironment(reference.Environment, candidate.Environment) {
		comparison.Compatible = false
		comparison.Errors = append(comparison.Errors, "runtime, host, database, pool, or concurrency environment differs")
	}

	refs, refErr := uniqueResultMap(reference.Results)
	candidates, candidateErr := uniqueResultMap(candidate.Results)
	if refErr != nil || candidateErr != nil {
		comparison.Compatible = false
		if refErr != nil {
			comparison.Errors = append(comparison.Errors, "reference: "+refErr.Error())
		}
		if candidateErr != nil {
			comparison.Errors = append(comparison.Errors, "candidate: "+candidateErr.Error())
		}
		return comparison
	}
	ids := make([]string, 0, len(refs))
	for id, ref := range refs {
		ids = append(ids, id)
		candidateResult, ok := candidates[id]
		if !ok {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("candidate is missing scenario %q", id))
			continue
		}
		if ref.QueryFingerprint != candidateResult.QueryFingerprint {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("scenario %q query fingerprint differs", id))
		}
		if ref.SQLStatements != candidateResult.SQLStatements {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("scenario %q SQL statement summary differs", id))
		}
		if ref.Rows != candidateResult.Rows {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("scenario %q row summary differs", id))
		}
		scenario, _ := ScenarioByID(id)
		if scenario.StochasticOutput {
			if !metricWithinBounds(ref.OutputBytes, scenario.MinimumOutputBytes, scenario.MaximumOutputBytes) ||
				!metricWithinBounds(candidateResult.OutputBytes, scenario.MinimumOutputBytes, scenario.MaximumOutputBytes) {
				comparison.Compatible = false
				comparison.Errors = append(comparison.Errors, fmt.Sprintf("scenario %q stochastic output is outside declared bounds", id))
			}
		} else if ref.OutputBytes != candidateResult.OutputBytes {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("scenario %q output-byte summary differs", id))
		}
	}
	for id := range candidates {
		if _, ok := refs[id]; !ok {
			comparison.Compatible = false
			comparison.Errors = append(comparison.Errors, fmt.Sprintf("candidate has unknown scenario %q", id))
		}
	}
	if !comparison.Compatible {
		return comparison
	}

	sort.Strings(ids)
	for _, id := range ids {
		ref := refs[id]
		got := candidates[id]
		absolute := got.Latency.P50 - ref.Latency.P50
		relative := 0.0
		if ref.Latency.P50 != 0 {
			relative = float64(absolute) / float64(ref.Latency.P50) * 100
		}
		policy := TimingRegressionPolicy{RelativePercent: 15, MinimumNanos: 1_000_000}
		if scenario, ok := ScenarioByID(id); ok {
			policy = scenario.TimingPolicy
			if policy.RelativePercent == 0 {
				policy.RelativePercent = 15
			}
		}
		delta := ScenarioDelta{
			ScenarioID: id, BaselineP50: ref.Latency.P50, CandidateP50: got.Latency.P50,
			RelativePercent: relative, AbsoluteNanos: absolute,
			Regression: absolute >= policy.MinimumNanos && relative >= policy.RelativePercent,
		}
		comparison.Deltas = append(comparison.Deltas, delta)
		if delta.Regression {
			comparison.Regressions = append(comparison.Regressions, delta)
		}
	}
	return comparison
}

func metricWithinBounds(metric MetricSummary, minimum, maximum int64) bool {
	return maximum > 0 && metric.Minimum >= minimum && metric.Maximum <= maximum && metric.P50 >= minimum && metric.P50 <= maximum
}

func compatibleEnvironment(a, b Environment) bool {
	return a.GoVersion == b.GoVersion && a.OS == b.OS && a.Arch == b.Arch && a.HostID == b.HostID &&
		a.CPUModel == b.CPUModel && a.CPUCount == b.CPUCount && a.MemoryBytes == b.MemoryBytes &&
		a.Database == b.Database && a.DatabaseVersion == b.DatabaseVersion &&
		a.PoolSize == b.PoolSize && a.Concurrency == b.Concurrency
}

func uniqueResultMap(results []ScenarioResult) (map[string]ScenarioResult, error) {
	out := make(map[string]ScenarioResult, len(results))
	for _, result := range results {
		if result.ScenarioID == "" {
			return nil, fmt.Errorf("scenario result has an empty ID")
		}
		if _, exists := out[result.ScenarioID]; exists {
			return nil, fmt.Errorf("duplicate scenario %q", result.ScenarioID)
		}
		out[result.ScenarioID] = result
	}
	return out, nil
}
