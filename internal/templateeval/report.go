package templateeval

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// ReportSchemaVersion is the shape of RunReport. Bump it on any change a
// reader of an older report would misread.
const ReportSchemaVersion = 3

// RegressionThresholdPP is the pass-rate drop, in percentage points, beyond
// which Compare reports a kind as regressed.
const RegressionThresholdPP = 10.0

// RunReport is one eval run.
type RunReport struct {
	SchemaVersion   int          `json:"schemaVersion"`
	CorpusVersion   string       `json:"corpusVersion"`
	FixtureChecksum string       `json:"fixtureChecksum"`
	Commit          string       `json:"commit,omitempty"`
	StartedAt       string       `json:"startedAt,omitempty"`
	FinishedAt      string       `json:"finishedAt,omitempty"`
	Model           string       `json:"model,omitempty"`
	Timeout         string       `json:"timeout,omitempty"`
	Repeats         int          `json:"repeats"`
	Results         []CaseResult `json:"results"`
	Summary         Summary      `json:"summary"`
}

// CaseResult is one run of one case.
type CaseResult struct {
	Case       string `json:"case"`
	Kind       Kind   `json:"kind"`
	Carrier    string `json:"carrier"`
	Repeat     int    `json:"repeat"`
	DurationMS int64  `json:"durationMs"`
	// DisclosureBytes is the size of the disclosure the dry run returned: the
	// text of every message section the request would send.
	DisclosureBytes int                `json:"disclosureBytes,omitempty"`
	Pass            bool               `json:"pass"`
	Checks          Checks             `json:"checks"`
	Calls           []Call             `json:"calls,omitempty"`
	Details         *Details           `json:"details,omitempty"`
	Draft           *Draft             `json:"draft,omitempty"`
	Previews        map[string]Preview `json:"previews,omitempty"`
}

// Summary holds one GroupSummary for the whole run ("all"), per kind ("pair")
// and per kind and carrier ("pair/group").
type Summary struct {
	Groups []GroupSummary `json:"groups"`
}

// Group returns the summary with that key, or nil.
func (s Summary) Group(key string) *GroupSummary {
	for i := range s.Groups {
		if s.Groups[i].Key == key {
			return &s.Groups[i]
		}
	}
	return nil
}

// GroupSummary aggregates runs. Every rate is the share of runs with at least
// one occurrence.
type GroupSummary struct {
	Key                  string  `json:"key"`
	Runs                 int     `json:"runs"`
	Passed               int     `json:"passed"`
	PassRate             float64 `json:"passRate"`
	ProviderErrorRate    float64 `json:"providerErrorRate"`
	TruncationRate       float64 `json:"truncationRate"`
	MalformedRate        float64 `json:"malformedRate"`
	WrongShapeRate       float64 `json:"wrongShapeRate"`
	LintErrorRate        float64 `json:"lintErrorRate"`
	UnknownShortcodeRate float64 `json:"unknownShortcodeRate"`
	MRQLErrorRate        float64 `json:"mrqlErrorRate"`
	IncompletePairRate   float64 `json:"incompletePairRate"`
	RenderEmptyRate      float64 `json:"renderEmptyRate"`
	PreviewErrorRate     float64 `json:"previewErrorRate"`
	ExecutableMarkupRate float64 `json:"executableMarkupRate"`
	Calls                int     `json:"calls"`
	Usage                Usage   `json:"usage"`
	// CacheHitRatio is CacheHit / (CacheHit + CacheMiss) over every call.
	CacheHitRatio float64 `json:"cacheHitRatio"`
	MeanLatencyMS int64   `json:"meanLatencyMs"`
	MaxLatencyMS  int64   `json:"maxLatencyMs"`
}

