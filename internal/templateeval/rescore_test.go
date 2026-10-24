package templateeval

import (
	"os"
	"testing"

	"mahresources/shortcodes"
)

// TestRescoreReport re-scores the report named by TEMPLATE_EVAL_RESCORE with
// the current scorer and logs the corrected summary. It skips without it.
func TestRescoreReport(t *testing.T) {
	path := os.Getenv("TEMPLATE_EVAL_RESCORE")
	if path == "" {
		t.Skip("TEMPLATE_EVAL_RESCORE is not set")
	}
	report, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	before := report.Summary
	got, err := RescoreUnknownShortcodes(report, Env{Known: shortcodes.KnownFromBuiltins()})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recorded:\n%s\nre-scored:\n%s", FormatSummary(before), FormatSummary(got.Summary))
	if out := os.Getenv("TEMPLATE_EVAL_RESCORE_OUT"); out != "" {
		if err := WriteJSONAtomic(out, got); err != nil {
			t.Fatal(err)
		}
	}
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
	return CaseResult{Case: c.ID, Kind: c.Kind, Carrier: c.Carrier, Repeat: repeat, Checks: checks, Draft: draft}
}

func TestRescoreRefusesAReportMissingADraft(t *testing.T) {
	c := firstCase(t, KindPair)
	slots := map[string]string{c.Slot: `[mrql query="type = note"][header]H[/header][/mrql]`}
	draft := &Draft{Status: 200, Target: "bundle", Valid: true, Slots: slots}
	report := reportWith(scorable(c, 0, draft), scorable(c, 1, nil))
	if _, err := RescoreUnknownShortcodes(report, Env{Known: shortcodes.KnownFromBuiltins()}); err == nil {
		t.Fatal("a report with a missing draft was re-scored")
	}
}

func TestRescoreStampsProvenanceAndCompareRefusesMixes(t *testing.T) {
	c := firstCase(t, KindPair)
	slots := map[string]string{c.Slot: `[mrql query="type = note"][header]H[/header][/mrql]`}
	draft := &Draft{Status: 200, Target: "bundle", Valid: true, Slots: slots}
	measured := reportWith(scorable(c, 0, draft), scorable(c, 1, draft))
	measured.ScorerVersion = ScorerVersion
	rescored, err := RescoreUnknownShortcodes(measured, Env{Known: shortcodes.KnownFromBuiltins()})
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
	old := reportWith(scorable(c, 0, draft), scorable(c, 1, draft)) // no ScorerVersion: version 1
	if got := Compare(old, measured); got.Compatible {
		t.Fatalf("different scorer versions accepted: %#v", got)
	}
}
