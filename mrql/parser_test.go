package mrql

import (
	"strings"
	"testing"
)

// helper to parse and expect no error
func mustParse(t *testing.T, input string) *Query {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", input, err)
	}
	return q
}

// helper to parse and expect an error
func mustFail(t *testing.T, input string) *ParseError {
	t.Helper()
	_, err := Parse(input)
	if err == nil {
		t.Fatalf("Parse(%q) expected error but got nil", input)
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("Parse(%q) expected *ParseError, got %T: %v", input, err, err)
	}
	return pe
}

// Test 1: Empty query returns Query with nil Where
func TestParserEmptyQuery(t *testing.T) {
	q := mustParse(t, "")
	if q.Where != nil {
		t.Errorf("empty query: expected nil Where, got %v", q.Where)
	}
	if q.Limit != -1 {
		t.Errorf("empty query: expected Limit=-1, got %d", q.Limit)
	}
	if q.Offset != -1 {
		t.Errorf("empty query: expected Offset=-1, got %d", q.Offset)
	}
	if len(q.OrderBy) != 0 {
		t.Errorf("empty query: expected empty OrderBy, got %v", q.OrderBy)
	}
}

// Test 2: Simple string comparison: name = "hello"
func TestParserSimpleStringComparison(t *testing.T) {
	q := mustParse(t, `name = "hello"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "name" {
		t.Errorf("expected field name, got %q", cmp.Field.Name())
	}
	if cmp.Operator.Type != TokenEq {
		t.Errorf("expected =, got %v", cmp.Operator.Type)
	}
	str, ok := cmp.Value.(*StringLiteral)
	if !ok {
		t.Fatalf("expected *StringLiteral, got %T", cmp.Value)
	}
	if str.Value != "hello" {
		t.Errorf("expected 'hello', got %q", str.Value)
	}
}

// Test 3: Number with unit: fileSize > 10mb
func TestParserNumberWithUnit(t *testing.T) {
	q := mustParse(t, `fileSize > 10mb`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "fileSize" {
		t.Errorf("expected field fileSize, got %q", cmp.Field.Name())
	}
	if cmp.Operator.Type != TokenGt {
		t.Errorf("expected >, got %v", cmp.Operator.Type)
	}
	num, ok := cmp.Value.(*NumberLiteral)
	if !ok {
		t.Fatalf("expected *NumberLiteral, got %T", cmp.Value)
	}
	if num.Unit != "mb" {
		t.Errorf("expected unit 'mb', got %q", num.Unit)
	}
	if num.Value != 10 {
		t.Errorf("expected value 10, got %f", num.Value)
	}
	// 10mb = 10 * 1024 * 1024
	if num.Raw != 10*1024*1024 {
		t.Errorf("expected raw %d, got %d", 10*1024*1024, num.Raw)
	}
}

// Test 4: Relative date: created > -7d
func TestParserRelativeDate(t *testing.T) {
	q := mustParse(t, `created > -7d`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "created" {
		t.Errorf("expected field created, got %q", cmp.Field.Name())
	}
	rel, ok := cmp.Value.(*RelDateLiteral)
	if !ok {
		t.Fatalf("expected *RelDateLiteral, got %T", cmp.Value)
	}
	if rel.Amount != 7 {
		t.Errorf("expected amount 7, got %d", rel.Amount)
	}
	if rel.Unit != "d" {
		t.Errorf("expected unit 'd', got %q", rel.Unit)
	}
}

// Test 5: Function value: created >= NOW()
func TestParserFunctionValue(t *testing.T) {
	q := mustParse(t, `created >= NOW()`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Operator.Type != TokenGte {
		t.Errorf("expected >=, got %v", cmp.Operator.Type)
	}
	fn, ok := cmp.Value.(*FuncCall)
	if !ok {
		t.Fatalf("expected *FuncCall, got %T", cmp.Value)
	}
	if fn.Name != "NOW()" {
		t.Errorf("expected 'NOW()', got %q", fn.Name)
	}
}

// Test 6: LIKE operator: name ~ "sun*"
func TestParserLikeOperator(t *testing.T) {
	q := mustParse(t, `name ~ "sun*"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Operator.Type != TokenLike {
		t.Errorf("expected ~, got %v", cmp.Operator.Type)
	}
}

// Test 7: NOT LIKE operator: name !~ "*draft*"
func TestParserNotLikeOperator(t *testing.T) {
	q := mustParse(t, `name !~ "*draft*"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Operator.Type != TokenNotLike {
		t.Errorf("expected !~, got %v", cmp.Operator.Type)
	}
}

// Test 8: Dotted field: meta.rating = 5
func TestParserDottedField(t *testing.T) {
	q := mustParse(t, `meta.rating = 5`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "meta.rating" {
		t.Errorf("expected 'meta.rating', got %q", cmp.Field.Name())
	}
	if len(cmp.Field.Parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(cmp.Field.Parts))
	}
}

// Test 9: Boolean AND: name = "a" AND tags = "b"
func TestParserBooleanAnd(t *testing.T) {
	q := mustParse(t, `name = "a" AND tags = "b"`)
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenAnd {
		t.Errorf("expected AND operator, got %v", bin.Operator.Type)
	}
	// Left: name = "a"
	leftCmp, ok := bin.Left.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected left *ComparisonExpr, got %T", bin.Left)
	}
	if leftCmp.Field.Name() != "name" {
		t.Errorf("expected left field 'name', got %q", leftCmp.Field.Name())
	}
	// Right: tags = "b"
	rightCmp, ok := bin.Right.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected right *ComparisonExpr, got %T", bin.Right)
	}
	if rightCmp.Field.Name() != "tags" {
		t.Errorf("expected right field 'tags', got %q", rightCmp.Field.Name())
	}
}

// Test 10: Operator precedence: a OR b AND c should parse as a OR (b AND c)
func TestParserPrecedenceOrAndAnd(t *testing.T) {
	q := mustParse(t, `name = "a" OR name = "b" AND name = "c"`)
	// Top-level should be OR
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenOr {
		t.Errorf("expected top-level OR, got %v", bin.Operator.Type)
	}
	// Right should be AND
	rightBin, ok := bin.Right.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected right *BinaryExpr, got %T", bin.Right)
	}
	if rightBin.Operator.Type != TokenAnd {
		t.Errorf("expected right-side AND, got %v", rightBin.Operator.Type)
	}
}

// Test 11: NOT: NOT name = "draft"
func TestParserNotExpr(t *testing.T) {
	q := mustParse(t, `NOT name = "draft"`)
	not, ok := q.Where.(*NotExpr)
	if !ok {
		t.Fatalf("expected *NotExpr, got %T", q.Where)
	}
	cmp, ok := not.Expr.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected inner *ComparisonExpr, got %T", not.Expr)
	}
	if cmp.Field.Name() != "name" {
		t.Errorf("expected field 'name', got %q", cmp.Field.Name())
	}
}

// Test 12: Parentheses: (a OR b) AND c
func TestParserParenthesizedGrouping(t *testing.T) {
	q := mustParse(t, `(name = "a" OR name = "b") AND name = "c"`)
	// Top-level should be AND
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenAnd {
		t.Errorf("expected top-level AND, got %v", bin.Operator.Type)
	}
	// Left should be OR
	leftBin, ok := bin.Left.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected left *BinaryExpr, got %T", bin.Left)
	}
	if leftBin.Operator.Type != TokenOr {
		t.Errorf("expected left-side OR, got %v", leftBin.Operator.Type)
	}
}

// Test 13: IN expression: tags IN ("a", "b", "c")
func TestParserInExpression(t *testing.T) {
	q := mustParse(t, `tags IN ("a", "b", "c")`)
	inExpr, ok := q.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected *InExpr, got %T", q.Where)
	}
	if inExpr.Field.Name() != "tags" {
		t.Errorf("expected field 'tags', got %q", inExpr.Field.Name())
	}
	if inExpr.Negated {
		t.Error("expected Negated=false")
	}
	if len(inExpr.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(inExpr.Values))
	}
	// Check values
	for i, expected := range []string{"a", "b", "c"} {
		str, ok := inExpr.Values[i].(*StringLiteral)
		if !ok {
			t.Fatalf("value %d: expected *StringLiteral, got %T", i, inExpr.Values[i])
		}
		if str.Value != expected {
			t.Errorf("value %d: expected %q, got %q", i, expected, str.Value)
		}
	}
}

// Test 14: NOT IN expression: category NOT IN ("Archive", "Trash")
func TestParserNotInExpression(t *testing.T) {
	q := mustParse(t, `category NOT IN ("Archive", "Trash")`)
	inExpr, ok := q.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected *InExpr, got %T", q.Where)
	}
	if inExpr.Field.Name() != "category" {
		t.Errorf("expected field 'category', got %q", inExpr.Field.Name())
	}
	if !inExpr.Negated {
		t.Error("expected Negated=true")
	}
	if len(inExpr.Values) != 2 {
		t.Errorf("expected 2 values, got %d", len(inExpr.Values))
	}
}

// Test 15: IS EMPTY
func TestParserIsEmpty(t *testing.T) {
	q := mustParse(t, `description IS EMPTY`)
	isExpr, ok := q.Where.(*IsExpr)
	if !ok {
		t.Fatalf("expected *IsExpr, got %T", q.Where)
	}
	if isExpr.Field.Name() != "description" {
		t.Errorf("expected field 'description', got %q", isExpr.Field.Name())
	}
	if isExpr.Negated {
		t.Error("expected Negated=false")
	}
	if isExpr.IsNull {
		t.Error("expected IsNull=false (EMPTY)")
	}
}

// Test 16: IS NOT NULL
func TestParserIsNotNull(t *testing.T) {
	q := mustParse(t, `description IS NOT NULL`)
	isExpr, ok := q.Where.(*IsExpr)
	if !ok {
		t.Fatalf("expected *IsExpr, got %T", q.Where)
	}
	if !isExpr.Negated {
		t.Error("expected Negated=true")
	}
	if !isExpr.IsNull {
		t.Error("expected IsNull=true (NULL)")
	}
}

// Test 17: IS NULL
func TestParserIsNull(t *testing.T) {
	q := mustParse(t, `description IS NULL`)
	isExpr, ok := q.Where.(*IsExpr)
	if !ok {
		t.Fatalf("expected *IsExpr, got %T", q.Where)
	}
	if isExpr.Negated {
		t.Error("expected Negated=false")
	}
	if !isExpr.IsNull {
		t.Error("expected IsNull=true (NULL)")
	}
}

// Test 18: IS NOT EMPTY
func TestParserIsNotEmpty(t *testing.T) {
	q := mustParse(t, `description IS NOT EMPTY`)
	isExpr, ok := q.Where.(*IsExpr)
	if !ok {
		t.Fatalf("expected *IsExpr, got %T", q.Where)
	}
	if !isExpr.Negated {
		t.Error("expected Negated=true")
	}
	if isExpr.IsNull {
		t.Error("expected IsNull=false (EMPTY)")
	}
}

// Test 19: TEXT search: TEXT ~ "quarterly review"
func TestParserTextSearch(t *testing.T) {
	q := mustParse(t, `TEXT ~ "quarterly review"`)
	textExpr, ok := q.Where.(*TextSearchExpr)
	if !ok {
		t.Fatalf("expected *TextSearchExpr, got %T", q.Where)
	}
	if textExpr.Value.Value != "quarterly review" {
		t.Errorf("expected 'quarterly review', got %q", textExpr.Value.Value)
	}
}

// Test 20: ORDER BY, LIMIT, OFFSET combined
func TestParserOrderByLimitOffset(t *testing.T) {
	q := mustParse(t, `name = "a" ORDER BY created DESC, name ASC LIMIT 10 OFFSET 20`)
	// Check WHERE
	_, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr WHERE clause, got %T", q.Where)
	}
	// Check ORDER BY
	if len(q.OrderBy) != 2 {
		t.Fatalf("expected 2 ORDER BY clauses, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Field.Name() != "created" {
		t.Errorf("expected first ORDER BY field 'created', got %q", q.OrderBy[0].Field.Name())
	}
	if q.OrderBy[0].Ascending {
		t.Error("expected first ORDER BY DESC")
	}
	if q.OrderBy[1].Field.Name() != "name" {
		t.Errorf("expected second ORDER BY field 'name', got %q", q.OrderBy[1].Field.Name())
	}
	if !q.OrderBy[1].Ascending {
		t.Error("expected second ORDER BY ASC")
	}
	// Check LIMIT and OFFSET
	if q.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Limit)
	}
	if q.Offset != 20 {
		t.Errorf("expected OFFSET 20, got %d", q.Offset)
	}
}

// Test 21: Multi-level traversal: parent.parent.name = "a" is now valid (up to 5 parts allowed).
func TestParserThreeLevelFieldRejected(t *testing.T) {
	q := mustParse(t, `parent.parent.name = "a"`)
	comp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if len(comp.Field.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(comp.Field.Parts))
	}
}

// Test 22: TYPE field: type = resource AND name = "a"
// (bare identifier as value)
func TestParserTypeFieldAndBareIdentifier(t *testing.T) {
	q := mustParse(t, `type = resource AND name = "a"`)
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	// Left should be type = resource
	leftCmp, ok := bin.Left.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected left *ComparisonExpr, got %T", bin.Left)
	}
	if leftCmp.Field.Name() != "type" {
		t.Errorf("expected left field 'type', got %q", leftCmp.Field.Name())
	}
	// "resource" is a bare identifier, treated as string-like value (StringLiteral)
	str, ok := leftCmp.Value.(*StringLiteral)
	if !ok {
		t.Fatalf("expected bare identifier as *StringLiteral, got %T", leftCmp.Value)
	}
	if str.Value != "resource" {
		t.Errorf("expected 'resource', got %q", str.Value)
	}
}

// Test 23: Error position: name = "a" AND AND should give error with position
func TestParserErrorPosition(t *testing.T) {
	pe := mustFail(t, `name = "a" AND AND`)
	// The second AND is at position 15 (after "name = \"a\" AND ")
	// We just verify there's a positive position
	if pe.Pos < 0 {
		t.Errorf("expected non-negative error position, got %d", pe.Pos)
	}
	if pe.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// Test 24: Order by without WHERE
func TestParserOrderByOnly(t *testing.T) {
	q := mustParse(t, `ORDER BY name ASC`)
	if q.Where != nil {
		t.Errorf("expected nil WHERE, got %v", q.Where)
	}
	if len(q.OrderBy) != 1 {
		t.Fatalf("expected 1 ORDER BY clause, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Field.Name() != "name" {
		t.Errorf("expected field 'name', got %q", q.OrderBy[0].Field.Name())
	}
	if !q.OrderBy[0].Ascending {
		t.Error("expected ASC")
	}
}

// Test 25: LIMIT and OFFSET without WHERE
func TestParserLimitOffsetOnly(t *testing.T) {
	q := mustParse(t, `LIMIT 5 OFFSET 10`)
	if q.Where != nil {
		t.Errorf("expected nil WHERE, got %v", q.Where)
	}
	if q.Limit != 5 {
		t.Errorf("expected LIMIT 5, got %d", q.Limit)
	}
	if q.Offset != 10 {
		t.Errorf("expected OFFSET 10, got %d", q.Offset)
	}
}

// Test 26: ORDER BY without direction defaults to ASC
func TestParserOrderByDefaultAsc(t *testing.T) {
	q := mustParse(t, `ORDER BY name`)
	if len(q.OrderBy) != 1 {
		t.Fatalf("expected 1 ORDER BY clause, got %d", len(q.OrderBy))
	}
	if !q.OrderBy[0].Ascending {
		t.Error("expected default ASC when no direction specified")
	}
}

// Test 27: Nested NOT: NOT NOT name = "a"
func TestParserNestedNot(t *testing.T) {
	q := mustParse(t, `NOT NOT name = "a"`)
	not1, ok := q.Where.(*NotExpr)
	if !ok {
		t.Fatalf("expected outer *NotExpr, got %T", q.Where)
	}
	not2, ok := not1.Expr.(*NotExpr)
	if !ok {
		t.Fatalf("expected inner *NotExpr, got %T", not1.Expr)
	}
	_, ok = not2.Expr.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected innermost *ComparisonExpr, got %T", not2.Expr)
	}
}

// Test 28: != comparison
func TestParserNotEqualOperator(t *testing.T) {
	q := mustParse(t, `status != "archived"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Operator.Type != TokenNeq {
		t.Errorf("expected !=, got %v", cmp.Operator.Type)
	}
}

// Test 29: < and <= comparisons
func TestParserLtLteOperators(t *testing.T) {
	q1 := mustParse(t, `size < 100`)
	cmp1, ok := q1.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q1.Where)
	}
	if cmp1.Operator.Type != TokenLt {
		t.Errorf("expected <, got %v", cmp1.Operator.Type)
	}

	q2 := mustParse(t, `size <= 100`)
	cmp2, ok := q2.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q2.Where)
	}
	if cmp2.Operator.Type != TokenLte {
		t.Errorf("expected <=, got %v", cmp2.Operator.Type)
	}
}

