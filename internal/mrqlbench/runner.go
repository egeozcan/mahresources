package mrqlbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/mrql"
	"mahresources/server/api_handlers"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type RunOptions struct {
	Scenarios     []string
	Warmups       int
	Samples       int
	Timeout       time.Duration
	Status        string
	Revision      string
	CanonicalHost string
	PoolSize      int
	Concurrency   int
}

type Runner struct {
	db        *gorm.DB
	app       *application_context.MahresourcesContext
	collector *Collector
	manifest  FixtureManifest
}

func NewRunner(db *gorm.DB, manifest FixtureManifest) (*Runner, error) {
	if err := ValidateFixture(db, manifest); err != nil {
		return nil, err
	}
	collector := NewCollector(db.Config.Logger)
	instrumented := db.Session(&gorm.Session{Logger: collector})
	app := application_context.NewMahresourcesContext(afero.NewMemMapFs(), instrumented, nil, &application_context.MahresourcesConfig{
		DbType: dialectConstant(manifest.Dialect), AltFileSystems: map[string]string{}, PluginsDisabled: true, MRQLDefaultLimit: 500,
		MRQLPageQueryBudget: 200, HashSimilarityThreshold: 10, HashAHashThreshold: 5,
	})
	if manifest.Features["fts"] {
		app.UseExistingFTS()
	}
	return &Runner{db: instrumented, app: app, collector: collector, manifest: manifest}, nil
}

// Measure executes one catalog scenario through its production path. It is the
// standard testing.B entry point; fixture setup and native-plan capture remain
// outside the measured call.
func (r *Runner) Measure(ctx context.Context, scenarioID, sampleID string) (Sample, error) {
	scenario, ok := ScenarioByID(scenarioID)
	if !ok {
		return Sample{}, fmt.Errorf("unknown scenario %q", scenarioID)
	}
	return r.runOnce(ctx, scenario, sampleID)
}

func (r *Runner) Run(ctx context.Context, options RunOptions) (RunArtifact, error) {
	if options.Warmups < 0 {
		return RunArtifact{}, errors.New("warmups cannot be negative")
	}
	if options.Samples <= 0 {
		options.Samples = 100
	}
	if options.Timeout <= 0 {
		options.Timeout = 30 * time.Second
	}
	if options.Status == "" {
		options.Status = "reference"
	}
	if options.Status != "raw" && options.Status != "reference" && options.Status != "canonical" {
		return RunArtifact{}, fmt.Errorf("unsupported artifact status %q", options.Status)
	}
	if options.Status == "canonical" {
		if options.Samples < 100 {
			return RunArtifact{}, errors.New("canonical artifacts require at least 100 measured samples")
		}
		if options.CanonicalHost == "" {
			return RunArtifact{}, errors.New("canonical artifacts require an explicit stable host ID")
		}
		if options.Revision == "" || options.Revision == "unknown" || strings.Contains(options.Revision, "dirty") {
			return RunArtifact{}, errors.New("canonical artifacts require a clean known Git revision")
		}
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 1
	}
	scenarios, err := selectScenarios(options.Scenarios, r.manifest.Features)
	if err != nil {
		return RunArtifact{}, err
	}
	version, err := databaseVersion(r.db)
	if err != nil {
		return RunArtifact{}, err
	}
	cpuModel, cpuCount, memoryBytes := hardwareInfo()
	scenarioIDs := make([]string, len(scenarios))
	for i := range scenarios {
		scenarioIDs[i] = scenarios[i].ID
	}
	artifact := RunArtifact{
		SchemaVersion: ArtifactSchemaVersion, CatalogVersion: ScenarioCatalogVersion, Status: options.Status,
		StartedAt: time.Now().UTC(), Fixture: r.manifest, Warmups: options.Warmups, MeasuredSamples: options.Samples,
		Environment: Environment{GitRevision: options.Revision, GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
			HostID: options.CanonicalHost, CPUModel: cpuModel, CPUCount: cpuCount, MemoryBytes: memoryBytes,
			Database: r.manifest.Dialect, DatabaseVersion: version, PoolSize: options.PoolSize, Concurrency: options.Concurrency},
		Configuration: RunConfiguration{ScenarioIDs: scenarioIDs, Warmups: options.Warmups, Samples: options.Samples, TimeoutNanos: options.Timeout.Nanoseconds(), PoolSize: options.PoolSize, Concurrency: options.Concurrency},
		Results:       make([]ScenarioResult, 0, len(scenarios)),
	}
	for _, scenario := range scenarios {
		result, err := r.runScenario(ctx, scenario, options)
		if err != nil {
			return RunArtifact{}, fmt.Errorf("scenario %s: %w", scenario.ID, err)
		}
		artifact.Results = append(artifact.Results, result)
	}
	return artifact, nil
}

