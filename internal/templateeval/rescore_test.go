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
