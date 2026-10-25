package templateeval

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/shortcodes"
)

// TestRescoreReport re-scores the report named by TEMPLATE_EVAL_RESCORE with
// the current scorer and logs the corrected summary. It skips without it.
func TestRescoreReport(t *testing.T) {
	path := os.Getenv("TEMPLATE_EVAL_RESCORE")
	if path == "" {
		t.Skip("TEMPLATE_EVAL_RESCORE is not set")
	}
	root, err := ModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	report, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	before := report.Summary
	got, err := RescoreReport(report, Env{Known: shortcodes.KnownFromBuiltins(), MRQLDialect: "sqlite", MRQLFTS: true, ValidateMRQL: func(q string, opts shortcodes.MRQLCheckOptions) error {
		return application_context.CheckTemplateMRQL(q, opts, application_context.MRQLStaticEnv{FTS: true})
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recorded:\n%s\nre-scored:\n%s", FormatSummary(before), FormatSummary(got.Summary))
	if out := os.Getenv("TEMPLATE_EVAL_RESCORE_OUT"); out != "" {
		if !filepath.IsAbs(out) {
			out = filepath.Join(root, out)
		}
		if err := WriteJSONAtomic(out, got); err != nil {
			t.Fatal(err)
		}
		aggregate := strings.TrimSuffix(out, filepath.Ext(out)) + ".aggregate.json"
		if err := WriteJSONAtomic(aggregate, AggregateOnly(got)); err != nil {
			t.Fatal(err)
		}
	}
}

func rescorableReport(results ...CaseResult) RunReport {
	report := reportWith(results...)
	report.ScoringEnvironment = testEnv().Fingerprint()
	return report
}

func firstCase(t *testing.T, kind Kind) Case {
	t.Helper()
	for _, c := range Corpus() {
		if c.Kind == kind && c.Slot != "" {
			return c
		}
	}
	t.Fatalf("no %s case", kind)
	return Case{}
}

func scorable(c Case, repeat int, draft *Draft) CaseResult {
	checks := Checks{Kind: c.Kind, HTTPStatus: 200, UnknownShortcodeCount: 1}
	var probes []QueryProbe
	if draft != nil {
		for slot, text := range draftTexts(c, *draft) {
			probes = append(probes, QueryProbes(slot, text)...)
		}
	}
	return CaseResult{Case: c.ID, Kind: c.Kind, Carrier: c.Carrier, Repeat: repeat, Checks: checks, Draft: draft, Details: &Details{}, Probes: probes}
}

func TestRescoreRefusesAReportMissingADraft(t *testing.T) {
	c := firstCase(t, KindPair)
	slots := map[string]string{c.Slot: `[mrql query="type = note"][header]H[/header][/mrql]`}
	draft := &Draft{Status: 200, Target: "bundle", Valid: true, Slots: slots}
	report := rescorableReport(scorable(c, 0, draft), scorable(c, 1, nil))
	if _, err := RescoreReport(report, testEnv()); err == nil {
		t.Fatal("a report with a missing draft was re-scored")
	}
}

func TestRescoreStampsProvenanceAndCompareRefusesMixes(t *testing.T) {
	c := firstCase(t, KindPair)
	slots := map[string]string{c.Slot: `[mrql query="type = note"][header]H[/header][/mrql]`}
	draft := &Draft{Status: 200, Target: "bundle", Valid: true, Slots: slots}
	measured := rescorableReport(scorable(c, 0, draft), scorable(c, 1, draft))
	measured.ScorerVersion = ScorerVersion
	rescored, err := RescoreReport(measured, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if rescored.ScorerVersion != ScorerVersion || len(rescored.Rescored) != 1 {
		t.Fatalf("provenance = %d %v", rescored.ScorerVersion, rescored.Rescored)
	}
	if got := Compare(measured, rescored); got.Compatible {
		t.Fatalf("measured vs re-scored accepted: %#v", got)
	}
	if got := Compare(rescored, rescored); !got.Compatible {
		t.Fatalf("identically re-scored refused: %v", got.Errors)
	}
	old := rescorableReport(scorable(c, 0, draft), scorable(c, 1, draft)) // no ScorerVersion: version 1
	if got := Compare(old, measured); got.Compatible {
		t.Fatalf("different scorer versions accepted: %#v", got)
	}
}

func TestRescoreKeepsStoredRuntimeProbeVerdicts(t *testing.T) {
	c := firstCase(t, KindPair)
	text := `[mrql query="type = note"][header]H[/header][/mrql]`
	draft := Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{c.Slot: text, c.Slot + "CSS": ""}}
	probes := QueryProbes(c.Slot, text)
	for i := range probes {
		probes[i].Failed = true
	}
	previews := map[string]Preview{c.Slot: {Status: 200, HTML: `<div class="mrql-error">query failed</div>`}}
	checks, details := Score(c, draft, nil, previews, probes, testEnv())
	if checks.MRQLErrors != 1 {
		t.Fatalf("control did not record a query failure: %#v", checks)
	}
	r := CaseResult{Case: c.ID, Kind: c.Kind, Carrier: c.Carrier, Draft: &draft, Previews: previews, Probes: probes, Details: &details, Checks: checks, Pass: checks.Pass()}
	report := rescorableReport(r)
	report.Results[0].Checks.UnknownShortcodeCount = 2
	report.Results[0].Checks.WrongShape = true
	got, err := RescoreReport(report, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Results[0].Checks, checks) {
		t.Fatalf("re-scored checks = %#v, want %#v", got.Results[0].Checks, checks)
	}
	if report.Results[0].Checks.UnknownShortcodeCount != 2 {
		t.Fatal("re-scoring mutated its input")
	}
}

func TestRescoreRefusesMissingRuntimeEvidence(t *testing.T) {
	c := firstCase(t, KindPair)
	draft := Draft{Status: 200, Slots: map[string]string{c.Slot: `[mrql query="type = note"]`}}
	r := scorable(c, 0, &draft)
	r.Probes = nil
	r.Details = &Details{MRQLErrors: []string{c.Slot + ": type = note"}}
	if _, err := RescoreReport(rescorableReport(r), testEnv()); err == nil {
		t.Fatal("re-scored a draft whose runtime probe verdicts are missing")
	}
}

func TestRescoreRefusesMissingOrDifferentSourceEnvironment(t *testing.T) {
	c := firstCase(t, KindPair)
	draft := Draft{Status: 200, Slots: map[string]string{c.Slot: "<p>x</p>", c.Slot + "CSS": ""}}
	original := rescorableReport(scorable(c, 0, &draft))
	for _, source := range []string{"", "different-environment"} {
		report := original
		report.ScoringEnvironment = source
		if _, err := RescoreReport(report, testEnv()); err == nil {
			t.Fatal("reused runtime observations from an unknown or different environment")
		}
	}
	unchecked := Env{Known: shortcodes.KnownFromBuiltins()}
	if _, err := RescoreReport(original, unchecked); err == nil {
		t.Fatal("changed the scoring environment without replaying runtime observations")
	}
	other := original
	other.ScoringEnvironment = unchecked.Fingerprint()
	if cmp := Compare(original, other); cmp.Compatible {
		t.Fatal("compared reports from different scoring environments")
	}
}

func TestRescoreRefusesPartialProbeEvidence(t *testing.T) {
	c := firstCase(t, KindPair)
	draft := Draft{Status: 200, Slots: map[string]string{c.Slot: `[mrql query="type = note"][mrql query="type = group"]`}}
	r := scorable(c, 0, &draft)
	if len(r.Probes) != 2 {
		t.Fatalf("control found %d probes", len(r.Probes))
	}
	r.Probes = r.Probes[:1]
	if _, err := RescoreReport(rescorableReport(r), testEnv()); err == nil {
		t.Fatal("re-scored a report missing one query verdict")
	}
}

func TestOfflineRescorePreservesReplayProvenance(t *testing.T) {
	c := firstCase(t, KindPair)
	draft := Draft{Status: 200, Slots: map[string]string{c.Slot: "<p>x</p>", c.Slot + "CSS": ""}}
	original := rescorableReport(scorable(c, 0, &draft))
	replayed := original
	replayed.Rescored = []string{"previews", "queryProbes", RescoredScore}
	got, err := RescoreReport(replayed, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Rescored, replayed.Rescored) {
		t.Fatalf("lost replay provenance: %v", got.Rescored)
	}
	measured, err := RescoreReport(original, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cmp := Compare(got, measured); cmp.Compatible {
		t.Fatal("offline re-score erased the difference between original and replayed runtime evidence")
	}
}