func (r *Runner) runScenario(parent context.Context, scenario Scenario, options RunOptions) (ScenarioResult, error) {
	ctx, cancel := context.WithTimeout(parent, options.Timeout)
	defer cancel()
	first, err := r.runOnce(ctx, scenario, scenario.ID+":first")
	if err != nil {
		return ScenarioResult{}, err
	}
	for i := 0; i < options.Warmups; i++ {
		if _, err := r.runOnce(ctx, scenario, fmt.Sprintf("%s:warmup:%d", scenario.ID, i)); err != nil {
			return ScenarioResult{}, err
		}
	}
	samples := make([]Sample, options.Samples)
	measure := func(i int) error {
		sample, err := r.runOnce(ctx, scenario, fmt.Sprintf("%s:sample:%d", scenario.ID, i))
		if err != nil {
			return err
		}
		if sample.SQLStatements < scenario.MinimumSQLStatements || sample.SQLStatements > scenario.MaximumSQLStatements {
			return fmt.Errorf("SQL statements %d outside expected [%d,%d] (observations: %#v)", sample.SQLStatements, scenario.MinimumSQLStatements, scenario.MaximumSQLStatements, sample.Statements)
		}
		minimumRows, maximumRows := expectedRowBounds(scenario, r.manifest.Profile)
		if scenario.CheckRows && (sample.Rows < minimumRows || sample.Rows > maximumRows) {
			return fmt.Errorf("rows %d outside expected [%d,%d]", sample.Rows, minimumRows, maximumRows)
		}
		if scenario.MaximumOutputBytes > 0 && (sample.OutputBytes < scenario.MinimumOutputBytes || sample.OutputBytes > scenario.MaximumOutputBytes) {
			return fmt.Errorf("output bytes %d outside expected [%d,%d]", sample.OutputBytes, scenario.MinimumOutputBytes, scenario.MaximumOutputBytes)
		}
		samples[i] = sample
		return nil
	}
	if options.Concurrency == 1 {
		for i := range samples {
			if err := measure(i); err != nil {
				return ScenarioResult{}, err
			}
		}
	} else {
		jobs := make(chan int)
		workerCtx, stopWorkers := context.WithCancel(ctx)
		var wg sync.WaitGroup
		var once sync.Once
		var workerErr error
		workers := min(options.Concurrency, options.Samples)
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					if workerCtx.Err() != nil {
						return
					}
					if err := measure(i); err != nil {
						once.Do(func() { workerErr = err; stopWorkers() })
						return
					}
				}
			}()
		}
	dispatch:
		for i := range samples {
			select {
			case jobs <- i:
			case <-workerCtx.Done():
				break dispatch
			}
		}
		close(jobs)
		wg.Wait()
		stopWorkers()
		if workerErr != nil {
			return ScenarioResult{}, workerErr
		}
	}
	latencies := make([]int64, 0, options.Samples)
	sqlCounts := make([]int64, 0, options.Samples)
	rowCounts := make([]int64, 0, options.Samples)
	outputBytes := make([]int64, 0, options.Samples)
	for _, sample := range samples {
		latencies = append(latencies, sample.ElapsedNanos)
		sqlCounts = append(sqlCounts, int64(sample.SQLStatements))
		rowCounts = append(rowCounts, sample.Rows)
		outputBytes = append(outputBytes, sample.OutputBytes)
	}
	percentiles, err := CalculatePercentiles(latencies)
	if err != nil {
		return ScenarioResult{}, err
	}
	sqlSummary, err := SummarizeMetric(sqlCounts)
	if err != nil {
		return ScenarioResult{}, err
	}
	rowSummary, err := SummarizeMetric(rowCounts)
	if err != nil {
		return ScenarioResult{}, err
	}
	outputSummary, err := SummarizeMetric(outputBytes)
	if err != nil {
		return ScenarioResult{}, err
	}
	fingerprint, plans, err := r.explainScenario(ctx, scenario)
	if err != nil {
		return ScenarioResult{}, err
	}
	return ScenarioResult{
		ScenarioID: scenario.ID, QueryFingerprint: fingerprint, FirstRunNanos: first.ElapsedNanos,
		Samples: samples, Latency: percentiles, SQLStatements: sqlSummary, Rows: rowSummary,
		OutputBytes: outputSummary, Plans: plans,
	}, nil
}

