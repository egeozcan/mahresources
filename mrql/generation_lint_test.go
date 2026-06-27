package mrql

import (
	"strings"
	"testing"
)

func TestLintGeneratedQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		mutate    func(*Query)
		wantValid bool
		wantMsg   string
	}{
		{
			name:      "valid modest resource query",
			query:     `type = resource AND contentType ~ "image/*" LIMIT 50`,
			wantValid: true,
		},
		{
			name:      "limit too large",
			query:     `type = resource LIMIT 1000000`,
			wantValid: false,
			wantMsg:   "LIMIT must be between 1 and 500",
		},
		{
			name:      "offset too large",
			query:     `type = resource LIMIT 50 OFFSET 10001`,
			wantValid: false,
			wantMsg:   "OFFSET must be between 0 and 10000",
		},
		{
			name:      "text wildcard only",
			query:     `TEXT ~ "*" LIMIT 50`,
			wantValid: false,
			wantMsg:   "TEXT search must contain at least one alphanumeric term",
		},
		{
			name:      "text punctuation only",
			query:     `TEXT ~ "!!!" LIMIT 50`,
			wantValid: false,
			wantMsg:   "TEXT search must contain at least one alphanumeric term",
		},
		{
			name:      "string category rejected",
			query:     `type = group AND category = "Invoices" LIMIT 50`,
			wantValid: false,
			wantMsg:   "category requires a numeric ID in generated MRQL",
		},
		{
			name:      "numeric category allowed",
			query:     `type = group AND category = 7 LIMIT 50`,
			wantValid: true,
		},
		{
			name:      "string note type rejected",
			query:     `type = note AND noteType = "Meeting" LIMIT 50`,
			wantValid: false,
			wantMsg:   "noteType requires a numeric ID in generated MRQL",
		},
		{
			name:      "string resource category rejected",
			query:     `type = resource AND category = "Scans" LIMIT 50`,
			mutate:    renameGeneratedLintField("category", "resourceCategory"),
			wantValid: false,
			wantMsg:   "resourceCategory requires a numeric ID in generated MRQL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			if err := Validate(parsed); err != nil {
				t.Fatalf("Validate(%q): %v", tc.query, err)
			}
			if tc.mutate != nil {
				tc.mutate(parsed)
			}

			errs := LintGeneratedQuery(parsed)
			if tc.wantValid && len(errs) != 0 {
				t.Fatalf("expected no lint errors, got %#v", errs)
			}
			if !tc.wantValid {
				if len(errs) == 0 {
					t.Fatalf("expected lint error containing %q", tc.wantMsg)
				}
				got := errs[0]["message"]
				if got == nil || !strings.Contains(got.(string), tc.wantMsg) {
					t.Fatalf("first lint error = %#v, want message containing %q", errs[0], tc.wantMsg)
				}
			}
		})
	}
}

func renameGeneratedLintField(from, to string) func(*Query) {
	return func(q *Query) {
		walkGeneratedLintTestNode(q.Where, func(n Node) {
			expr, ok := n.(*ComparisonExpr)
			if !ok || expr.Field.Name() != from {
				return
			}
			expr.Field.Parts[0].Value = to
		})
	}
}

func walkGeneratedLintTestNode(n Node, visit func(Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch x := n.(type) {
	case *BinaryExpr:
		walkGeneratedLintTestNode(x.Left, visit)
		walkGeneratedLintTestNode(x.Right, visit)
	case *NotExpr:
		walkGeneratedLintTestNode(x.Expr, visit)
	}
}