// Test 30: IN with number values
func TestParserInWithNumbers(t *testing.T) {
	q := mustParse(t, `rating IN (1, 2, 3)`)
	inExpr, ok := q.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected *InExpr, got %T", q.Where)
	}
	if len(inExpr.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(inExpr.Values))
	}
	for i, expected := range []float64{1, 2, 3} {
		num, ok := inExpr.Values[i].(*NumberLiteral)
		if !ok {
			t.Fatalf("value %d: expected *NumberLiteral, got %T", i, inExpr.Values[i])
		}
		if num.Value != expected {
			t.Errorf("value %d: expected %f, got %f", i, expected, num.Value)
		}
	}
}

// Test 31: Complex: TEXT ~ "q" AND category NOT IN ("Archive") ORDER BY name DESC LIMIT 10
func TestParserComplexQuery(t *testing.T) {
	q := mustParse(t, `TEXT ~ "quarterly" AND category NOT IN ("Archive") ORDER BY name DESC LIMIT 10`)
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenAnd {
		t.Errorf("expected AND, got %v", bin.Operator.Type)
	}
	_, ok = bin.Left.(*TextSearchExpr)
	if !ok {
		t.Fatalf("expected left *TextSearchExpr, got %T", bin.Left)
	}
	inExpr, ok := bin.Right.(*InExpr)
	if !ok {
		t.Fatalf("expected right *InExpr, got %T", bin.Right)
	}
	if !inExpr.Negated {
		t.Error("expected NOT IN")
	}
	if len(q.OrderBy) != 1 {
		t.Fatalf("expected 1 ORDER BY clause, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Field.Name() != "name" {
		t.Errorf("expected ORDER BY 'name', got %q", q.OrderBy[0].Field.Name())
	}
	if q.OrderBy[0].Ascending {
		t.Error("expected DESC")
	}
	if q.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Limit)
	}
}

