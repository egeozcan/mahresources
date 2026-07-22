package mrqlbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AggregateOnly removes per-sample traces while retaining percentile, SQL,
// row, output-byte, environment, fingerprint, and native-plan summaries. Use it
// for committed reference/canonical baselines.
func AggregateOnly(artifact RunArtifact) RunArtifact {
	artifact.Results = append([]ScenarioResult(nil), artifact.Results...)
	for i := range artifact.Results {
		artifact.Results[i].Samples = nil
	}
	return artifact
}

func WriteJSONAtomic(path string, value any) error {
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return nil
}

func ReadManifest(path string) (FixtureManifest, error) {
	var manifest FixtureManifest
	if err := readJSON(path, &manifest); err != nil {
		return manifest, err
	}
	if manifest.SchemaVersion != ArtifactSchemaVersion || manifest.GeneratorVersion != GeneratorVersion || manifest.Marker != "mahresources-mrql-benchmark" {
		return manifest, fmt.Errorf("manifest %q is incompatible", path)
	}
	return manifest, nil
}

func ReadRun(path string) (RunArtifact, error) {
	var artifact RunArtifact
	if err := readJSON(path, &artifact); err != nil {
		return artifact, err
	}
	if artifact.SchemaVersion != ArtifactSchemaVersion || artifact.CatalogVersion != ScenarioCatalogVersion {
		return artifact, fmt.Errorf("run artifact %q is incompatible", path)
	}
	return artifact, nil
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
