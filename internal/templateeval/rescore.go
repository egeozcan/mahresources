package templateeval

import "strings"

// RescoreUnknownShortcodes re-reads each stored draft with the current
// unknown-shortcode scan and recomputes what depends on it: the count, the
// names, Pass and the summary. It calls no provider. Everything else in a
// result is kept as recorded: the per-query runtime probes are not stored, so
// lint, MRQL and preview checks cannot be re-derived from a report.
func RescoreUnknownShortcodes(report RunReport, env Env) (RunReport, error) {
	cases := map[string]Case{}
	for _, c := range Corpus() {
		cases[c.ID] = c
	}
	report.Results = append([]CaseResult(nil), report.Results...)
	for i := range report.Results {
		r := &report.Results[i]
		c, ok := cases[r.Case]
		if !ok || r.Draft == nil || r.Kind == KindMetaSchema || r.Checks.HTTPStatus != 200 {
			continue
		}
		unknown := map[string]bool{}
		for slot, text := range draftTexts(c, *r.Draft) {
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
	return report, nil
}
