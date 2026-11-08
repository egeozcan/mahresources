package application_context

import (
	"context"
	"strings"
	"testing"

	"mahresources/shortcodes"
)

// The planner's sharedCssAdditions are appended into CustomCSS by assembleFanOut,
// after every pair has passed. A Placeholder only there is caught by the final
// withholding pass, and the pairs stay (review finding 12).
func TestPlannerOnlySharedCSSAdditionsWithPlaceholderAreWithheld(t *testing.T) {
	p := NewTemplatePrivacy("pc")
	ph := p.Text("Protected Heading Text")
	in := TemplateGenerationInput{
		Target: TemplateTargetBundle, Privacy: p,
		BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS", "CustomCSS"},
	}
	fan, err := templateFanOutFor(in)
	if err != nil || fan == nil {
		t.Fatalf("fan-out: %v %v", fan, err)
	}
	plan := templateDesignPlan{SharedCSSAdditions: ".t-x::after{content:'" + ph + "'}"}
	pair := func(base string) pairOutcome {
		return pairOutcome{base: base, result: &TemplateGenerationResult{
			Slots:      map[string]string{base: "<p>ok</p>", base + "CSS": ".t-a{}"},
			SlotIssues: map[string][]shortcodes.LintIssue{},
		}}
	}
	result, err := assembleFanOut(in, fan, plan, []pairOutcome{pair("CustomHeader"), pair("CustomSidebar")})
	if err != nil {
		t.Fatal(err)
	}
	withholdPlaceholders(result, in)
	if strings.Join(result.WithheldFields, ",") != "CustomCSS" {
		t.Fatalf("only the planner CSS carries the Placeholder: %v", result.WithheldFields)
	}
	if result.Slots["CustomCSS"] != "" || result.Slots["CustomHeader"] != "<p>ok</p>" || result.Slots["CustomSidebarCSS"] != ".t-a{}" {
		t.Fatalf("pairs must stay intact: %+v", result.Slots)
	}
	if result.Valid {
		t.Fatal("a withheld field makes the result invalid")
	}
}

// The harness backstop in buildResult, driven through the real method.
func TestHarnessBuildResultWithholdsACommittedPlaceholder(t *testing.T) {
	p := NewTemplatePrivacy("hb")
	ph := p.Text("Protected Heading Text")
	in := TemplateGenerationInput{Target: TemplateTargetBundle, Privacy: p, BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"}, SlotsSelected: true}
	fields, required, css := templateRunFieldsOf(in)
	r := &harnessRun{
		in: in, privacy: p,
		plan:       &harnessPlan{fields: fields, required: required, cssAdditions: css},
		subs:       newTemplateRunSubmissions(in, css),
		fieldNames: map[string]bool{"CustomHeader": true, "CustomHeaderCSS": true},
	}
	// submit refuses a Placeholder, so this can only be reached by a bug: that is
	// what the backstop is for.
	r.subs.commit("CustomHeader", "<p>"+ph+"</p>")
	r.subs.commit("CustomHeaderCSS", ".a{}")
	result := r.buildResult(context.Background(), TemplateStopDone, nil)
	if strings.Join(result.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS" {
		t.Fatalf("a pair is withheld whole: %v", result.WithheldFields)
	}
	if result.Slots["CustomHeader"] != "" || result.Slots["CustomHeaderCSS"] != "" || result.Valid {
		t.Fatalf("%+v", result)
	}
	hasError := false
	for _, issue := range result.SlotIssues["CustomHeader"] {
		hasError = hasError || issue.Severity == "error"
	}
	if !hasError {
		t.Fatalf("a withheld field carries an error issue: %+v", result.SlotIssues)
	}
}
