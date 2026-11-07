//go:build templateeval && json1 && fts5

package api_tests

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/internal/templateeval"
)

// The quality eval for the template generator. It calls the real DeepSeek
// provider and costs money; see benchmarks/template-generation/README.md.
// One run produces both modes and their comparison: classic first, then
// harness, through the same provider configuration.
//
//	DEEPSEEK_API_KEY=... go test --tags 'json1 fts5 templateeval' ./server/api_tests \
//	  -run 'TestTemplateGenerationEval$' -count=1 -timeout 60m -v
//
// TEMPLATE_EVAL_OUT=X.json writes X.classic.json, X.harness.json,
// X.comparison.txt and X.comparison.json (each report with its .aggregate.json).
func TestTemplateGenerationEval(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY is not set: the template eval calls the real provider and was skipped; see benchmarks/template-generation/README.md.")
	}
	repeats := 3
	if raw := os.Getenv("TEMPLATE_EVAL_REPEATS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("invalid TEMPLATE_EVAL_REPEATS=%q", raw)
		}
		repeats = n
	}
	var ids []string
	if raw := strings.TrimSpace(os.Getenv("TEMPLATE_EVAL_CASES")); raw != "" {
		ids = strings.Split(raw, ",")
	}
	cases, err := templateeval.FilterCases(templateeval.Corpus(), ids)
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := templateeval.ModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	// Relative to the module root, not to server/api_tests where this runs.
	out := templateeval.ResolveOutPath(os.Getenv("TEMPLATE_EVAL_OUT"), repoRoot, time.Now())

	// The provider configuration main.go reads (DEEPSEEK_MODEL, DEEPSEEK_TIMEOUT
	// and their defaults).
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = application_context.DefaultDeepSeekMRQLGenerationModel
	}
	timeout := application_context.DefaultDeepSeekMRQLGenerationTimeout
	if raw := os.Getenv("DEEPSEEK_TIMEOUT"); raw != "" {
		if timeout, err = time.ParseDuration(raw); err != nil {
			t.Fatalf("invalid DEEPSEEK_TIMEOUT=%q: %v", raw, err)
		}
	}

	result := runTemplateEvalBothModes(t, evalRunConfig{
		Provider: "deepseek", APIKey: apiKey, Model: model, Timeout: timeout,
		Lookup: os.Getenv, Cases: cases, Repeats: repeats, Commit: evalCommit(repoRoot),
	})
	paths, err := writeEvalTwoModeReports(out, result, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("classic: %s\nharness: %s\ncomparison: %s\n%s\n%s\n%s", paths.Classic, paths.Harness, paths.Comparison,
		templateeval.FormatSummary(result.Classic.Summary), templateeval.FormatSummary(result.Harness.Summary), templateeval.FormatComparison(result.Comparison))
}

// TestTemplateGenerationEvalReplay scores saved drafts against fresh copies of
// the corpus fixtures. It makes no generation/provider call, and recreates the
// runtime query probes older reports did not store separately from lint errors.
func TestTemplateGenerationEvalReplay(t *testing.T) {
	path := os.Getenv("TEMPLATE_EVAL_REPLAY")
	if path == "" {
		t.Skip("set TEMPLATE_EVAL_REPLAY to replay a full saved report")
	}
	root, err := templateeval.ModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	path = templateeval.ResolveOutPath(path, root, time.Now())
	report, err := templateeval.ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.CorpusVersion != templateeval.CorpusVersion || report.FixtureChecksum != templateeval.FixtureChecksum() {
		t.Fatal("the saved report uses a different corpus or fixture")
	}
	cases := map[string]templateeval.Case{}
	for _, c := range templateeval.Corpus() {
		cases[c.ID] = c
	}
	for _, r := range report.Results {
		if _, ok := cases[r.Case]; !ok || r.Draft == nil {
			t.Fatalf("%s #%d lacks a corpus case or raw draft", r.Case, r.Repeat)
		}
	}
	tc := SetupTestEnv(t)
	seeded := seedTemplateEvalFixtures(t, tc)
	env := evalScoringEnv(tc)
	for i := range report.Results {
		r := &report.Results[i]
		c := cases[r.Case]
		fixture, ok := templateeval.FixtureForCase(c)
		if !ok {
			t.Fatalf("case %s has no fixture", c.ID)
		}
		ids, ok := seeded[fixture.Key]
		if !ok || ids.category == 0 || ids.sample == 0 {
			t.Fatalf("case %s lacks seeded category or sample IDs", c.ID)
		}
		r.Previews = evalPreviews(tc, c, *r.Draft, ids.category, ids.sample)
		r.Probes = evalQueryProbes(tc, c, *r.Draft, ids.category, ids.sample)
		checks, details := templateeval.Score(c, *r.Draft, r.Calls, r.Previews, r.Probes, env)
		r.Checks, r.Details, r.Pass = checks, &details, checks.Pass()
		r.FillRunMetrics()
	}
	report.Summary = templateeval.Summarize(report.Results)
	report.ScorerVersion = templateeval.ScorerVersion
	report.ScoringEnvironment = env.Fingerprint()
	report.Rescored = []string{"previews", "queryProbes", templateeval.RescoredScore}
	out := os.Getenv("TEMPLATE_EVAL_REPLAY_OUT")
	if out == "" {
		out = strings.TrimSuffix(path, ".json") + ".replayed.json"
	}
	out = templateeval.ResolveOutPath(out, root, time.Now())
	if err := templateeval.WriteJSONAtomic(out, report); err != nil {
		t.Fatal(err)
	}
	aggregate := strings.TrimSuffix(out, ".json") + ".aggregate.json"
	if err := templateeval.WriteJSONAtomic(aggregate, templateeval.AggregateOnly(report)); err != nil {
		t.Fatal(err)
	}
	t.Logf("replayed report: %s\naggregate: %s\n%s", out, aggregate, templateeval.FormatSummary(report.Summary))
}

// TestTemplateGenerationEvalCompare compares two saved reports. It makes no
// network calls.
func TestTemplateGenerationEvalCompare(t *testing.T) {
	basePath, candidatePath := os.Getenv("TEMPLATE_EVAL_BASELINE"), os.Getenv("TEMPLATE_EVAL_CANDIDATE")
	if basePath == "" && candidatePath == "" {
		t.Skip("set TEMPLATE_EVAL_BASELINE and TEMPLATE_EVAL_CANDIDATE to compare two reports")
	}
	if basePath == "" || candidatePath == "" {
		t.Fatal("TEMPLATE_EVAL_BASELINE and TEMPLATE_EVAL_CANDIDATE must both be set")
	}
	root, err := templateeval.ModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	basePath = templateeval.ResolveOutPath(basePath, root, time.Now())
	candidatePath = templateeval.ResolveOutPath(candidatePath, root, time.Now())
	base, err := templateeval.ReadReport(basePath)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := templateeval.ReadReport(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	cmp := templateeval.Compare(base, candidate)
	t.Logf("\n%s", templateeval.FormatComparison(cmp))
	if !cmp.Compatible {
		t.Fatalf("the reports cannot be compared: %s", strings.Join(cmp.Errors, "; "))
	}
	for _, r := range cmp.Regressions {
		t.Errorf("regression: %s", r)
	}
}
