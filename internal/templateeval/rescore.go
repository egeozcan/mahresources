package templateeval

import (
	"fmt"
	"strings"
)

// RescoreUnknownShortcodes re-reads each stored draft with the current
// unknown-shortcode scan and recomputes what depends on it: the count, the
// names, Pass and the summary. It calls no provider. Everything else in a
// result is kept as recorded: the per-query runtime probes are not stored, so
// lint, MRQL and preview checks cannot be re-derived from a report. The result
// is stamped ScorerVersion with Rescored naming the one re-derived check, and
// Compare refuses it against a report scored any other way.
//
// It refuses a report in which any scorable result lacks its draft, since a
// partial re-score would mix two scorers in one summary.
func RescoreUnknownShortcodes(report RunReport, env Env) (RunReport, error) {
	cases := map[string]Case{}
	for _, c := range Corpus() {
		cases[c.ID] = c
	}
	for _, r := range report.Results {
		if r.Kind == KindMetaSchema || r.Checks.HTTPStatus != 200 {
			continue
		}
		if _, ok := cases[r.Case]; !ok {
			return report, fmt.Errorf("cannot re-score %s #%d: case is not in the corpus", r.Case, r.Repeat)
		}
		if r.Draft == nil {
			return report, fmt.Errorf("cannot re-score %s #%d: the report holds no draft for it", r.Case, r.Repeat)
		}
	}
	report.Results = append([]CaseResult(nil), report.Results...)
	for i := range report.Results {
		r := &report.Results[i]
		if r.Kind == KindMetaSchema || r.Checks.HTTPStatus != 200 {
			continue
		}
		unknown := map[string]bool{}
		for slot, text := range draftTexts(cases[r.Case], *r.Draft) {
			for _, name := range unknownShortcodeNames(text, env.Known, strings.HasSuffix(slot, "CSS")) {
				unknown[name] = true
			}
		}
		details := Details{}
		if r.Details != nil {
			details = *r.Details
		}
		details.UnknownShortcodes = sortedKeys(unknown)
		r.Details = &details
		r.Checks.UnknownShortcodeCount = len(details.UnknownShortcodes)
		r.Pass = r.Checks.Pass()
	}
	report.Summary = Summarize(report.Results)
	report.ScorerVersion = ScorerVersion
	report.Rescored = []string{RescoredUnknownShortcodes}
	return report, nil
}
