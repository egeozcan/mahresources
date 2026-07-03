package mrql

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// mustParse parses or fails the test.
func mustParseParam(t *testing.T, s string) *Query {
	t.Helper()
	q, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", s, err)
	}
	return q
}

func TestLexParam(t *testing.T) {
	lex := NewLexer("tags = $tag AND name ~ $needle")
	var params []string
	for {
		tok := lex.Next()
		if tok.Type == TokenEOF {
			break
		}
		if tok.Type == TokenParam {
			params = append(params, tok.Value)
		}
		if tok.Type == TokenIllegal {
			t.Fatalf("unexpected illegal token %q", tok.Value)
		}
	}
	if !reflect.DeepEqual(params, []string{"tag", "needle"}) {
		t.Fatalf("got %v, want [tag needle]", params)
	}
}

func TestLexParam_IllegalLoneDollar(t *testing.T) {
	for _, s := range []string{"$", "$ ", "$1", "$-", "$."} {
		lex := NewLexer("name = " + s)
		var sawIllegal bool
		for {
			tok := lex.Next()
			if tok.Type == TokenEOF {
				break
			}
			if tok.Type == TokenIllegal {
				sawIllegal = true
			}
		}
		if !sawIllegal {
			t.Errorf("expected illegal token for %q", s)
		}
	}
}

func TestParam_InString_StaysLiteral(t *testing.T) {
	// $x inside a quoted string is literal text, not a placeholder.
	q := mustParseParam(t, `name = "$x"`)
	if got := ListParams(q); len(got) != 0 {
		t.Fatalf("expected no params, got %v", got)
	}
	cmp := q.Where.(*ComparisonExpr)
	if sl, ok := cmp.Value.(*StringLiteral); !ok || sl.Value != "$x" {
		t.Fatalf("expected string literal \"$x\", got %#v", cmp.Value)
	}
}