func (r *Runner) runOnce(ctx context.Context, scenario Scenario, sampleID string) (Sample, error) {
	r.collector.Reset(sampleID)
	sampleCtx := WithSample(ctx, sampleID)
	started := time.Now()
	var output any
	phaseNanos := map[string]int64{}
	cacheHits, cacheMisses := 0, 0

	switch scenario.Mode {
	case "direct", "grouped":
		phase := time.Now()
		parsed, err := mrql.Parse(scenario.Query)
		if err != nil {
			return Sample{}, err
		}
		if err := mrql.BindParams(parsed, nil); err != nil {
			return Sample{}, err
		}
		if err := mrql.Validate(parsed); err != nil {
			return Sample{}, err
		}
		if scenario.Mode == "grouped" {
			parsed.EntityType = mrql.ExtractEntityType(parsed)
		}
		phaseNanos["parse_bind_validate"] = time.Since(phase).Nanoseconds()

		phase = time.Now()
		if scenario.Mode == "grouped" {
			output, err = r.app.ExecuteMRQLGrouped(sampleCtx, parsed)
		} else {
			limit := 0
			if scenario.Page > 0 {
				limit = 50
			}
			output, err = r.app.ExecuteMRQLParsed(sampleCtx, parsed, limit, scenario.Page)
		}
		if err != nil {
			return Sample{}, err
		}
		phaseNanos["translate_execute_transfer_decode"] = time.Since(phase).Nanoseconds()

	case "http":
		body, _ := json.Marshal(map[string]any{"query": scenario.Query})
		req := httptest.NewRequest(http.MethodPost, "/v1/mrql", bytes.NewReader(body)).WithContext(sampleCtx)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		api_handlers.GetExecuteMRQLHandler(r.app)(recorder, req)
		if recorder.Code != http.StatusOK {
			return Sample{}, fmt.Errorf("HTTP %d: %s", recorder.Code, recorder.Body.String())
		}
		output = recorder.Body.Bytes()
		phaseNanos["http_end_to_end"] = time.Since(started).Nanoseconds()

	case "shortcode":
		renderCtx := application_context.WithMRQLRenderDataCache(sampleCtx)
		renderCtx = shortcodes.WithQueryBudget(renderCtx, 200)
		executor := template_filters.BuildQueryExecutor(r.app)
		attrs := map[string]string{"query": scenario.Query}
		if scenario.RenderMode != "custom" && scenario.RenderMode != "nested" {
			attrs["format"] = scenario.RenderMode
		}
		html := shortcodes.RenderMRQLShortcode(renderCtx, shortcodes.Shortcode{Name: "mrql", Attrs: attrs}, shortcodes.MetaShortcodeContext{}, nil, executor, 0)
		if strings.Contains(html, "mrql-error") {
			return Sample{}, fmt.Errorf("shortcode rendered an error: %s", html)
		}
		output = html
		if budget := shortcodes.QueryBudgetFrom(renderCtx); budget != nil {
			stats := budget.Stats()
			cacheHits, cacheMisses = stats.CacheHits, stats.CacheMisses
		}
		phaseNanos["template_render"] = time.Since(started).Nanoseconds()

	default:
		return Sample{}, fmt.Errorf("unsupported scenario mode %q", scenario.Mode)
	}

	encodeStarted := time.Now()
	encoded, err := encodeOutput(output)
	if err != nil {
		return Sample{}, err
	}
	phaseNanos["encode"] = time.Since(encodeStarted).Nanoseconds()
	observations := r.collector.Snapshot(sampleID)
	rows := int64(0)
	for _, observation := range observations {
		if observation.Rows > 0 {
			rows += observation.Rows
		}
	}
	return Sample{
		ElapsedNanos: time.Since(started).Nanoseconds(), PhaseNanos: phaseNanos,
		SQLStatements: len(observations), Rows: rows, OutputBytes: int64(len(encoded)), Statements: observations,
		CacheHits: cacheHits, CacheMisses: cacheMisses,
	}, nil
}

