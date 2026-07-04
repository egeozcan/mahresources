package mrql

import "testing"

func TestInsertScopeClause(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"bare entity", `type = "resource"`, `type = "resource" SCOPE 5`},
		{"where only", `name ~ "x"`, `name ~ "x" SCOPE 5`},
		{"before LIMIT", `type = "resource" LIMIT 10`, `type = "resource" SCOPE 5 LIMIT 10`},
		{"before OFFSET", `type = "resource" OFFSET 3`, `type = "resource" SCOPE 5 OFFSET 3`},
		{"before ORDER BY", `type = "resource" ORDER BY name`, `type = "resource" SCOPE 5 ORDER BY name`},
		{"before LIMIT and OFFSET", `type = "resource" LIMIT 10 OFFSET 5`, `type = "resource" SCOPE 5 LIMIT 10 OFFSET 5`},
		{"before GROUP BY", `type = "resource" GROUP BY contentType COUNT()`, `type = "resource" SCOPE 5 GROUP BY contentType COUNT()`},
		{"where then order/limit", `name ~ "x" ORDER BY name DESC LIMIT 20`, `name ~ "x" SCOPE 5 ORDER BY name DESC LIMIT 20`},
		{"trailing whitespace", `type = "resource"   `, `type = "resource" SCOPE 5`},
		{"keyword-like string literal not confused", `name = "my LIMIT test"`, `name = "my LIMIT test" SCOPE 5`},
		{"already scoped is unchanged", `type = "resource" SCOPE 9`, `type = "resource" SCOPE 9`},
		{"already scoped before limit unchanged", `type = "resource" SCOPE 9 LIMIT 4`, `type = "resource" SCOPE 9 LIMIT 4`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InsertScopeClause(tc.query, 5)
			if got != tc.want {
				t.Fatalf("InsertScopeClause(%q) = %q, want %q", tc.query, got, tc.want)
			}
			// The result must parse (unless it was already scoped, which we don't
			// re-scope) — proving the SCOPE landed in a valid position.
			if _, err := Parse(got); err != nil {
				t.Fatalf("result %q does not parse: %v", got, err)
			}
		})
	}
}
