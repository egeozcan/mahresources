package mrqlbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAggregateOnlyDropsPerSampleTraces(t *testing.T) {
	artifact := RunArtifact{Results: []ScenarioResult{{ScenarioID: "x", Samples: []Sample{{ElapsedNanos: 1}}, SQLStatements: MetricSummary{P50: 2}}}}
	compacted := AggregateOnly(artifact)
	if compacted.Results[0].Samples != nil {
		t.Fatalf("samples remain: %#v", compacted.Results[0].Samples)
	}
	if compacted.Results[0].SQLStatements.P50 != 2 || len(artifact.Results[0].Samples) != 1 {
		t.Fatal("compaction lost summaries or mutated input")
	}
}

func TestWriteJSONAtomicAndReadManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "manifest.json")
	manifest := FixtureManifest{SchemaVersion: ArtifactSchemaVersion, GeneratorVersion: GeneratorVersion, Marker: "mahresources-mrql-benchmark", Dialect: "sqlite"}
	if err := WriteJSONAtomic(path, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Dialect != "sqlite" {
		t.Fatalf("loaded = %#v", loaded)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".manifest.json-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary artifacts remain: %v", matches)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