// Summarize groups results by kind and by kind and carrier.
func Summarize(results []CaseResult) Summary {
	type acc struct {
		g                                                                                     GroupSummary
		provider, trunc, malformed, shape, lint, unknown, mrql, pairs, empty, preview, markup int
		latency                                                                               int64
	}
	groups := map[string]*acc{}
	add := func(key string, r CaseResult) {
		a := groups[key]
		if a == nil {
			a = &acc{g: GroupSummary{Key: key}}
			groups[key] = a
		}
		c := r.Checks
		a.g.Runs++
		count := func(n *int, hit bool) {
			if hit {
				*n++
			}
		}
		count(&a.g.Passed, r.Pass)
		count(&a.provider, c.HTTPStatus != 200)
		count(&a.trunc, c.Truncated)
		count(&a.malformed, c.Malformed)
		count(&a.shape, c.WrongShape)
		count(&a.lint, c.LintErrors > 0)
		count(&a.unknown, c.UnknownShortcodeCount > 0)
		count(&a.mrql, c.MRQLErrors > 0)
		count(&a.pairs, c.IncompletePairs > 0)
		count(&a.empty, len(c.RenderEmptySlots) > 0)
		count(&a.preview, c.PreviewErrors > 0)
		count(&a.markup, len(c.ExecutableMarkup) > 0 || len(c.RenderedExecutableMarkup) > 0)
		for _, call := range r.Calls {
			a.g.Calls++
			a.g.Usage = a.g.Usage.add(call.Usage)
			a.latency += call.LatencyMS
			if call.LatencyMS > a.g.MaxLatencyMS {
				a.g.MaxLatencyMS = call.LatencyMS
			}
		}
	}
	for _, r := range results {
		add("all", r)
		add(string(r.Kind), r)
		add(string(r.Kind)+"/"+r.Carrier, r)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if (keys[i] == "all") != (keys[j] == "all") {
			return keys[i] == "all"
		}
		return keys[i] < keys[j]
	})
	summary := Summary{Groups: make([]GroupSummary, 0, len(keys))}
	for _, k := range keys {
		a := groups[k]
		rate := func(n int) float64 { return float64(n) / float64(a.g.Runs) }
		a.g.PassRate = rate(a.g.Passed)
		a.g.ProviderErrorRate, a.g.TruncationRate, a.g.LintErrorRate = rate(a.provider), rate(a.trunc), rate(a.lint)
		a.g.MalformedRate, a.g.WrongShapeRate = rate(a.malformed), rate(a.shape)
		a.g.UnknownShortcodeRate, a.g.MRQLErrorRate, a.g.IncompletePairRate = rate(a.unknown), rate(a.mrql), rate(a.pairs)
		a.g.RenderEmptyRate, a.g.PreviewErrorRate, a.g.ExecutableMarkupRate = rate(a.empty), rate(a.preview), rate(a.markup)
		if cached := a.g.Usage.CacheHit + a.g.Usage.CacheMiss; cached > 0 {
			a.g.CacheHitRatio = float64(a.g.Usage.CacheHit) / float64(cached)
		}
		if a.g.Calls > 0 {
			a.g.MeanLatencyMS = a.latency / int64(a.g.Calls)
		}
		summary.Groups = append(summary.Groups, a.g)
	}
	return summary
}

// FormatSummary renders a summary as a text table.
func FormatSummary(s Summary) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "group\truns\tpass\tprovider err\ttruncated\tmalformed\twrong shape\tlint err\tunknown sc\tmrql err\trender empty\tpreview err\texec markup\tcalls\tprompt tok\tcache hit\tmean ms")
	for _, g := range s.Groups {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\n",
			g.Key, g.Runs, pct(g.PassRate), pct(g.ProviderErrorRate), pct(g.TruncationRate), pct(g.MalformedRate), pct(g.WrongShapeRate), pct(g.LintErrorRate),
			pct(g.UnknownShortcodeRate), pct(g.MRQLErrorRate), pct(g.RenderEmptyRate), pct(g.PreviewErrorRate),
			pct(g.ExecutableMarkupRate), g.Calls, g.Usage.Prompt, pct(g.CacheHitRatio), g.MeanLatencyMS)
	}
	_ = w.Flush()
	return b.String()
}