// Test 32: Unmatched left paren error
func TestParserUnmatchedParen(t *testing.T) {
	mustFail(t, `(name = "a"`)
}

// Test 33: Missing value after operator
func TestParserMissingValue(t *testing.T) {
	mustFail(t, `name =`)
}

// Test 34: Dotted field with dotted ORDER BY
func TestParserDottedFieldInOrderBy(t *testing.T) {
	q := mustParse(t, `ORDER BY meta.rating DESC`)
	if len(q.OrderBy) != 1 {
		t.Fatalf("expected 1 ORDER BY clause, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Field.Name() != "meta.rating" {
		t.Errorf("expected 'meta.rating', got %q", q.OrderBy[0].Field.Name())
	}
	if q.OrderBy[0].Ascending {
		t.Error("expected DESC")
	}
}

// Test 35: parent.name as field (2-part dotted field is OK)
func TestParserParentNameField(t *testing.T) {
	q := mustParse(t, `parent.name = "a"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "parent.name" {
		t.Errorf("expected 'parent.name', got %q", cmp.Field.Name())
	}
}

// Test 36: ParseError has all required fields
func TestParserErrorHasFields(t *testing.T) {
	pe := mustFail(t, `name = "a" AND AND`)
	if pe.Message == "" {
		t.Error("ParseError.Message should not be empty")
	}
	// Pos should be >= 0
	if pe.Pos < 0 {
		t.Errorf("ParseError.Pos should be >= 0, got %d", pe.Pos)
	}
}

// Test 37: Multiple OR conditions
func TestParserMultipleOrConditions(t *testing.T) {
	q := mustParse(t, `a = "x" OR b = "y" OR c = "z"`)
	// Should parse as (a = "x" OR b = "y") OR c = "z" (left-associative)
	outerBin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if outerBin.Operator.Type != TokenOr {
		t.Errorf("expected outer OR, got %v", outerBin.Operator.Type)
	}
}

// Test 38: Number without unit
func TestParserNumberWithoutUnit(t *testing.T) {
	q := mustParse(t, `rating = 5`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	num, ok := cmp.Value.(*NumberLiteral)
	if !ok {
		t.Fatalf("expected *NumberLiteral, got %T", cmp.Value)
	}
	if num.Value != 5 {
		t.Errorf("expected 5, got %f", num.Value)
	}
	if num.Unit != "" {
		t.Errorf("expected no unit, got %q", num.Unit)
	}
	if num.Raw != 5 {
		t.Errorf("expected raw 5, got %d", num.Raw)
	}
}

// Test 39: IN with relative dates
func TestParserInWithRelDates(t *testing.T) {
	q := mustParse(t, `created IN (-7d, -30d)`)
	inExpr, ok := q.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected *InExpr, got %T", q.Where)
	}
	if len(inExpr.Values) != 2 {
		t.Errorf("expected 2 values, got %d", len(inExpr.Values))
	}
	rel, ok := inExpr.Values[0].(*RelDateLiteral)
	if !ok {
		t.Fatalf("expected *RelDateLiteral, got %T", inExpr.Values[0])
	}
	if rel.Amount != 7 || rel.Unit != "d" {
		t.Errorf("expected -7d, got -%d%s", rel.Amount, rel.Unit)
	}
}

// Test 40: gb unit for file size
func TestParserGbUnit(t *testing.T) {
	q := mustParse(t, `fileSize < 1gb`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	num, ok := cmp.Value.(*NumberLiteral)
	if !ok {
		t.Fatalf("expected *NumberLiteral, got %T", cmp.Value)
	}
	if num.Unit != "gb" {
		t.Errorf("expected unit 'gb', got %q", num.Unit)
	}
	if num.Raw != 1*1024*1024*1024 {
		t.Errorf("expected raw %d, got %d", 1*1024*1024*1024, num.Raw)
	}
}

// Test 41: kb unit for file size
func TestParserKbUnit(t *testing.T) {
	q := mustParse(t, `fileSize > 500kb`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	num, ok := cmp.Value.(*NumberLiteral)
	if !ok {
		t.Fatalf("expected *NumberLiteral, got %T", cmp.Value)
	}
	if num.Unit != "kb" {
		t.Errorf("expected unit 'kb', got %q", num.Unit)
	}
	if num.Raw != 500*1024 {
		t.Errorf("expected raw %d, got %d", 500*1024, num.Raw)
	}
}

func TestParserMultiPartField(t *testing.T) {
	q := mustParse(t, `owner.parent.name = "test"`)
	comp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatal("expected ComparisonExpr")
	}
	if len(comp.Field.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(comp.Field.Parts))
	}
	if comp.Field.Parts[0].Value != "owner" {
		t.Fatalf("expected part[0] = owner, got %q", comp.Field.Parts[0].Value)
	}
	if comp.Field.Parts[1].Value != "parent" {
		t.Fatalf("expected part[1] = parent, got %q", comp.Field.Parts[1].Value)
	}
	if comp.Field.Parts[2].Value != "name" {
		t.Fatalf("expected part[2] = name, got %q", comp.Field.Parts[2].Value)
	}
}

func TestParserMaxDepthField(t *testing.T) {
	q := mustParse(t, `owner.parent.parent.parent.name = "test"`)
	comp := q.Where.(*ComparisonExpr)
	if len(comp.Field.Parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(comp.Field.Parts))
	}
}

func TestParserTooDeepFieldRejected(t *testing.T) {
	_, err := Parse(`a.b.c.d.e.f = "test"`)
	if err == nil {
		t.Fatal("expected error for 6-part field, got nil")
	}
	if !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("expected 'too deep' error, got: %v", err)
	}
}
