package mrql

import (
	"strings"
	"testing"
)

// TestLexerRegexOperators verifies ~* and !~* lex to distinct tokens, and that
// `~ *` (with a space) stays TokenLike followed by an illegal '*'.
func TestLexerRegexOperators(t *testing.T) {
	cases := []struct {
		input    string
		wantType TokenType
		wantVal  string
	}{
		{"~*", TokenRegex, "~*"},
		{"!~*", TokenNotRegex, "!~*"},
		{"~", TokenLike, "~"},
		{"!~", TokenNotLike, "!~"},
	}
	for _, c := range cases {
		l := NewLexer(c.input)
		tok := l.Next()
		if tok.Type != c.wantType || tok.Value != c.wantVal {
			t.Errorf("input=%q: got (%v,%q), want (%v,%q)", c.input, tok.Type, tok.Value, c.wantType, c.wantVal)
		}
	}

	// `~ *` — the space breaks the two-char operator: ~ stays TokenLike, * is illegal.
	l := NewLexer("~ *")
	first := l.Next()
	if first.Type != TokenLike {
		t.Fatalf("`~ *`: expected first token TokenLike, got %v", first.Type)
	}
	second := l.Next()
	if second.Type != TokenIllegal {
		t.Fatalf("`~ *`: expected second token TokenIllegal, got %v", second.Type)
	}
}

// TestParseRegexOperator confirms ~* / !~* flow into ComparisonExpr.Operator.
func TestParseRegexOperator(t *testing.T) {
	q := mustParse(t, `type = "resource" AND name ~* "^IMG_"`)
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", q.Where)
	}
	cmp, ok := bin.Right.(*ComparisonExpr)
	if !ok || cmp.Operator.Type != TokenRegex {
		t.Fatalf("expected right = ComparisonExpr(~*), got %T", bin.Right)
	}

	q2 := mustParse(t, `type = "resource" AND name !~* "\.tmp$"`)
	bin2 := q2.Where.(*BinaryExpr)
	cmp2, ok := bin2.Right.(*ComparisonExpr)
	if !ok || cmp2.Operator.Type != TokenNotRegex {
		t.Fatalf("expected right = ComparisonExpr(!~*), got %T", bin2.Right)
	}
}

// TestRegexRejectedOnSQLite verifies the translator errors on SQLite (no native regex).
func TestRegexRejectedOnSQLite(t *testing.T) {
	db := setupTestDB(t)
	q := mustParse(t, `type = "resource" AND name ~* "^IMG_"`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validate error: %v", err)
	}
	_, err := Translate(q, db)
	if err == nil || !strings.Contains(err.Error(), "requires PostgreSQL") {
		t.Fatalf("expected SQLite regex rejection, got %v", err)
	}
	var te *TranslateError
	if !asTranslateError(err, &te) {
		t.Fatalf("expected *TranslateError, got %T", err)
	}
}

func asTranslateError(err error, target **TranslateError) bool {
	if te, ok := err.(*TranslateError); ok {
		*target = te
		return true
	}
	return false
}

// TestRegexRejectedOnNumericField validates numeric/datetime fields reject regex.
func TestRegexRejectedOnNumericField(t *testing.T) {
	for _, q := range []string{
		`type = "resource" AND fileSize ~* "1"`,
		`type = "resource" AND created ~* "2024"`,
	} {
		parsed := mustParse(t, q)
		parsed.EntityType = EntityResource
		err := Validate(parsed)
		if err == nil || !strings.Contains(err.Error(), "does not support regex match") {
			t.Fatalf("query %q: expected numeric/datetime rejection, got %v", q, err)
		}
	}
}

// TestRegexRejectedOnTraversalNonStringLeaf validates that traversal and
// recursive chains reject regex on tags and numeric/datetime leaves — the same
// rule enforced for single-part fields. Without this, `owner.tags ~* "x"` would
// silently translate to an equality match on the pattern string.
func TestRegexRejectedOnTraversalNonStringLeaf(t *testing.T) {
	for _, q := range []string{
		`type = "resource" AND owner.tags ~* "^x"`,
		`type = "resource" AND owner.tags !~* "^x"`,
		`type = "group" AND ancestors.tags ~* "^x"`,
		`type = "group" AND descendants.tags !~* "^x"`,
		`type = "resource" AND owner.id ~* "1"`,
		`type = "resource" AND owner.created ~* "2024"`,
	} {
		parsed := mustParse(t, q)
		err := Validate(parsed)
		if err == nil || !strings.Contains(err.Error(), "does not support regex match") {
			t.Fatalf("query %q: expected traversal-leaf rejection, got %v", q, err)
		}
	}
}

// TestRegexAllowedOnStringAndMetaLeaves pins the allowed set: string leaves,
// meta keys (including one literally named "tags"), and traversal meta subpaths.
func TestRegexAllowedOnStringAndMetaLeaves(t *testing.T) {
	for _, q := range []string{
		`type = "resource" AND owner.name ~* "^x"`,
		`type = "resource" AND meta.tags ~* "^x"`,
		`type = "resource" AND owner.meta.tags ~* "^x"`,
		`type = "group" AND ancestors.name ~* "^x"`,
	} {
		parsed := mustParse(t, q)
		if err := Validate(parsed); err != nil {
			t.Fatalf("query %q: expected valid, got %v", q, err)
		}
	}
}

// TestRegexRequiresStringPattern rejects a non-string regex value.
func TestRegexRequiresStringPattern(t *testing.T) {
	parsed := mustParse(t, `type = "resource" AND name ~* 5`)
	parsed.EntityType = EntityResource
	err := Validate(parsed)
	if err == nil || !strings.Contains(err.Error(), "string pattern") {
		t.Fatalf("expected string-pattern rejection, got %v", err)
	}
}

// TestContainsRegexOperator confirms the AST scan the app-context gate uses.
func TestContainsRegexOperator(t *testing.T) {
	yes := mustParse(t, `type = "resource" AND (name ~* "x" OR name = "y")`)
	if !ContainsRegexOperator(yes) {
		t.Fatalf("expected ContainsRegexOperator true")
	}
	no := mustParse(t, `type = "resource" AND name ~ "x"`)
	if ContainsRegexOperator(no) {
		t.Fatalf("expected ContainsRegexOperator false for plain ~")
	}
}
