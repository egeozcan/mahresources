package mrqlbench

import "time"

const (
	ArtifactSchemaVersion  = 5
	GeneratorVersion       = "mrql-bench-fixture-v7"
	ScenarioCatalogVersion = "mrql-bench-scenarios-v3"
)

type Profile struct {
	ID        string `json:"id"`
	Resources int    `json:"resources"`
	Notes     int    `json:"notes"`
	Groups    int    `json:"groups"`
	Tags      int    `json:"tags"`
	Seed      int64  `json:"seed"`
}

type FixtureManifest struct {
	SchemaVersion    int              `json:"schemaVersion"`
	GeneratorVersion string           `json:"generatorVersion"`
	Profile          Profile          `json:"profile"`
	Dialect          string           `json:"dialect"`
	DatabaseVersion  string           `json:"databaseVersion"`
	SchemaRevision   string           `json:"schemaRevision"`
	FixedEpoch       time.Time        `json:"fixedEpoch"`
	BatchSize        int              `json:"batchSize"`
	Features         map[string]bool  `json:"features"`
	Counts           map[string]int64 `json:"counts"`
	FTSChecks        map[string]int64 `json:"ftsChecks"`
	FTSDigest        string           `json:"ftsDigest"`
	Anchors          map[string]any   `json:"anchors"`
	LogicalChecksum  string           `json:"logicalChecksum"`
	PreparedAt       time.Time        `json:"preparedAt"`
	PreparationNanos int64            `json:"preparationNanos"`
	Marker           string           `json:"marker"`
}

type Scenario struct {
	ID                   string                 `json:"id"`
	Family               string                 `json:"family"`
	Description          string                 `json:"description"`
	Query                string                 `json:"query"`
	Mode                 string                 `json:"mode"`
	RenderMode           string                 `json:"renderMode,omitempty"`
	Page                 int                    `json:"page,omitempty"`
	ScopeClass           string                 `json:"scopeClass"`
	RequiredFeatures     []string               `json:"requiredFeatures,omitempty"`
	MinimumSQLStatements int                    `json:"minimumSqlStatements"`
	MaximumSQLStatements int                    `json:"maximumSqlStatements"`
	CheckRows            bool                   `json:"checkRows"`
	MinimumRows          int64                  `json:"minimumRows"`
	MaximumRows          int64                  `json:"maximumRows"`
	StochasticOutput     bool                   `json:"stochasticOutput,omitempty"`
	MinimumOutputBytes   int64                  `json:"minimumOutputBytes,omitempty"`
	MaximumOutputBytes   int64                  `json:"maximumOutputBytes,omitempty"`
	TimingPolicy         TimingRegressionPolicy `json:"timingPolicy"`
}

type TimingRegressionPolicy struct {
	RelativePercent float64 `json:"relativePercent"`
	MinimumNanos    int64   `json:"minimumNanos"`
}

type Environment struct {
	GitRevision     string `json:"gitRevision"`
	GoVersion       string `json:"goVersion"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	HostID          string `json:"hostId,omitempty"`
	CPUModel        string `json:"cpuModel"`
	CPUCount        int    `json:"cpuCount"`
	MemoryBytes     uint64 `json:"memoryBytes"`
	Database        string `json:"database"`
	DatabaseVersion string `json:"databaseVersion"`
	PoolSize        int    `json:"poolSize"`
	Concurrency     int    `json:"concurrency"`
}

type StatementObservation struct {
	Fingerprint  string `json:"fingerprint"`
	Class        string `json:"class"`
	Rows         int64  `json:"rows"`
	ElapsedNanos int64  `json:"elapsedNanos"`
	Error        string `json:"error,omitempty"`
}

type Sample struct {
	ElapsedNanos  int64                  `json:"elapsedNanos"`
	PhaseNanos    map[string]int64       `json:"phaseNanos,omitempty"`
	SQLStatements int                    `json:"sqlStatements"`
	Rows          int64                  `json:"rows"`
	OutputBytes   int64                  `json:"outputBytes"`
	Allocations   uint64                 `json:"allocations,omitempty"`
	CacheHits     int                    `json:"cacheHits,omitempty"`
	CacheMisses   int                    `json:"cacheMisses,omitempty"`
	Statements    []StatementObservation `json:"statements,omitempty"`
}

type Percentiles struct {
	Samples int    `json:"samples"`
	P50     int64  `json:"p50Nanos"`
	P95     int64  `json:"p95Nanos,omitempty"`
	P99     *int64 `json:"p99Nanos,omitempty"`
}

type PlanArtifact struct {
	SQLFingerprint string   `json:"sqlFingerprint"`
	Dialect        string   `json:"dialect"`
	Format         string   `json:"format"`
	Plan           any      `json:"plan"`
	Signature      []string `json:"signature"`
}

type MetricSummary struct {
	Samples int   `json:"samples"`
	Minimum int64 `json:"minimum"`
	Maximum int64 `json:"maximum"`
	P50     int64 `json:"p50"`
}

type ScenarioResult struct {
	ScenarioID       string         `json:"scenarioId"`
	QueryFingerprint string         `json:"queryFingerprint"`
	FirstRunNanos    int64          `json:"firstRunNanos"`
	Samples          []Sample       `json:"samples,omitempty"`
	Latency          Percentiles    `json:"latency"`
	SQLStatements    MetricSummary  `json:"sqlStatements"`
	Rows             MetricSummary  `json:"rows"`
	OutputBytes      MetricSummary  `json:"outputBytes"`
	Plans            []PlanArtifact `json:"plans,omitempty"`
}

type RunConfiguration struct {
	ScenarioIDs  []string `json:"scenarioIds"`
	Warmups      int      `json:"warmups"`
	Samples      int      `json:"samples"`
	TimeoutNanos int64    `json:"timeoutNanos"`
	PoolSize     int      `json:"poolSize"`
	Concurrency  int      `json:"concurrency"`
}

type RunArtifact struct {
	SchemaVersion   int              `json:"schemaVersion"`
	CatalogVersion  string           `json:"catalogVersion"`
	Status          string           `json:"status"`
	StartedAt       time.Time        `json:"startedAt"`
	Environment     Environment      `json:"environment"`
	Configuration   RunConfiguration `json:"configuration"`
	Fixture         FixtureManifest  `json:"fixture"`
	Warmups         int              `json:"warmups"`
	MeasuredSamples int              `json:"measuredSamples"`
	Results         []ScenarioResult `json:"results"`
}

type Comparison struct {
	Compatible  bool            `json:"compatible"`
	Regressions []ScenarioDelta `json:"regressions,omitempty"`
	Deltas      []ScenarioDelta `json:"deltas"`
	Errors      []string        `json:"errors,omitempty"`
}

type ScenarioDelta struct {
	ScenarioID      string  `json:"scenarioId"`
	BaselineP50     int64   `json:"baselineP50Nanos"`
	CandidateP50    int64   `json:"candidateP50Nanos"`
	RelativePercent float64 `json:"relativePercent"`
	AbsoluteNanos   int64   `json:"absoluteNanos"`
	Regression      bool    `json:"regression"`
}
