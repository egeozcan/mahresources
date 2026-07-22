package mrqlbench

import (
	"encoding/json"
	"testing"
	"time"

	"mahresources/mrql"
)

func TestStandardProfilesAreResourceLed(t *testing.T) {
	want := map[string]int{"100k": 100_000, "1m": 1_000_000, "3m": 3_000_000}
	for _, profile := range Profiles() {
		if profile.Resources != want[profile.ID] {
			t.Errorf("profile %s resources = %d, want %d", profile.ID, profile.Resources, want[profile.ID])
		}
		if profile.Notes <= 0 || profile.Groups <= 0 || profile.Tags <= 0 {
			t.Errorf("profile %s has incomplete derived cardinalities: %#v", profile.ID, profile)
		}
	}
}

func TestScenarioCatalogIsUniqueAndParses(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatal(err)
	}
	families := map[string]bool{}
	for _, scenario := range Scenarios() {
		families[scenario.Family] = true
		q, err := mrql.Parse(scenario.Query)
		if err != nil {
			t.Errorf("scenario %s does not parse: %v", scenario.ID, err)
			continue
		}
		if err := mrql.Validate(q); err != nil {
			t.Errorf("scenario %s does not validate: %v", scenario.ID, err)
		}
	}
	for _, required := range []string{"scalar", "relation", "scope", "hierarchy", "metadata", "fts", "similarity", "pagination", "aggregate", "bucket", "cross-entity", "rendering"} {
		if !families[required] {
			t.Errorf("scenario family %q is missing", required)
		}
	}
}

func TestCalculatePercentilesUsesNearestRankAndRequiresSamplesForP99(t *testing.T) {
	twenty := make([]int64, 20)
	for i := range twenty {
		twenty[i] = int64(i + 1)
	}
	got, err := CalculatePercentiles(twenty)
	if err != nil {
		t.Fatal(err)
	}
	if got.P50 != 10 || got.P95 != 19 || got.P99 != nil {
		t.Fatalf("20-sample percentiles = %#v", got)
	}

	hundred := make([]int64, 100)
	for i := range hundred {
		hundred[i] = int64(100 - i)
	}
	got, err = CalculatePercentiles(hundred)
	if err != nil {
		t.Fatal(err)
	}
	if got.P99 == nil || *got.P99 != 99 {
		t.Fatalf("100-sample p99 = %#v", got)
	}
}

func TestCompareRejectsIncompatibleArtifactsAndFlagsRegression(t *testing.T) {
	base := RunArtifact{
		SchemaVersion: ArtifactSchemaVersion, CatalogVersion: ScenarioCatalogVersion,
		Fixture:     FixtureManifest{Dialect: "sqlite", LogicalChecksum: "same"},
		Environment: Environment{DatabaseVersion: "3", Concurrency: 1},
		Results:     []ScenarioResult{{ScenarioID: "scalar-selective", QueryFingerprint: "shape", Latency: Percentiles{P50: 10_000_000}}},
	}
	candidate := base
	candidate.Results = []ScenarioResult{{ScenarioID: "scalar-selective", QueryFingerprint: "shape", Latency: Percentiles{P50: 12_000_000}}}
	comparison := Compare(base, candidate)
	if !comparison.Compatible || len(comparison.Regressions) != 1 {
		t.Fatalf("comparison = %#v", comparison)
	}

	candidate.Fixture.LogicalChecksum = "different"
	comparison = Compare(base, candidate)
	if comparison.Compatible || len(comparison.Errors) == 0 {
		t.Fatalf("incompatible comparison = %#v", comparison)
	}
}

func TestCompareRejectsDeterministicMetricChangesAndDuplicateScenarios(t *testing.T) {
	base := RunArtifact{
		SchemaVersion: ArtifactSchemaVersion, CatalogVersion: ScenarioCatalogVersion, Status: "reference", Warmups: 1, MeasuredSamples: 100,
		Fixture:     FixtureManifest{Dialect: "sqlite", GeneratorVersion: GeneratorVersion, SchemaRevision: "schema", LogicalChecksum: "same", Profile: TinyProfile()},
		Environment: Environment{GoVersion: "go", OS: "os", Arch: "arch", CPUModel: "cpu", CPUCount: 1, MemoryBytes: 1, Database: "sqlite", DatabaseVersion: "3", PoolSize: 1, Concurrency: 1},
		Results:     []ScenarioResult{{ScenarioID: "scalar-selective", QueryFingerprint: "shape", Latency: Percentiles{P50: 1}, SQLStatements: MetricSummary{Samples: 100, Minimum: 1, Maximum: 1, P50: 1}}},
	}
	candidate := base
	candidate.Results = append([]ScenarioResult(nil), base.Results...)
	candidate.Results[0].SQLStatements.P50 = 2
	if got := Compare(base, candidate); got.Compatible {
		t.Fatalf("SQL-count mismatch compared as compatible: %#v", got)
	}
	candidate = base
	candidate.Results = append(candidate.Results, candidate.Results[0])
	if got := Compare(base, candidate); got.Compatible {
		t.Fatalf("duplicate scenario compared as compatible: %#v", got)
	}
}

func TestCompareAllowsBoundedStochasticOutputVariation(t *testing.T) {
	base := RunArtifact{
		SchemaVersion: ArtifactSchemaVersion, CatalogVersion: ScenarioCatalogVersion, Status: "raw", Warmups: 0, MeasuredSamples: 3,
		Fixture:     FixtureManifest{Dialect: "sqlite", GeneratorVersion: GeneratorVersion, SchemaRevision: "schema", LogicalChecksum: "same", Profile: TinyProfile()},
		Environment: Environment{GoVersion: "go", OS: "os", Arch: "arch", CPUModel: "cpu", CPUCount: 1, MemoryBytes: 1, Database: "sqlite", DatabaseVersion: "3", PoolSize: 1, Concurrency: 1},
		Results:     []ScenarioResult{{ScenarioID: "cross-entity-random", QueryFingerprint: "shape", Latency: Percentiles{P50: 1}, SQLStatements: MetricSummary{Samples: 3, Minimum: 3, Maximum: 3, P50: 3}, Rows: MetricSummary{Samples: 3, Minimum: 50, Maximum: 50, P50: 50}, OutputBytes: MetricSummary{Samples: 3, Minimum: 29_000, Maximum: 31_000, P50: 30_000}}},
	}
	candidate := base
	candidate.Results = append([]ScenarioResult(nil), base.Results...)
	candidate.Results[0].OutputBytes = MetricSummary{Samples: 3, Minimum: 28_000, Maximum: 32_000, P50: 31_000}
	if got := Compare(base, candidate); !got.Compatible {
		t.Fatalf("bounded stochastic output compared as incompatible: %#v", got)
	}
}

func TestRunArtifactJSONDoesNotRequirePerSampleTrace(t *testing.T) {
	artifact := RunArtifact{
		SchemaVersion:  ArtifactSchemaVersion,
		CatalogVersion: ScenarioCatalogVersion,
		Status:         "reference",
		StartedAt:      time.Unix(0, 0).UTC(),
		Results:        []ScenarioResult{{ScenarioID: "x", Samples: []Sample{{ElapsedNanos: 1}}}},
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunArtifact
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Results[0].Samples[0].ElapsedNanos != 1 {
		t.Fatalf("round trip = %#v", decoded)
	}
}
