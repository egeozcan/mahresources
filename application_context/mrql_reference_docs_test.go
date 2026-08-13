package application_context

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The execution-bound half of the MRQL guardrail table lives here rather than in
// mrql/, because the caps are policy on this side of the seam. mrql's own
// reference_docs_test.go covers the parser-side constants; together they keep
// docs-site/docs/features/mrql-reference.md (the page the agent skill is
// generated from) honest about every documented maximum.

const mrqlReferencePath = "../docs-site/docs/features/mrql-reference.md"

var guardrailNumberRE = regexp.MustCompile(`\d[\d,]*`)

func TestReferenceExecutionLimitsMatchPolicy(t *testing.T) {
	data, err := os.ReadFile(mrqlReferencePath)
	if err != nil {
		t.Fatalf("reading %s: %v", mrqlReferencePath, err)
	}
	page := string(data)

	cases := []struct {
		label string
		want  int
	}{
		{"Interactive `LIMIT`", MaxMRQLInteractiveLimit},
		{"Interactive `OFFSET`", MaxMRQLInteractiveOffset},
		{"Export `LIMIT`", MaxMRQLExportLimit},
		{"Export `OFFSET`", MaxMRQLExportOffset},
	}

	for _, tc := range cases {
		got, ok := guardrailMax(page, tc.label)
		if !ok {
			t.Errorf("%s: no guardrail row for %q", mrqlReferencePath, tc.label)
			continue
		}
		if got != fmt.Sprint(tc.want) {
			t.Errorf("guardrail %q documents %q, want %d", tc.label, got, tc.want)
		}
	}
}

func guardrailMax(page, label string) (string, bool) {
	for _, line := range strings.Split(page, "\n") {
		if !strings.HasPrefix(line, "| "+label) {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			return "", false
		}
		return strings.ReplaceAll(guardrailNumberRE.FindString(cells[2]), ",", ""), true
	}
	return "", false
}