func encodeOutput(output any) ([]byte, error) {
	switch value := output.(type) {
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	default:
		return json.Marshal(value)
	}
}

func (r *Runner) explainScenario(ctx context.Context, scenario Scenario) (string, []PlanArtifact, error) {
	parsed, err := mrql.Parse(scenario.Query)
	if err != nil {
		return "", nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return "", nil, err
	}
	if scenario.Page > 0 {
		parsed.Limit = 50
		parsed.Offset = (scenario.Page - 1) * 50
	}
	explained, err := r.app.ExplainMRQLWithOptions(ctx, parsed, application_context.MRQLExplainOptions{NativePlan: true})
	if err != nil {
		return "", nil, err
	}
	plans := make([]PlanArtifact, 0, len(explained.Statements))
	for _, statement := range explained.Statements {
		if statement.NativePlan == nil {
			continue
		}
		var plan any
		if err := json.Unmarshal(statement.NativePlan.Plan, &plan); err != nil {
			return "", nil, err
		}
		plans = append(plans, PlanArtifact{SQLFingerprint: sqlFingerprint(statement.Interpolated), Dialect: statement.NativePlan.Dialect, Format: statement.NativePlan.Format, Plan: plan, Signature: planSignature(plan)})
	}
	return explained.QueryFingerprint, plans, nil
}

func planSignature(plan any) []string {
	set := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, key := range []string{"Node Type", "Join Type", "Relation Name", "Index Name", "detail"} {
				if value, ok := typed[key].(string); ok {
					set[key+"="+value] = true
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(plan)
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func selectScenarios(ids []string, features map[string]bool) ([]Scenario, error) {
	requested := map[string]bool{}
	for _, id := range ids {
		requested[id] = true
	}
	filtering := len(requested) > 0
	var selected []Scenario
	for _, scenario := range Scenarios() {
		if filtering && !requested[scenario.ID] {
			continue
		}
		supported := true
		for _, feature := range scenario.RequiredFeatures {
			if !features[feature] {
				supported = false
			}
		}
		if supported {
			selected = append(selected, scenario)
		}
		delete(requested, scenario.ID)
	}
	if len(requested) > 0 {
		unknown := make([]string, 0, len(requested))
		for id := range requested {
			unknown = append(unknown, id)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown scenarios: %s", strings.Join(unknown, ", "))
	}
	return selected, nil
}

func expectedRowBounds(scenario Scenario, profile Profile) (int64, int64) {
	if profile.ID == "tiny" && scenario.Family == "pagination" && scenario.Page > 1 {
		return 0, 50
	}
	return scenario.MinimumRows, scenario.MaximumRows
}

func hardwareInfo() (model string, count int, memoryBytes uint64) {
	count = runtime.NumCPU()
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		model = info[0].ModelName
	}
	if memory, err := mem.VirtualMemory(); err == nil {
		memoryBytes = memory.Total
	}
	return model, count, memoryBytes
}

func dialectConstant(dialect string) string {
	if normalizeDialect(dialect) == "postgres" {
		return constants.DbTypePosgres
	}
	return constants.DbTypeSqlite
}