func TestListParams_FirstAppearanceDedup(t *testing.T) {
	q := mustParseParam(t, `type = resource AND tags = $tag AND created > $since AND name ~ $tag`)
	got := ListParams(q)
	want := []string{"tag", "since"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListParams_InAndHaving(t *testing.T) {
	q := mustParseParam(t, `type = resource AND tags IN ($a, $b) GROUP BY contentType COUNT() HAVING COUNT() > $min`)
	got := ListParams(q)
	want := []string{"a", "b", "min"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBindParams_MissingAndExtra(t *testing.T) {
	q := mustParseParam(t, `type = resource AND tags = $tag`)
	if err := BindParams(q, map[string]any{}); err == nil {
		t.Fatal("expected missing-param error")
	} else if err.Error() != "missing parameter $tag" {
		t.Fatalf("unexpected message: %q", err.Error())
	}

	q2 := mustParseParam(t, `type = resource AND tags = $tag`)
	if err := BindParams(q2, map[string]any{"tag": "x", "other": "y"}); err == nil {
		t.Fatal("expected unknown-param error")
	} else if err.Error() != "unknown parameter $other" {
		t.Fatalf("unexpected message: %q", err.Error())
	}
}

func TestBindParams_MissingMultiple(t *testing.T) {
	q := mustParseParam(t, `type = resource AND tags = $tag AND created > $since`)
	err := BindParams(q, map[string]any{})
	if err == nil || err.Error() != "missing parameters: $tag, $since" {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBindParams_DuplicatePlaceholderBindsAll(t *testing.T) {
	q := mustParseParam(t, `type = resource AND name = $x OR description = $x`)
	if err := BindParams(q, map[string]any{"x": "hello"}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	// Both occurrences should now be StringLiteral "hello".
	var count int
	walkValueNodes(q, func(np *Node) {
		if sl, ok := (*np).(*StringLiteral); ok && sl.Value == "hello" {
			count++
		}
	})
	if count != 2 {
		t.Fatalf("expected 2 bound occurrences, got %d", count)
	}
	if len(ListParams(q)) != 0 {
		t.Fatal("expected no remaining params after bind")
	}
}

func TestCoerceStringParam_Table(t *testing.T) {
	tok := Token{Type: TokenParam, Value: "p"}
	tests := []struct {
		name   string
		in     string
		assert func(t *testing.T, n Node)
	}{
		{"bareNumber", "42", func(t *testing.T, n Node) {
			nl, ok := n.(*NumberLiteral)
			if !ok || nl.Value != 42 || nl.Unit != "" {
				t.Fatalf("got %#v", n)
			}
		}},
		{"unitNumber", "10mb", func(t *testing.T, n Node) {
			nl, ok := n.(*NumberLiteral)
			if !ok || nl.Unit != "mb" || nl.Raw != 10*1024*1024 {
				t.Fatalf("got %#v", n)
			}
		}},
		{"relDate", "-7d", func(t *testing.T, n Node) {
			rd, ok := n.(*RelDateLiteral)
			if !ok || rd.Amount != 7 || rd.Unit != "d" {
				t.Fatalf("got %#v", n)
			}
		}},
		{"funcNow", "NOW()", func(t *testing.T, n Node) {
			if _, ok := n.(*FuncCall); !ok {
				t.Fatalf("got %#v", n)
			}
		}},
		{"quotedForceString", `"42"`, func(t *testing.T, n Node) {
			sl, ok := n.(*StringLiteral)
			if !ok || sl.Value != "42" {
				t.Fatalf("got %#v", n)
			}
		}},
		{"bareIdentifier", "photo", func(t *testing.T, n Node) {
			sl, ok := n.(*StringLiteral)
			if !ok || sl.Value != "photo" {
				t.Fatalf("got %#v", n)
			}
		}},
		{"injectionShaped", `x" OR 1=1`, func(t *testing.T, n Node) {
			sl, ok := n.(*StringLiteral)
			if !ok || sl.Value != `x" OR 1=1` {
				t.Fatalf("got %#v", n)
			}
		}},
		{"withSpaces", "hello world", func(t *testing.T, n Node) {
			sl, ok := n.(*StringLiteral)
			if !ok || sl.Value != "hello world" {
				t.Fatalf("got %#v", n)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, coerceStringParam(tc.in, tok))
		})
	}
}

func TestCoerceParamValue_JSONNumber(t *testing.T) {
	tok := Token{Type: TokenParam, Value: "p"}
	n, err := coerceParamValue(float64(5), tok)
	if err != nil {
		t.Fatal(err)
	}
	nl, ok := n.(*NumberLiteral)
	if !ok || nl.Value != 5 || nl.Raw != 5 {
		t.Fatalf("got %#v", n)
	}
}

func TestBindParams_ReValidatesAfterBind(t *testing.T) {
	// A parameterized query validates while unbound.
	q := mustParseParam(t, `type = resource AND fileSize > $min`)
	if err := Validate(q); err != nil {
		t.Fatalf("unbound validate failed: %v", err)
	}
	// Bind a numeric value; re-validate should pass (numeric field, numeric value).
	if err := BindParams(q, map[string]any{"min": float64(1000)}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("post-bind validate failed: %v", err)
	}
}

func TestBindParams_PostBindTypeMismatch(t *testing.T) {
	// fileSize is numeric; binding a non-numeric string should fail re-validation.
	q := mustParseParam(t, `type = resource AND fileSize > $min`)
	if err := BindParams(q, map[string]any{"min": "notanumber"}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected post-bind validation to reject non-numeric value on numeric field")
	}
}

func TestGenerationLint_RejectsParams(t *testing.T) {
	q := mustParseParam(t, `type = resource AND tags = $tag LIMIT 10`)
	errs := LintGeneratedQuery(q)
	var found bool
	for _, e := range errs {
		if msg, _ := e["message"].(string); strings.Contains(msg, "parameter") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected param rejection in lint errors, got %v", errs)
	}
}

func TestBindParams_RelDateResolvesToTime(t *testing.T) {
	q := mustParseParam(t, `type = resource AND created > $since`)
	if err := BindParams(q, map[string]any{"since": "-7d"}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	cmp := q.Where.(*BinaryExpr).Right.(*ComparisonExpr)
	rd, ok := cmp.Value.(*RelDateLiteral)
	if !ok {
		t.Fatalf("expected RelDateLiteral, got %#v", cmp.Value)
	}
	// Sanity: resolves to a time roughly 7 days ago.
	got := resolveRelativeDate(rd)
	if got.After(time.Now()) {
		t.Fatalf("relative date resolved into the future: %v", got)
	}
}