func pct(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

// Comparison is the result of comparing a candidate run against a baseline.
type Comparison struct {
	Compatible  bool         `json:"compatible"`
	Errors      []string     `json:"errors,omitempty"`
	Deltas      []GroupDelta `json:"deltas,omitempty"`
	Regressions []string     `json:"regressions,omitempty"`
}

// GroupDelta is one summary group in both runs. Deltas are in percentage points.
type GroupDelta struct {
	Key                  string  `json:"key"`
	BaseRuns             int     `json:"baseRuns"`
	CandidateRuns        int     `json:"candidateRuns"`
	BasePassRate         float64 `json:"basePassRate"`
	CandidatePassRate    float64 `json:"candidatePassRate"`
	PassRatePP           float64 `json:"passRatePp"`
	TruncationPP         float64 `json:"truncationPp"`
	UnknownShortcodePP   float64 `json:"unknownShortcodePp"`
	MRQLErrorPP          float64 `json:"mrqlErrorPp"`
	CandidateCacheHitPct float64 `json:"candidateCacheHitPct"`
}

// Compare refuses reports with a different schema, corpus or fixture, a
// different number of results for any case, or a stored summary that does not
// match its own results. Otherwise it lists every group's deltas, from
// summaries recomputed from the results. A kind whose pass rate drops by more
// than RegressionThresholdPP is a regression.
func Compare(base, candidate RunReport) Comparison {
	cmp := Comparison{Compatible: true}
	fail := func(format string, args ...any) {
		cmp.Compatible = false
		cmp.Errors = append(cmp.Errors, fmt.Sprintf(format, args...))
	}
	if base.SchemaVersion != candidate.SchemaVersion {
		fail("report schema versions differ: %d vs %d", base.SchemaVersion, candidate.SchemaVersion)
	}
	if base.CorpusVersion != candidate.CorpusVersion {
		fail("corpus versions differ: %s vs %s", base.CorpusVersion, candidate.CorpusVersion)
	}
	if base.FixtureChecksum != candidate.FixtureChecksum {
		fail("fixture checksums differ: %s vs %s", base.FixtureChecksum, candidate.FixtureChecksum)
	}
	// Equal counts per case, not just equal case sets: three repeats of a
	// passing case against one of a failing case would read as a gain.
	if b, c := caseCounts(base), caseCounts(candidate); b != c {
		fail("the runs differ in cases or repeats: [%s] vs [%s]", b, c)
	}
	for _, r := range []struct {
		name   string
		report RunReport
	}{{"baseline", base}, {"candidate", candidate}} {
		for _, res := range r.report.Results {
			if res.Pass != res.Checks.Pass() {
				fail("the %s's stored verdict for %s #%d disagrees with its checks", r.name, res.Case, res.Repeat)
			}
		}
	}
	baseSummary, candidateSummary := Summarize(base.Results), Summarize(candidate.Results)
	if !reflect.DeepEqual(base.Summary, baseSummary) {
		fail("the baseline's stored summary does not match its results")
	}
	if !reflect.DeepEqual(candidate.Summary, candidateSummary) {
		fail("the candidate's stored summary does not match its results")
	}
	if !cmp.Compatible {
		return cmp
	}
	keys := map[string]bool{}
	for _, g := range append(append([]GroupSummary(nil), baseSummary.Groups...), candidateSummary.Groups...) {
		keys[g.Key] = true
	}
	for _, key := range sortedKeys(keys) {
		b, c := baseSummary.Group(key), candidateSummary.Group(key)
		if b == nil {
			b = &GroupSummary{Key: key}
		}
		if c == nil {
			c = &GroupSummary{Key: key}
		}
		d := GroupDelta{
			Key: key, BaseRuns: b.Runs, CandidateRuns: c.Runs,
			BasePassRate: b.PassRate, CandidatePassRate: c.PassRate,
			PassRatePP:           pp(c.PassRate - b.PassRate),
			TruncationPP:         pp(c.TruncationRate - b.TruncationRate),
			UnknownShortcodePP:   pp(c.UnknownShortcodeRate - b.UnknownShortcodeRate),
			MRQLErrorPP:          pp(c.MRQLErrorRate - b.MRQLErrorRate),
			CandidateCacheHitPct: pp(c.CacheHitRatio),
		}
		cmp.Deltas = append(cmp.Deltas, d)
		if key != "all" && !strings.Contains(key, "/") && b.Runs > 0 && c.Runs > 0 && d.PassRatePP < -RegressionThresholdPP {
			cmp.Regressions = append(cmp.Regressions, fmt.Sprintf("%s pass rate fell %.1f points (%s to %s)", key, -d.PassRatePP, pct(b.PassRate), pct(c.PassRate)))
		}
	}
	return cmp
}

// pp converts a rate difference to percentage points, rounded to 0.1 so that
// float noise cannot push an exact 10-point drop over the threshold.
func pp(f float64) float64 { return math.Round(f*1000) / 10 }

// caseCounts lists each case with its number of results, sorted.
func caseCounts(r RunReport) string {
	counts := map[string]int{}
	for _, res := range r.Results {
		counts[res.Case]++
	}
	parts := make([]string, 0, len(counts))
	for id, n := range counts {
		parts = append(parts, id+"×"+strconv.Itoa(n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// FormatComparison renders a comparison as text.
func FormatComparison(c Comparison) string {
	var b strings.Builder
	if !c.Compatible {
		b.WriteString("incompatible reports:\n")
		for _, e := range c.Errors {
			b.WriteString("  " + e + "\n")
		}
		return b.String()
	}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "group\truns\tbase pass\tcandidate pass\tΔ pass pp\tΔ truncated pp\tΔ unknown sc pp\tΔ mrql err pp\tcandidate cache hit")
	for _, d := range c.Deltas {
		fmt.Fprintf(w, "%s\t%d/%d\t%s\t%s\t%+.1f\t%+.1f\t%+.1f\t%+.1f\t%.0f%%\n", d.Key, d.BaseRuns, d.CandidateRuns,
			pct(d.BasePassRate), pct(d.CandidatePassRate), d.PassRatePP, d.TruncationPP, d.UnknownShortcodePP, d.MRQLErrorPP, d.CandidateCacheHitPct)
	}
	_ = w.Flush()
	for _, r := range c.Regressions {
		b.WriteString("REGRESSION: " + r + "\n")
	}
	return b.String()
}

// AggregateOnly drops every piece of free text (details, drafts, previews and
// raw model replies), keeping checks, usage and the summary, which hold only
// numbers, booleans, IDs, fixed enums and the harness's own config. Use it
// for committed baselines. TestAggregateKeepsNoFreeText walks the result.
func AggregateOnly(report RunReport) RunReport {
	report.Results = append([]CaseResult(nil), report.Results...)
	for i := range report.Results {
		r := &report.Results[i]
		r.Details, r.Draft, r.Previews = nil, nil, nil
		r.Calls = append([]Call(nil), r.Calls...)
		for j := range r.Calls {
			r.Calls[j].Content = ""
		}
	}
	return report
}

// ReadReport reads a report and refuses an unknown schema version.
func ReadReport(path string) (RunReport, error) {
	var report RunReport
	file, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, fmt.Errorf("decode %s: %w", path, err)
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return report, fmt.Errorf("report %q has schema version %d, want %d", path, report.SchemaVersion, ReportSchemaVersion)
	}
	return report, nil
}

// WriteJSONAtomic is internal/mrqlbench/artifact.go's WriteJSONAtomic: write a
// temporary file beside the target, sync it, then rename it into place.
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
	return os.Rename(tempName, path)
}
