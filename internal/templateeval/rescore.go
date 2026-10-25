package templateeval

import (
	"fmt"
	"sort"
)

// RescoreReport runs the current scorer over stored drafts, replies, previews
// and query probes. It calls no provider or renderer. Missing query probes
// cannot be recovered from the old merged lint/runtime error details: use the
// eval replay command to render a legacy report against the seeded fixtures.
// Every result must have its raw draft, so aggregate or partial reports cannot
// silently mix old and new checks.
func RescoreReport(report RunReport, env Env) (RunReport, error) {
	cases := map[string]Case{}
	for _, c := range Corpus() {
		cases[c.ID] = c
	}
	if report.CorpusVersion != CorpusVersion || report.FixtureChecksum != FixtureChecksum() {
		return report, fmt.Errorf("cannot re-score a different corpus or fixture")
	}
	if report.ScoringEnvironment == "" || report.ScoringEnvironment != env.Fingerprint() {
		return report, fmt.Errorf("cannot re-score stored runtime evidence from a missing or different scoring environment; use TestTemplateGenerationEvalReplay")
	}
	for _, r := range report.Results {
		if _, ok := cases[r.Case]; !ok {
			return report, fmt.Errorf("cannot re-score %s #%d: case is not in the corpus", r.Case, r.Repeat)
		}
		if r.Draft == nil {
			return report, fmt.Errorf("cannot re-score %s #%d: the report holds no draft for it", r.Case, r.Repeat)
		}
		if r.Draft.Status == 200 {
			type probeKey struct{ slot, query, snippet string }
			missing := map[probeKey]int{}
			for slot, text := range draftTexts(cases[r.Case], *r.Draft) {
				for _, probe := range QueryProbes(slot, text) {
					missing[probeKey{probe.Slot, probe.Query, probe.Snippet}]++
				}
			}
			for _, probe := range r.Probes {
				missing[probeKey{probe.Slot, probe.Query, probe.Snippet}]--
			}
			for _, count := range missing {
				if count != 0 {
					return report, fmt.Errorf("cannot re-score %s #%d: stored query probes do not match the draft; use TestTemplateGenerationEvalReplay", r.Case, r.Repeat)
				}
			}
		}
	}
	report.Results = append([]CaseResult(nil), report.Results...)
	for i := range report.Results {
		r := &report.Results[i]

		checks, details := Score(cases[r.Case], *r.Draft, r.Calls, r.Previews, r.Probes, env)
		r.Checks, r.Details, r.Pass = checks, &details, checks.Pass()
	}
	report.Summary = Summarize(report.Results)
	report.ScorerVersion = ScorerVersion
	report.Rescored = append([]string(nil), report.Rescored...)
	if !containsString(report.Rescored, RescoredScore) {
		report.Rescored = append(report.Rescored, RescoredScore)
	}
	sort.Strings(report.Rescored)
	report.ScoringEnvironment = env.Fingerprint()
	return report, nil
}
