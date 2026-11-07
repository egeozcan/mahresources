package application_context

import (
	"context"
	"testing"

	"mahresources/shortcodes"
)

// A cancelled run must not keep validating: lint, MetaSchema lint and schema
// compilation all see the run's context, so each reports the cancellation
// instead of a verdict on content it never finished checking.
func TestValidateTemplateRunContentHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cases := map[string]string{
		"CustomHeader": "<h1>[property path=\"Name\"]</h1>",
		"CustomCSS":    ".a{color:red}",
		"MetaSchema":   `{"type":"object","properties":{"a":{"type":"string"}}}`,
	}
	for field, content := range cases {
		if issues := ValidateTemplateRunContent(context.Background(), TemplateRunValidation{}, field, content); hasErrorIssue(issues) {
			t.Fatalf("%s: live context gave errors %+v", field, issues)
		}
		issues := ValidateTemplateRunContent(ctx, TemplateRunValidation{}, field, content)
		if !hasErrorIssue(issues) {
			t.Errorf("%s: cancelled validation returned a clean verdict %+v", field, issues)
		}
	}
}

var _ = shortcodes.SeverityError
