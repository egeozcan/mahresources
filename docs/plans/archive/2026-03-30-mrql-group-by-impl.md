# MRQL GROUP BY Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GROUP BY support with aggregate functions (COUNT, SUM, AVG, MIN, MAX) to MRQL, supporting both flat aggregation mode and bucketed entity mode.

**Architecture:** Extend the existing lexer → parser → validator → translator → executor pipeline. New tokens and AST nodes for GROUP BY/aggregates. Translator branches into two paths: aggregated (SQL GROUP BY returning `[]map[string]any`) and bucketed (keys query + per-bucket entity queries). New `MRQLGroupedResult` response type.

**Tech Stack:** Go, GORM, SQLite (json1), PostgreSQL, Playwright (E2E), Cobra (CLI)

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `mrql/token.go` | Modify | Add `TokenGroupBy`, `TokenCount`, `TokenSum`, `TokenAvg`, `TokenMin`, `TokenMax` |
| `mrql/lexer.go` | Modify | Recognize `GROUP BY` (two-word merge), aggregate keywords with peek-ahead for `(` |
| `mrql/ast.go` | Modify | Add `AggregateFunc`, `GroupByClause` structs; add `GroupBy` field to `Query` |
| `mrql/parser.go` | Modify | Parse GROUP BY fields and aggregate functions between expression and ORDER BY |
| `mrql/validator.go` | Modify | Validate GROUP BY: entity type required, field types, no traversals, aggregate field types, ORDER BY interaction |
| `mrql/translator.go` | Modify | Add `TranslateGroupBy()` for aggregated mode SQL generation |
| `mrql/translator_groupby.go` | Create | Bucketed mode: keys query + per-bucket queries |
| `mrql/completer.go` | Modify | Suggest GROUP BY, aggregate functions at appropriate positions |
| `application_context/mrql_context.go` | Modify | Add `MRQLGroupedResult`, `MRQLBucket`, `executeGroupedQuery()`, `executeAggregatedQuery()`, `executeBucketedQuery()` |
| `server/api_handlers/mrql_api_handlers.go` | Modify | Return `MRQLGroupedResult` when GROUP BY present |
| `cmd/mr/commands/mrql.go` | Modify | Render aggregated/bucketed output in CLI |
| `docs-site/docs/features/mrql.md` | Modify | Document GROUP BY syntax, aggregates, examples |
| `mrql/lexer_test.go` | Modify | Test new tokens |
| `mrql/parser_test.go` | Modify | Test GROUP BY/aggregate parsing |
| `mrql/validator_test.go` | Modify | Test validation rules |
| `mrql/translator_test.go` | Modify | Test SQL generation |
| `mrql/translator_comprehensive_test.go` | Modify | End-to-end GROUP BY tests with seeded data |

---

### Task 1: Tokens and Lexer

**Files:**
- Modify: `mrql/token.go`
- Modify: `mrql/lexer.go`
- Test: `mrql/lexer_test.go`

- [ ] **Step 1: Write failing lexer tests for GROUP BY and aggregate tokens**

Add to `mrql/lexer_test.go`:

```go
func TestLexer_GroupBy(t *testing.T) {
	l := NewLexer("GROUP BY contentType")
	tok := l.Next()
	if tok.Type != TokenGroupBy || tok.Value != "GROUP BY" {
		t.Errorf("expected TokenGroupBy 'GROUP BY', got %v %q", tok.Type, tok.Value)
	}
	tok = l.Next()
	if tok.Type != TokenIdentifier || tok.Value != "contentType" {
		t.Errorf("expected identifier 'contentType', got %v %q", tok.Type, tok.Value)
	}
}

func TestLexer_GroupByCaseInsensitive(t *testing.T) {
	l := NewLexer("group by name")
	tok := l.Next()
	if tok.Type != TokenGroupBy {
		t.Errorf("expected TokenGroupBy, got %v %q", tok.Type, tok.Value)
	}
}

func TestLexer_AggregateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
		value    string
	}{
		{"COUNT(", TokenCount, "COUNT"},
		{"SUM(", TokenSum, "SUM"},
		{"AVG(", TokenAvg, "AVG"},
		{"MIN(", TokenMin, "MIN"},
		{"MAX(", TokenMax, "MAX"},
		{"count(", TokenCount, "count"},
		{"Sum(", TokenSum, "Sum"},
	}
	for _, tt := range tests {
		l := NewLexer(tt.input)
		tok := l.Next()
		if tok.Type != tt.expected {
			t.Errorf("input %q: expected %v, got %v %q", tt.input, tt.expected, tok.Type, tok.Value)
		}
	}
}

func TestLexer_AggregateWithoutParenIsIdentifier(t *testing.T) {
	// "count" without "(" should be a plain identifier
	l := NewLexer("count = 5")
	tok := l.Next()
	if tok.Type != TokenIdentifier {
		t.Errorf("expected TokenIdentifier for bare 'count', got %v", tok.Type)
	}
}

func TestLexer_GroupWithoutByIsIdentifier(t *testing.T) {
	// "group" without "BY" should be a plain identifier (for field names)
	l := NewLexer("group = \"Photos\"")
	tok := l.Next()
	if tok.Type != TokenIdentifier {
		t.Errorf("expected TokenIdentifier for bare 'group', got %v", tok.Type)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestLexer_Group|TestLexer_Aggregate' -v`
Expected: FAIL — `TokenGroupBy`, `TokenCount`, etc. are undefined.

- [ ] **Step 3: Add new token types to `token.go`**

Add after `TokenOffset` (line ~26 in `token.go`):

```go
TokenGroupBy // GROUP BY (two words, merged by lexer)
TokenCount   // COUNT (followed by '(')
TokenSum     // SUM (followed by '(')
TokenAvg     // AVG (followed by '(')
TokenMin     // MIN (followed by '(')
TokenMax     // MAX (followed by '(')
```

- [ ] **Step 4: Add GROUP BY merging and aggregate peek-ahead in `lexer.go`**

In `readWord()`, after the `ORDER BY` check (around line 253), add the `GROUP BY` check using the same pattern:

```go
// Check for GROUP BY (two-word keyword): word is "GROUP" followed by whitespace then "BY"
if upper == "GROUP" {
	savedPos := l.pos
	tmp := l.pos
	for tmp < len(l.input) && unicode.IsSpace(rune(l.input[tmp])) {
		tmp++
	}
	if tmp+2 <= len(l.input) && strings.ToUpper(l.input[tmp:tmp+2]) == "BY" {
		endBy := tmp + 2
		if endBy >= len(l.input) || !isWordChar(l.input[endBy]) {
			l.pos = endBy
			return Token{
				Type:   TokenGroupBy,
				Value:  "GROUP BY",
				Pos:    start,
				Length: l.pos - start,
			}
		}
	}
	l.pos = savedPos
}
```

Then, before the keyword lookup (around line 265), add aggregate peek-ahead. Replace the existing function-check block and keyword lookup with:

```go
// Check if it's an aggregate function keyword: word followed by "("
// (but NOT "()") — aggregate parens are consumed by parser, not lexer)
if l.pos < len(l.input) && l.input[l.pos] == '(' {
	if aggType, ok := aggregateKeywords[upper]; ok {
		return Token{Type: aggType, Value: word, Pos: start, Length: l.pos - start}
	}
}

// Check if it's a function call: word followed by "()"
if l.pos+1 < len(l.input) && l.input[l.pos] == '(' && l.input[l.pos+1] == ')' {
	funcName := upper
	if isFunctionName(funcName) {
		l.pos += 2
		val := l.input[start:l.pos]
		return Token{Type: TokenFunc, Value: val, Pos: start, Length: l.pos - start}
	}
}
```

Add the `aggregateKeywords` map near the other maps:

```go
// aggregateKeywords maps aggregate function names to their token types.
// These are only recognized when immediately followed by '('.
var aggregateKeywords = map[string]TokenType{
	"COUNT": TokenCount,
	"SUM":   TokenSum,
	"AVG":   TokenAvg,
	"MIN":   TokenMin,
	"MAX":   TokenMax,
}
```

**Important:** The aggregate check MUST come before the existing function call check (`l.pos+1 < len(l.input) && l.input[l.pos] == '(' && l.input[l.pos+1] == ')'`). The aggregate check matches `WORD(` (without consuming the `(`), and the existing function check matches `WORD()` (consuming both parens). `COUNT()` should hit the aggregate branch first since `(` matches, and the parser will consume `()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestLexer_Group|TestLexer_Aggregate' -v`
Expected: PASS

- [ ] **Step 6: Run full lexer test suite to ensure no regressions**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestLexer -v`
Expected: All existing tests PASS

- [ ] **Step 7: Commit**

```bash
git add mrql/token.go mrql/lexer.go mrql/lexer_test.go
git commit -m "feat(mrql): add GROUP BY and aggregate tokens to lexer"
```

---

### Task 2: AST Nodes

**Files:**
- Modify: `mrql/ast.go`

- [ ] **Step 1: Add new AST types and modify Query struct in `ast.go`**

Add after the `FuncCall` type (around line 125):

```go
// AggregateFunc represents an aggregate function call: COUNT(), SUM(field), etc.
type AggregateFunc struct {
	Token Token      // the aggregate keyword token (COUNT, SUM, etc.)
	Name  string     // uppercase: "COUNT", "SUM", "AVG", "MIN", "MAX"
	Field *FieldExpr // nil for COUNT(), required for SUM/AVG/MIN/MAX
}

// GroupByClause holds GROUP BY fields and optional aggregate functions.
type GroupByClause struct {
	Fields     []*FieldExpr    // the fields to group by
	Aggregates []AggregateFunc // aggregate functions (empty = bucketed mode)
}
```

Add the `GroupBy` field to the `Query` struct (after `Where`):

```go
type Query struct {
	Where      Node             // the filter expression (may be nil)
	GroupBy    *GroupByClause   // GROUP BY clause (nil when absent)
	OrderBy    []OrderByClause  // ORDER BY clauses (may be empty)
	Limit      int              // -1 if not specified
	Offset     int              // -1 if not specified
	EntityType EntityType       // populated by validator or caller
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5' ./mrql/...`
Expected: Compiles without errors

- [ ] **Step 3: Commit**

```bash
git add mrql/ast.go
git commit -m "feat(mrql): add GroupByClause and AggregateFunc AST nodes"
```

---

### Task 3: Parser

**Files:**
- Modify: `mrql/parser.go`
- Test: `mrql/parser_test.go`

- [ ] **Step 1: Write failing parser tests**

Add to `mrql/parser_test.go`:

```go
func TestParser_GroupBySimple(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.GroupBy == nil {
		t.Fatal("expected GroupBy to be set")
	}
	if len(q.GroupBy.Fields) != 1 || q.GroupBy.Fields[0].Name() != "contentType" {
		t.Errorf("expected GROUP BY [contentType], got %v", q.GroupBy.Fields)
	}
	if len(q.GroupBy.Aggregates) != 0 {
		t.Errorf("expected no aggregates, got %d", len(q.GroupBy.Aggregates))
	}
}

func TestParser_GroupByMultipleFields(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType, meta.source`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.GroupBy == nil || len(q.GroupBy.Fields) != 2 {
		t.Fatalf("expected 2 GROUP BY fields, got %v", q.GroupBy)
	}
	if q.GroupBy.Fields[0].Name() != "contentType" {
		t.Errorf("first field: expected 'contentType', got %q", q.GroupBy.Fields[0].Name())
	}
	if q.GroupBy.Fields[1].Name() != "meta.source" {
		t.Errorf("second field: expected 'meta.source', got %q", q.GroupBy.Fields[1].Name())
	}
}

func TestParser_GroupByWithCount(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.GroupBy == nil {
		t.Fatal("expected GroupBy to be set")
	}
	if len(q.GroupBy.Aggregates) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(q.GroupBy.Aggregates))
	}
	agg := q.GroupBy.Aggregates[0]
	if agg.Name != "COUNT" {
		t.Errorf("expected COUNT, got %q", agg.Name)
	}
	if agg.Field != nil {
		t.Errorf("expected nil field for COUNT(), got %v", agg.Field)
	}
}

func TestParser_GroupByWithMultipleAggregates(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.GroupBy == nil || len(q.GroupBy.Aggregates) != 3 {
		t.Fatalf("expected 3 aggregates, got %v", q.GroupBy)
	}
	if q.GroupBy.Aggregates[0].Name != "COUNT" || q.GroupBy.Aggregates[0].Field != nil {
		t.Errorf("agg[0]: expected COUNT(), got %v", q.GroupBy.Aggregates[0])
	}
	if q.GroupBy.Aggregates[1].Name != "SUM" || q.GroupBy.Aggregates[1].Field.Name() != "fileSize" {
		t.Errorf("agg[1]: expected SUM(fileSize), got %v", q.GroupBy.Aggregates[1])
	}
	if q.GroupBy.Aggregates[2].Name != "AVG" || q.GroupBy.Aggregates[2].Field.Name() != "fileSize" {
		t.Errorf("agg[2]: expected AVG(fileSize), got %v", q.GroupBy.Aggregates[2])
	}
}

func TestParser_GroupByWithOrderByLimitOffset(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC LIMIT 10 OFFSET 5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.GroupBy == nil {
		t.Fatal("expected GroupBy")
	}
	if len(q.OrderBy) != 1 || q.OrderBy[0].Field.Name() != "count" || q.OrderBy[0].Ascending {
		t.Errorf("expected ORDER BY count DESC, got %v", q.OrderBy)
	}
	if q.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Limit)
	}
	if q.Offset != 5 {
		t.Errorf("expected OFFSET 5, got %d", q.Offset)
	}
}

func TestParser_GroupByMinMax(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType MIN(fileSize) MAX(created)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.GroupBy.Aggregates) != 2 {
		t.Fatalf("expected 2 aggregates, got %d", len(q.GroupBy.Aggregates))
	}
	if q.GroupBy.Aggregates[0].Name != "MIN" || q.GroupBy.Aggregates[0].Field.Name() != "fileSize" {
		t.Errorf("agg[0]: expected MIN(fileSize)")
	}
	if q.GroupBy.Aggregates[1].Name != "MAX" || q.GroupBy.Aggregates[1].Field.Name() != "created" {
		t.Errorf("agg[1]: expected MAX(created)")
	}
}

func TestParser_AggregateWithoutGroupByFails(t *testing.T) {
	_, err := Parse(`type = "resource" COUNT()`)
	if err == nil {
		t.Fatal("expected error for aggregate without GROUP BY")
	}
}

func TestParser_GroupByNoFields(t *testing.T) {
	_, err := Parse(`type = "resource" GROUP BY`)
	if err == nil {
		t.Fatal("expected error for GROUP BY without fields")
	}
}

func TestParser_SumWithoutFieldFails(t *testing.T) {
	_, err := Parse(`type = "resource" GROUP BY contentType SUM()`)
	if err == nil {
		t.Fatal("expected error: SUM requires a field argument")
	}
}

func TestParser_CountWithFieldFails(t *testing.T) {
	_, err := Parse(`type = "resource" GROUP BY contentType COUNT(fileSize)`)
	if err == nil {
		t.Fatal("expected error: COUNT does not take a field argument")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestParser_GroupBy|TestParser_Aggregate|TestParser_Sum|TestParser_Count' -v`
Expected: FAIL — parser doesn't handle GROUP BY yet.

- [ ] **Step 3: Implement GROUP BY parsing in `parser.go`**

In `parseQuery()`, between the WHERE expression parsing and the ORDER BY parsing, add GROUP BY and aggregate parsing. Also update the initial peek to recognize `TokenGroupBy`:

Replace the WHERE guard condition (line ~45):

```go
if tok.Type != TokenEOF && tok.Type != TokenOrderBy && tok.Type != TokenLimit && tok.Type != TokenOffset && tok.Type != TokenGroupBy {
```

After the WHERE expression block and before `// Optional ORDER BY`, add:

```go
// Optional GROUP BY
if p.lexer.Peek().Type == TokenGroupBy {
	groupBy, err := p.parseGroupBy()
	if err != nil {
		return nil, err
	}
	q.GroupBy = groupBy
}
```

Then add these new methods:

```go
// isAggregateToken returns true if the token is an aggregate function keyword.
func isAggregateToken(tt TokenType) bool {
	switch tt {
	case TokenCount, TokenSum, TokenAvg, TokenMin, TokenMax:
		return true
	}
	return false
}

// parseGroupBy = "GROUP BY" field ("," field)* [aggregates]
func (p *parser) parseGroupBy() (*GroupByClause, error) {
	p.lexer.Next() // consume GROUP BY

	clause := &GroupByClause{}

	// Parse first field
	field, err := p.parseField()
	if err != nil {
		return nil, err
	}
	clause.Fields = append(clause.Fields, field)

	// Parse additional comma-separated fields
	for p.lexer.Peek().Type == TokenComma {
		p.lexer.Next() // consume ','
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}
		clause.Fields = append(clause.Fields, field)
	}

	// Parse optional aggregate functions
	for isAggregateToken(p.lexer.Peek().Type) {
		agg, err := p.parseAggregateFunc()
		if err != nil {
			return nil, err
		}
		clause.Aggregates = append(clause.Aggregates, agg)
	}

	return clause, nil
}

// parseAggregateFunc = ("COUNT" "(" ")" | ("SUM"|"AVG"|"MIN"|"MAX") "(" field ")")
func (p *parser) parseAggregateFunc() (AggregateFunc, error) {
	tok := p.lexer.Next() // consume aggregate keyword (COUNT, SUM, etc.)
	name := strings.ToUpper(tok.Value)

	lp := p.lexer.Next() // consume '('
	if lp.Type != TokenLParen {
		return AggregateFunc{}, &ParseError{
			Message: fmt.Sprintf("expected '(' after %s, got %q", name, lp.Value),
			Pos:     lp.Pos,
			Length:  lp.Length,
		}
	}

	agg := AggregateFunc{Token: tok, Name: name}

	if name == "COUNT" {
		// COUNT() takes no arguments
		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return AggregateFunc{}, &ParseError{
				Message: fmt.Sprintf("COUNT() takes no arguments; expected ')', got %q", rp.Value),
				Pos:     rp.Pos,
				Length:  rp.Length,
			}
		}
	} else {
		// SUM, AVG, MIN, MAX require a field argument
		if p.lexer.Peek().Type == TokenRParen {
			return AggregateFunc{}, &ParseError{
				Message: fmt.Sprintf("%s() requires a field argument", name),
				Pos:     p.lexer.Peek().Pos,
				Length:  1,
			}
		}
		field, err := p.parseField()
		if err != nil {
			return AggregateFunc{}, err
		}
		agg.Field = field

		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return AggregateFunc{}, &ParseError{
				Message: fmt.Sprintf("expected ')' after %s field argument, got %q", name, rp.Value),
				Pos:     rp.Pos,
				Length:  rp.Length,
			}
		}
	}

	return agg, nil
}
```

Also handle the error case: an aggregate token appearing without GROUP BY. In `parseQuery()`, after the WHERE expression block but before GROUP BY, add detection for bare aggregates. Actually, this is best handled in the EOF check — if the parser sees an aggregate keyword at the end, it's because there's no GROUP BY. The existing "unexpected token" error at EOF will catch this, but let's improve the message. In the final EOF check block:

```go
final := p.lexer.Peek()
if final.Type != TokenEOF {
	if isAggregateToken(final.Type) {
		return nil, &ParseError{
			Message: fmt.Sprintf("aggregate function %s requires a preceding GROUP BY clause", final.Value),
			Pos:     final.Pos,
			Length:  final.Length,
		}
	}
	return nil, &ParseError{
		Message: fmt.Sprintf("unexpected token %q at end of query", final.Value),
		Pos:     final.Pos,
		Length:  final.Length,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestParser_GroupBy|TestParser_Aggregate|TestParser_Sum|TestParser_Count' -v`
Expected: PASS

- [ ] **Step 5: Run full parser test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestParser -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/parser.go mrql/parser_test.go
git commit -m "feat(mrql): parse GROUP BY clause with aggregate functions"
```

---

### Task 4: Validator

**Files:**
- Modify: `mrql/validator.go`
- Test: `mrql/validator_test.go`

- [ ] **Step 1: Write failing validator tests**

Add to `mrql/validator_test.go`:

```go
func TestValidate_GroupByRequiresEntityType(t *testing.T) {
	q, err := Parse(`name ~ "test" GROUP BY name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error: GROUP BY requires entity type")
	}
	if !strings.Contains(err.Error(), "GROUP BY requires an explicit entity type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupByValidScalarField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByMetaField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY meta.source COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByRelationField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY tags COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByRejectsTraversal(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY owner.name COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error for traversal in GROUP BY")
	}
	if !strings.Contains(err.Error(), "traversal") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupByRejectsUnknownField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY fakeField COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error for unknown field")
	}
}

func TestValidate_SumRequiresNumericField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(name)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error: SUM on string field")
	}
	if !strings.Contains(err.Error(), "numeric") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_AvgRequiresNumericField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType AVG(description)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error: AVG on string field")
	}
}

func TestValidate_MinAllowsDateTimeField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType MIN(created)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (MIN on datetime), got: %v", err)
	}
}

func TestValidate_MaxAllowsNumberField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType MAX(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (MAX on number), got: %v", err)
	}
}

func TestValidate_SumAllowsMetaField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(meta.size)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (SUM on meta), got: %v", err)
	}
}

func TestValidate_AggregateFieldMustExist(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(bogus)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error for unknown aggregate field")
	}
}

func TestValidate_GroupByOrderByAggregateKey(t *testing.T) {
	// In aggregated mode, ORDER BY can reference output keys like "count"
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid ORDER BY on aggregate key, got: %v", err)
	}
}

func TestValidate_GroupByOrderByGroupField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY contentType ASC`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid ORDER BY on group field, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestValidate_GroupBy|TestValidate_Sum|TestValidate_Avg|TestValidate_Min|TestValidate_Max|TestValidate_Aggregate' -v`
Expected: FAIL

- [ ] **Step 3: Implement GROUP BY validation in `validator.go`**

In the `Validate()` function, after the ORDER BY validation block, add GROUP BY validation:

```go
// Validate GROUP BY clause
if q.GroupBy != nil {
	if err := validateGroupBy(q.GroupBy, entityType, q.OrderBy); err != nil {
		return err
	}
}
```

Add the validation functions:

```go
// validateGroupBy validates the GROUP BY clause: entity type required, field
// types, no traversals, aggregate field type constraints, ORDER BY interaction.
func validateGroupBy(gb *GroupByClause, entityType EntityType, orderBy []OrderByClause) error {
	if entityType == EntityUnspecified {
		pos := 0
		if len(gb.Fields) > 0 {
			pos = gb.Fields[0].Pos()
		}
		return &ValidationError{
			Message: "GROUP BY requires an explicit entity type (e.g. type = \"resource\")",
			Pos:     pos,
			Length:  0,
		}
	}

	// Validate each GROUP BY field
	for _, f := range gb.Fields {
		// Reject traversal paths (multi-part fields that aren't meta.*)
		if len(f.Parts) >= 2 {
			prefix := f.Parts[0].Value
			if prefix != "meta" {
				return &ValidationError{
					Message: fmt.Sprintf("GROUP BY does not support traversal paths; use a direct field like %q instead of %q", prefix, f.Name()),
					Pos:     f.Pos(),
					Length:  len(f.Name()),
				}
			}
		}

		// Validate field exists for entity type
		if err := validateFieldExpr(f, entityType); err != nil {
			return err
		}
	}

	// Validate aggregate functions
	for _, agg := range gb.Aggregates {
		if agg.Field != nil {
			// Validate the field exists
			if err := validateFieldExpr(agg.Field, entityType); err != nil {
				return err
			}

			fieldName := agg.Field.Name()
			fd, ok := LookupField(entityType, fieldName)
			if !ok {
				// Meta fields are always ok
				if !strings.HasPrefix(fieldName, "meta.") {
					return &ValidationError{
						Message: fmt.Sprintf("unknown field %q for aggregate %s", fieldName, agg.Name),
						Pos:     agg.Field.Pos(),
						Length:  len(fieldName),
					}
				}
			} else {
				// SUM/AVG require numeric fields
				if agg.Name == "SUM" || agg.Name == "AVG" {
					if fd.Type != FieldNumber && fd.Type != FieldMeta {
						return &ValidationError{
							Message: fmt.Sprintf("%s requires a numeric field, but %q is %s", agg.Name, fieldName, fieldTypeName(fd.Type)),
							Pos:     agg.Field.Pos(),
							Length:  len(fieldName),
						}
					}
				}
				// MIN/MAX allow numeric and datetime
				if agg.Name == "MIN" || agg.Name == "MAX" {
					if fd.Type != FieldNumber && fd.Type != FieldDateTime && fd.Type != FieldMeta {
						return &ValidationError{
							Message: fmt.Sprintf("%s requires a numeric or datetime field, but %q is %s", agg.Name, fieldName, fieldTypeName(fd.Type)),
							Pos:     agg.Field.Pos(),
							Length:  len(fieldName),
						}
					}
				}
			}
		}
	}

	// Validate ORDER BY interaction in aggregated mode
	if len(gb.Aggregates) > 0 && len(orderBy) > 0 {
		validOrderKeys := buildAggregateOrderKeys(gb)
		for _, ob := range orderBy {
			obName := ob.Field.Name()
			if !validOrderKeys[obName] {
				return &ValidationError{
					Message: fmt.Sprintf("ORDER BY %q is not valid in aggregated GROUP BY; use a group-by field or aggregate key (e.g. count, sum_fileSize)", obName),
					Pos:     ob.Field.Pos(),
					Length:  len(obName),
				}
			}
		}
	}

	return nil
}

// buildAggregateOrderKeys returns the set of valid ORDER BY keys for an
// aggregated GROUP BY query: group field names + aggregate output keys.
func buildAggregateOrderKeys(gb *GroupByClause) map[string]bool {
	keys := make(map[string]bool)
	for _, f := range gb.Fields {
		keys[f.Name()] = true
	}
	for _, agg := range gb.Aggregates {
		if agg.Field == nil {
			keys["count"] = true
		} else {
			keys[strings.ToLower(agg.Name)+"_"+agg.Field.Name()] = true
		}
	}
	return keys
}

// fieldTypeName returns a human-readable name for a FieldType.
func fieldTypeName(ft FieldType) string {
	switch ft {
	case FieldString:
		return "a string field"
	case FieldNumber:
		return "a numeric field"
	case FieldDateTime:
		return "a datetime field"
	case FieldRelation:
		return "a relation field"
	case FieldMeta:
		return "a meta field"
	default:
		return "unknown"
	}
}
```

Also update the existing ORDER BY validation in `Validate()` to skip the standard `validateSortable` check when in aggregated GROUP BY mode (since ORDER BY can reference aggregate keys like "count" which aren't entity fields):

```go
// Validate ORDER BY fields — must be sortable (scalar or meta, not relation/traversal).
// In aggregated GROUP BY mode, ORDER BY is validated by validateGroupBy instead.
isAggregatedGroupBy := q.GroupBy != nil && len(q.GroupBy.Aggregates) > 0
for _, ob := range q.OrderBy {
	if !isAggregatedGroupBy {
		if err := validateFieldExpr(ob.Field, entityType); err != nil {
			return err
		}
		if err := validateSortable(ob.Field, entityType); err != nil {
			return err
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestValidate_GroupBy|TestValidate_Sum|TestValidate_Avg|TestValidate_Min|TestValidate_Max|TestValidate_Aggregate' -v`
Expected: PASS

- [ ] **Step 5: Run full validator and comprehensive test suites**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestValidat|TestComprehensive' -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/validator.go mrql/validator_test.go
git commit -m "feat(mrql): validate GROUP BY fields and aggregate constraints"
```

---

### Task 5: Translator — Aggregated Mode

**Files:**
- Modify: `mrql/translator.go`
- Test: `mrql/translator_test.go`

- [ ] **Step 1: Write failing translator tests for aggregated mode**

Add to `mrql/translator_test.go`:

```go
func TestTranslate_GroupByAggregated_Simple(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected mode 'aggregated', got %q", result.Mode)
	}
	if result.Rows == nil {
		t.Fatal("expected Rows to be set")
	}
}

func TestTranslate_GroupByAggregated_WithFilter(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" AND fileSize > 0 GROUP BY contentType COUNT() SUM(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected mode 'aggregated', got %q", result.Mode)
	}
	// Check that rows have expected keys
	for _, row := range result.Rows {
		if _, ok := row["contentType"]; !ok {
			t.Error("expected 'contentType' key in row")
		}
		if _, ok := row["count"]; !ok {
			t.Error("expected 'count' key in row")
		}
		if _, ok := row["sum_fileSize"]; !ok {
			t.Error("expected 'sum_fileSize' key in row")
		}
	}
}

func TestTranslate_GroupByAggregated_Meta(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY meta.source COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected mode 'aggregated', got %q", result.Mode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestTranslate_GroupBy' -v`
Expected: FAIL — `TranslateGroupBy` doesn't exist.

- [ ] **Step 3: Implement `TranslateGroupBy` in `translator.go`**

Add a new public function and the `GroupByResult` type:

```go
// GroupByResult holds the result of a GROUP BY query.
type GroupByResult struct {
	Mode string           `json:"mode"` // "aggregated" or "bucketed"
	Rows []map[string]any `json:"rows,omitempty"`
}

// TranslateGroupBy translates and executes a GROUP BY query.
// For aggregated mode (aggregates present), it returns flat rows.
// For bucketed mode (no aggregates), it returns nil — the caller handles bucketing.
func TranslateGroupBy(q *Query, db *gorm.DB) (*GroupByResult, error) {
	if q.GroupBy == nil {
		return nil, &TranslateError{Message: "TranslateGroupBy called without GROUP BY clause", Pos: 0}
	}

	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	result := db.Table(tc.tableName)

	// Apply WHERE clause
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	if len(q.GroupBy.Aggregates) > 0 {
		return tc.translateAggregatedGroupBy(result, q)
	}

	// Bucketed mode — return nil to signal caller should handle it
	return nil, nil
}

// translateAggregatedGroupBy builds SELECT ... GROUP BY ... and executes.
func (tc *translateContext) translateAggregatedGroupBy(db *gorm.DB, q *Query) (*GroupByResult, error) {
	// Add JOINs for relation fields (tags, owner, groups) if used in GROUP BY
	var relationExprs map[string]string
	db, relationExprs = tc.groupByRelationJoins(db, q.GroupBy.Fields)

	var selectCols []string
	var groupCols []string

	// Build SELECT and GROUP BY column lists
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		// Check if this field has a relation-based expression
		if relExpr, ok := relationExprs[fieldName]; ok {
			selectCols = append(selectCols, relExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, relExpr)
		} else {
			selectExpr, groupExpr := tc.groupByFieldExprs(fieldName)
			selectCols = append(selectCols, selectExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, groupExpr)
		}
	}

	// Build aggregate SELECT expressions
	for _, agg := range q.GroupBy.Aggregates {
		selectExpr, alias := tc.aggregateExpr(agg)
		selectCols = append(selectCols, selectExpr+` AS "`+alias+`"`)
	}

	db = db.Select(strings.Join(selectCols, ", "))

	for _, gc := range groupCols {
		db = db.Group(gc)
	}

	// ORDER BY
	for _, ob := range q.OrderBy {
		obName := ob.Field.Name()
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		// Check if it's an aggregate key or a group field
		db = db.Order(`"` + obName + `" ` + direction)
	}

	// LIMIT / OFFSET
	if q.Limit >= 0 {
		db = db.Limit(q.Limit)
	}
	if q.Offset >= 0 {
		db = db.Offset(q.Offset)
	}

	var rows []map[string]any
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}

	return &GroupByResult{
		Mode: "aggregated",
		Rows: rows,
	}, nil
}

// groupByFieldExprs returns the SELECT expression and GROUP BY expression for a field.
// For relation fields (tags, owner), this also sets up the necessary JOINs on the db.
func (tc *translateContext) groupByFieldExprs(fieldName string) (string, string) {
	// Meta fields
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			expr := fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			return expr, expr
		}
		expr := fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		return expr, expr
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		return tc.tableName + "." + fieldName, tc.tableName + "." + fieldName
	}

	col := tc.qualifiedColumn(fd.Column)
	return col, col
}

// groupByRelationJoins modifies the db query to add JOINs for relation fields
// used in GROUP BY. Returns the db and a map of fieldName → select/group expression.
func (tc *translateContext) groupByRelationJoins(db *gorm.DB, fields []*FieldExpr) (*gorm.DB, map[string]string) {
	exprMap := make(map[string]string)
	for _, f := range fields {
		fieldName := f.Name()
		fd, ok := LookupField(tc.entityType, fieldName)
		if !ok || fd.Type != FieldRelation {
			continue
		}

		switch fd.Column {
		case "tags":
			var junctionTable, entityCol string
			switch tc.entityType {
			case EntityResource:
				junctionTable = "resource_tags"
				entityCol = "resource_id"
			case EntityNote:
				junctionTable = "note_tags"
				entityCol = "note_id"
			case EntityGroup:
				junctionTable = "group_tags"
				entityCol = "group_id"
			}
			db = db.Joins(fmt.Sprintf("LEFT JOIN %s _gb_jt ON _gb_jt.%s = %s.id", junctionTable, entityCol, tc.tableName))
			db = db.Joins("LEFT JOIN tags _gb_t ON _gb_t.id = _gb_jt.tag_id")
			exprMap[fieldName] = "_gb_t.name"

		case "owner_id":
			db = db.Joins(fmt.Sprintf("LEFT JOIN groups _gb_owner ON _gb_owner.id = %s.owner_id", tc.tableName))
			exprMap[fieldName] = "_gb_owner.name"

		case "groups":
			var junctionTable, entityCol string
			switch tc.entityType {
			case EntityResource:
				junctionTable = "groups_related_resources"
				entityCol = "resource_id"
			case EntityNote:
				junctionTable = "groups_related_notes"
				entityCol = "note_id"
			}
			db = db.Joins(fmt.Sprintf("LEFT JOIN %s _gb_grp_jt ON _gb_grp_jt.%s = %s.id", junctionTable, entityCol, tc.tableName))
			db = db.Joins("LEFT JOIN groups _gb_g ON _gb_g.id = _gb_grp_jt.group_id")
			exprMap[fieldName] = "_gb_g.name"
		}
	}
	return db, exprMap
}

// aggregateExpr returns the SQL aggregate expression and the output alias.
func (tc *translateContext) aggregateExpr(agg AggregateFunc) (string, string) {
	switch agg.Name {
	case "COUNT":
		return "COUNT(*)", "count"
	default:
		fieldName := agg.Field.Name()
		col := tc.resolveAggregateColumn(fieldName)
		alias := strings.ToLower(agg.Name) + "_" + fieldName
		return fmt.Sprintf("%s(%s)", agg.Name, col), alias
	}
}

// resolveAggregateColumn converts a field name to its SQL column expression.
func (tc *translateContext) resolveAggregateColumn(fieldName string) string {
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			return fmt.Sprintf("(%s.meta->>'%s')::numeric", tc.tableName, key)
		}
		return fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		return tc.tableName + "." + fieldName
	}
	return tc.qualifiedColumn(fd.Column)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestTranslate_GroupBy' -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/translator.go mrql/translator_test.go
git commit -m "feat(mrql): translate aggregated GROUP BY queries to SQL"
```

---

### Task 6: Translator — Bucketed Mode

**Files:**
- Create: `mrql/translator_groupby.go`
- Test: `mrql/translator_test.go`

- [ ] **Step 1: Write failing translator tests for bucketed mode**

Add to `mrql/translator_test.go`:

```go
func TestTranslate_GroupByBucketed_Simple(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource
	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("translate keys: %v", err)
	}
	if keys == nil {
		t.Fatal("expected non-nil keys")
	}
	// keys should be a slice of maps
	for _, key := range keys {
		if _, ok := key["contentType"]; !ok {
			t.Error("expected 'contentType' in key")
		}
	}
}

func TestTranslate_GroupByBucketed_ItemsQuery(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource
	// Get a bucket query for a specific key
	bucketDB, err := TranslateGroupByBucket(q, db, map[string]any{"contentType": "image/png"})
	if err != nil {
		t.Fatalf("translate bucket: %v", err)
	}
	if bucketDB == nil {
		t.Fatal("expected non-nil DB")
	}
	// Should be able to Find resources
	var resources []testResource
	if err := bucketDB.Find(&resources).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestTranslate_GroupByBucketed' -v`
Expected: FAIL — `TranslateGroupByKeys` and `TranslateGroupByBucket` don't exist.

- [ ] **Step 3: Create `mrql/translator_groupby.go`**

```go
package mrql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// maxBuckets is the maximum number of distinct group keys allowed in bucketed mode.
const maxBuckets = 1000

// TranslateGroupByKeys executes a SELECT DISTINCT query to get unique bucket keys.
// Returns a slice of maps, each containing the group-by field values for one bucket.
func TranslateGroupByKeys(q *Query, db *gorm.DB) ([]map[string]any, error) {
	if q.GroupBy == nil || len(q.GroupBy.Aggregates) > 0 {
		return nil, &TranslateError{Message: "TranslateGroupByKeys requires GROUP BY without aggregates", Pos: 0}
	}

	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	result := db.Table(tc.tableName)

	// Apply WHERE clause
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Add JOINs for relation fields
	var relationExprs map[string]string
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Build SELECT DISTINCT for group-by fields
	var selectCols []string
	var groupCols []string
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		if relExpr, ok := relationExprs[fieldName]; ok {
			selectCols = append(selectCols, relExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, relExpr)
		} else {
			selectExpr, groupExpr := tc.groupByFieldExprs(fieldName)
			selectCols = append(selectCols, selectExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, groupExpr)
		}
	}

	result = result.Select(strings.Join(selectCols, ", "))
	for _, gc := range groupCols {
		result = result.Group(gc)
	}

	// ORDER BY for keys
	for _, ob := range q.OrderBy {
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		result = result.Order(`"` + ob.Field.Name() + `" ` + direction)
	}

	// Cap buckets
	result = result.Limit(maxBuckets)

	var keys []map[string]any
	if err := result.Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// TranslateGroupByBucket returns a GORM DB scoped to a specific bucket.
// The key map contains group-by field names mapped to their values.
// The caller should use the returned DB to Find entities and apply LIMIT.
func TranslateGroupByBucket(q *Query, db *gorm.DB, key map[string]any) (*gorm.DB, error) {
	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	result := db.Table(tc.tableName)

	// Apply WHERE clause from the original query
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Add JOINs for relation fields
	var relationExprs map[string]string
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Add bucket key constraints
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		val := key[fieldName]
		var expr string
		if relExpr, ok := relationExprs[fieldName]; ok {
			expr = relExpr
		} else {
			_, expr = tc.groupByFieldExprs(fieldName)
		}

		if val == nil {
			result = result.Where(expr + " IS NULL")
		} else {
			result = result.Where(expr+" = ?", val)
		}
	}

	// Apply per-bucket LIMIT
	if q.Limit >= 0 {
		result = result.Limit(q.Limit)
	}

	// Apply ORDER BY (within bucket)
	for _, ob := range q.OrderBy {
		col := tc.resolveOrderByColumn(ob.Field)
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		result = result.Order(col + " " + direction)
	}

	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestTranslate_GroupByBucketed' -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/translator_groupby.go mrql/translator_test.go
git commit -m "feat(mrql): implement bucketed GROUP BY with keys and per-bucket queries"
```

---

### Task 7: Execution Layer

**Files:**
- Modify: `application_context/mrql_context.go`

- [ ] **Step 1: Add result types and execution methods**

Add the new result types near the existing `MRQLResult`:

```go
// MRQLGroupedResult holds the results of a GROUP BY MRQL query.
type MRQLGroupedResult struct {
	EntityType string           `json:"entityType"`
	Mode       string           `json:"mode"` // "aggregated" or "bucketed"
	Rows       []map[string]any `json:"rows,omitempty"`
	Groups     []MRQLBucket     `json:"groups,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
}

// MRQLBucket is a single group of entities in bucketed mode.
type MRQLBucket struct {
	Key   map[string]any `json:"key"`
	Items any            `json:"items"` // []models.Resource, []models.Note, or []models.Group
}
```

- [ ] **Step 2: Modify `ExecuteMRQL` to branch on GROUP BY**

In `ExecuteMRQL()`, after validation and entity type extraction, add the GROUP BY branch before the existing single/cross-entity paths:

```go
entityType := mrql.ExtractEntityType(parsed)

// GROUP BY queries use a separate execution path
if parsed.GroupBy != nil {
	if entityType == mrql.EntityUnspecified {
		return nil, nil, errors.New("GROUP BY requires an explicit entity type")
	}
	parsed.EntityType = entityType
	grouped, err := ctx.executeGroupedQuery(reqCtx, parsed)
	return nil, grouped, err
}
```

The `ExecuteMRQL` return signature needs to change. Instead, add a new method and have the handler check:

Actually, to minimize API change, return `interface{}` or add a separate method. The cleanest approach: add `ExecuteMRQLGrouped`:

```go
// ExecuteMRQLGrouped executes a GROUP BY MRQL query and returns grouped results.
func (ctx *MahresourcesContext) ExecuteMRQLGrouped(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	if len(parsed.GroupBy.Aggregates) > 0 {
		return ctx.executeAggregatedQuery(queryCtx, parsed)
	}
	return ctx.executeBucketedQuery(queryCtx, parsed)
}

func (ctx *MahresourcesContext) executeAggregatedQuery(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)
	gbResult, err := mrql.TranslateGroupBy(parsed, db)
	if err != nil {
		return nil, err
	}

	// Apply default limit if not specified
	if gbResult.Rows == nil {
		gbResult.Rows = []map[string]any{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       gbResult.Mode,
		Rows:       gbResult.Rows,
	}, nil
}

func (ctx *MahresourcesContext) executeBucketedQuery(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)

	keys, err := mrql.TranslateGroupByKeys(parsed, db)
	if err != nil {
		return nil, err
	}

	var buckets []MRQLBucket
	for _, key := range keys {
		bucketDB, err := mrql.TranslateGroupByBucket(parsed, ctx.db.WithContext(reqCtx), key)
		if err != nil {
			return nil, err
		}

		bucket := MRQLBucket{Key: key}

		switch parsed.EntityType {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := bucketDB.Find(&resources).Error; err != nil {
				return nil, err
			}
			bucket.Items = resources
		case mrql.EntityNote:
			var notes []models.Note
			if err := bucketDB.Find(&notes).Error; err != nil {
				return nil, err
			}
			bucket.Items = notes
		case mrql.EntityGroup:
			var groups []models.Group
			if err := bucketDB.Find(&groups).Error; err != nil {
				return nil, err
			}
			bucket.Items = groups
		}

		buckets = append(buckets, bucket)
	}

	if buckets == nil {
		buckets = []MRQLBucket{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       "bucketed",
		Groups:     buckets,
	}, nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5' ./...`
Expected: Compiles

- [ ] **Step 4: Commit**

```bash
git add application_context/mrql_context.go
git commit -m "feat(mrql): add grouped query execution (aggregated + bucketed)"
```

---

### Task 8: API Handler

**Files:**
- Modify: `server/api_handlers/mrql_api_handlers.go`

- [ ] **Step 1: Modify `GetExecuteMRQLHandler` to handle GROUP BY**

Update the handler to detect GROUP BY and return the appropriate response type:

```go
func GetExecuteMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlExecuteRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if req.Query == "" {
			http_utils.HandleError(errors.New("query is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Parse and validate to check for GROUP BY before execution
		parsed, err := mrql.Parse(req.Query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		if err := mrql.Validate(parsed); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		// Override parsed LIMIT/OFFSET with request parameters
		if req.Limit > 0 {
			parsed.Limit = req.Limit
		}
		if req.Page >= 1 {
			effectiveLimit := parsed.Limit
			if effectiveLimit < 0 {
				effectiveLimit = 1000 // defaultMRQLLimit
			}
			parsed.Offset = (req.Page - 1) * effectiveLimit
		}

		// GROUP BY queries use a separate path
		if parsed.GroupBy != nil {
			entityType := mrql.ExtractEntityType(parsed)
			if entityType == mrql.EntityUnspecified {
				http_utils.HandleError(errors.New("GROUP BY requires an explicit entity type"), writer, request, http.StatusBadRequest)
				return
			}
			parsed.EntityType = entityType

			grouped, err := ctx.ExecuteMRQLGrouped(request.Context(), parsed)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(grouped)
			return
		}

		// Non-grouped query: use the existing path
		result, err := ctx.ExecuteMRQL(request.Context(), req.Query, req.Limit, req.Page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}
```

Add import for `"mahresources/mrql"` if not already present.

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5' ./...`
Expected: Compiles

- [ ] **Step 3: Commit**

```bash
git add server/api_handlers/mrql_api_handlers.go
git commit -m "feat(mrql): return MRQLGroupedResult for GROUP BY queries in API"
```

---

### Task 9: CLI Output

**Files:**
- Modify: `cmd/mr/commands/mrql.go`

- [ ] **Step 1: Add grouped response types and rendering**

Add the grouped response type:

```go
// mrqlGroupedResponse matches the MRQLGroupedResult struct.
type mrqlGroupedResponse struct {
	EntityType string             `json:"entityType"`
	Mode       string             `json:"mode"`
	Rows       []map[string]any   `json:"rows,omitempty"`
	Groups     []mrqlBucket       `json:"groups,omitempty"`
}

type mrqlBucket struct {
	Key   map[string]any `json:"key"`
	Items json.RawMessage `json:"items"`
}
```

Update the `mrqlCmd.RunE` to detect grouped responses:

```go
var raw json.RawMessage
if err := c.Post("/v1/mrql", nil, body, &raw); err != nil {
	return err
}

// Try grouped response first (has "mode" field)
var grouped mrqlGroupedResponse
if err := json.Unmarshal(raw, &grouped); err == nil && grouped.Mode != "" {
	if grouped.Mode == "aggregated" {
		columns, rows := aggregatedToTable(grouped.Rows)
		output.Print(*opts, columns, rows, raw)
	} else {
		printBucketedOutput(*opts, grouped, raw)
	}
	return nil
}

// Fall back to standard response
var resp mrqlResponse
if err := json.Unmarshal(raw, &resp); err != nil {
	output.PrintSingle(*opts, nil, raw)
	return nil
}

columns := []string{"ID", "TYPE", "NAME", "CREATED"}
rows := mrqlResponseToRows(resp)
output.Print(*opts, columns, rows, raw)
return nil
```

Add the rendering helpers:

```go
// aggregatedToTable converts aggregated rows to table columns/rows.
func aggregatedToTable(rows []map[string]any) ([]string, [][]string) {
	if len(rows) == 0 {
		return nil, nil
	}

	// Collect column names from the first row (order preserved by map iteration in Go 1.12+... not guaranteed)
	// Use a stable order: sort keys
	var columns []string
	for k := range rows[0] {
		columns = append(columns, k)
	}
	sort.Strings(columns)

	var tableRows [][]string
	for _, row := range rows {
		var cells []string
		for _, col := range columns {
			cells = append(cells, fmt.Sprintf("%v", row[col]))
		}
		tableRows = append(tableRows, cells)
	}

	// Uppercase column headers
	var headers []string
	for _, c := range columns {
		headers = append(headers, strings.ToUpper(c))
	}

	return headers, tableRows
}

// printBucketedOutput renders bucketed results with headers per group.
func printBucketedOutput(opts output.Options, grouped mrqlGroupedResponse, raw json.RawMessage) {
	if opts.JSON {
		output.PrintSingle(opts, nil, raw)
		return
	}

	for _, bucket := range grouped.Groups {
		// Print bucket header
		var keyParts []string
		for k, v := range bucket.Key {
			keyParts = append(keyParts, fmt.Sprintf("%s=%v", k, v))
		}
		output.PrintMessage(fmt.Sprintf("--- %s ---", strings.Join(keyParts, ", ")))

		// Parse items as entities
		var entities []mrqlEntity
		if err := json.Unmarshal(bucket.Items, &entities); err == nil {
			columns := []string{"ID", "NAME", "CREATED"}
			var rows [][]string
			for _, e := range entities {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(e.ID), 10),
					output.Truncate(e.Name, 40),
					e.CreatedAt.Format(time.RFC3339),
				})
			}
			output.Print(opts, columns, rows, nil)
		}
	}
}
```

Add `"sort"` to imports.

- [ ] **Step 2: Also update `newMRQLRunCmd` with the same grouped response handling**

The `run` subcommand also executes queries. Apply the same grouped response detection:

```go
// In newMRQLRunCmd's RunE, replace the response handling with:
var grouped mrqlGroupedResponse
if err := json.Unmarshal(raw, &grouped); err == nil && grouped.Mode != "" {
	if grouped.Mode == "aggregated" {
		columns, rows := aggregatedToTable(grouped.Rows)
		output.Print(*opts, columns, rows, raw)
	} else {
		printBucketedOutput(*opts, grouped, raw)
	}
	return nil
}

var resp mrqlResponse
if err := json.Unmarshal(raw, &resp); err != nil {
	output.PrintSingle(*opts, nil, raw)
	return nil
}

columns := []string{"ID", "TYPE", "NAME", "CREATED"}
rows := mrqlResponseToRows(resp)
output.Print(*opts, columns, rows, raw)
return nil
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5' ./...`
Expected: Compiles

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/mrql.go
git commit -m "feat(mrql): render aggregated and bucketed GROUP BY output in CLI"
```

---

### Task 10: Autocompletion

**Files:**
- Modify: `mrql/completer.go`
- Test: `mrql/completer_test.go`

- [ ] **Step 1: Write failing completer tests**

Add to `mrql/completer_test.go`:

```go
func TestComplete_SuggestsGroupByAfterValue(t *testing.T) {
	suggestions := Complete(`type = "resource" `, 20)
	found := false
	for _, s := range suggestions {
		if s.Value == "GROUP BY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected GROUP BY in suggestions after value")
	}
}

func TestComplete_SuggestsFieldsAfterGroupBy(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY `, 27)
	found := false
	for _, s := range suggestions {
		if s.Value == "contentType" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected field suggestions after GROUP BY, got: %v", suggestions)
	}
}

func TestComplete_SuggestsAggregatesAfterGroupByField(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY contentType `, 39)
	foundCount := false
	foundSum := false
	for _, s := range suggestions {
		if s.Value == "COUNT()" { foundCount = true }
		if s.Value == "SUM()" { foundSum = true }
	}
	if !foundCount || !foundSum {
		t.Errorf("expected aggregate suggestions, got: %v", suggestions)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestComplete_Suggests' -v`
Expected: FAIL

- [ ] **Step 3: Update `completer.go`**

Add GROUP BY to `postValueKeywords`:

```go
var postValueKeywords = []Suggestion{
	{Value: "AND", Type: "keyword"},
	{Value: "OR", Type: "keyword"},
	{Value: "GROUP BY", Type: "keyword"},
	{Value: "ORDER BY", Type: "keyword"},
	{Value: "LIMIT", Type: "keyword"},
}
```

Add aggregate suggestions:

```go
// aggregateSuggestions are suggested after GROUP BY field(s).
var aggregateSuggestions = []Suggestion{
	{Value: "COUNT()", Type: "function", Label: "count rows"},
	{Value: "SUM()", Type: "function", Label: "sum of field"},
	{Value: "AVG()", Type: "function", Label: "average of field"},
	{Value: "MIN()", Type: "function", Label: "minimum value"},
	{Value: "MAX()", Type: "function", Label: "maximum value"},
}

// postAggregateKeywords are suggested after aggregate functions.
var postAggregateKeywords = []Suggestion{
	{Value: "COUNT()", Type: "function", Label: "count rows"},
	{Value: "SUM()", Type: "function", Label: "sum of field"},
	{Value: "AVG()", Type: "function", Label: "average of field"},
	{Value: "MIN()", Type: "function", Label: "minimum value"},
	{Value: "MAX()", Type: "function", Label: "maximum value"},
	{Value: "ORDER BY", Type: "keyword"},
	{Value: "LIMIT", Type: "keyword"},
}
```

In `suggestionsForContext()`, add handling for `TokenGroupBy` and aggregate tokens:

```go
// After GROUP BY — suggest fields
if last.Type == TokenGroupBy {
	return fieldSuggestions(entityType)
}

// After an aggregate token's closing paren or after a field following GROUP BY,
// detect the GROUP BY context by scanning backwards for TokenGroupBy.
```

The simplest approach: in the `TokenRParen` case and the identifier-with-space case, check if we're in a GROUP BY context by scanning backwards for `TokenGroupBy`:

In `suggestionsForContext`, add before the existing `TokenIdentifier` handling:

```go
// Check for GROUP BY context: if we see TokenGroupBy in the token stream
// and we're past the group-by fields, suggest aggregates.
if isInGroupByContext(tokens) {
	// After a field or comma in GROUP BY, suggest comma (more fields) or aggregates
	if last.Type == TokenIdentifier || last.Type == TokenKwType {
		if !cursorAtTokenEnd {
			// Field name complete — suggest aggregates + ORDER BY
			var suggs []Suggestion
			suggs = append(suggs, aggregateSuggestions...)
			suggs = append(suggs, Suggestion{Value: "ORDER BY", Type: "keyword"})
			suggs = append(suggs, Suggestion{Value: "LIMIT", Type: "keyword"})
			return suggs
		}
	}
	if last.Type == TokenRParen {
		return postAggregateKeywords
	}
}
```

Add the helper:

```go
// isInGroupByContext returns true if the token stream contains a GROUP BY keyword.
func isInGroupByContext(tokens []Token) bool {
	for _, t := range tokens {
		if t.Type == TokenGroupBy {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestComplete_Suggests' -v`
Expected: PASS

- [ ] **Step 5: Run full completer and comprehensive tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestComplete|TestComprehensive_Completer' -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/completer.go mrql/completer_test.go
git commit -m "feat(mrql): add GROUP BY and aggregate autocompletion"
```

---

### Task 11: Comprehensive End-to-End Tests

**Files:**
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write comprehensive GROUP BY tests with seeded data**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_GroupByAggregatedCount(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	if len(result.Rows) == 0 {
		t.Error("expected at least one aggregated row")
	}
	// Each row should have contentType and count
	for _, row := range result.Rows {
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' in aggregated row")
		}
	}
}

func TestComprehensive_GroupByAggregatedSumAvg(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	for _, row := range result.Rows {
		if _, ok := row["sum_fileSize"]; !ok {
			t.Error("missing 'sum_fileSize'")
		}
		if _, ok := row["avg_fileSize"]; !ok {
			t.Error("missing 'avg_fileSize'")
		}
	}
}

func TestComprehensive_GroupByAggregatedMeta(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY meta.source COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
}

func TestComprehensive_GroupByAggregatedWithFilter(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" AND fileSize > 0 GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	// All rows should have count > 0 since we filtered fileSize > 0
	for _, row := range result.Rows {
		count, ok := row["count"]
		if !ok {
			t.Error("missing count")
			continue
		}
		// count may be int64 from SQLite
		if fmt.Sprintf("%v", count) == "0" {
			t.Error("expected non-zero count after filter")
		}
	}
}

func TestComprehensive_GroupByAggregatedOrderByLimit(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC LIMIT 2`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(result.Rows) > 2 {
		t.Errorf("expected at most 2 rows, got %d", len(result.Rows))
	}
}

func TestComprehensive_GroupByBucketedSimple(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one bucket key")
	}

	// Fetch items for each bucket
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(resources) > 5 {
			t.Errorf("expected at most 5 per bucket, got %d", len(resources))
		}
	}
}

func TestComprehensive_GroupByMinMax(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType MIN(fileSize) MAX(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	for _, row := range result.Rows {
		if _, ok := row["min_fileSize"]; !ok {
			t.Error("missing min_fileSize")
		}
		if _, ok := row["max_fileSize"]; !ok {
			t.Error("missing max_fileSize")
		}
	}
}

func TestComprehensive_GroupByMultipleKeys(t *testing.T) {
	db := setupComprehensiveDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType, meta.source COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	for _, row := range result.Rows {
		if _, ok := row["contentType"]; !ok {
			t.Error("missing contentType")
		}
		// meta.source may be nil for some groups — just check the key exists
		if _, ok := row["meta.source"]; !ok {
			t.Error("missing meta.source key")
		}
	}
}
```

- [ ] **Step 2: Run comprehensive GROUP BY tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run 'TestComprehensive_GroupBy' -v -count=1`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -v -count=1`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add mrql/translator_comprehensive_test.go
git commit -m "test(mrql): add comprehensive GROUP BY end-to-end tests"
```

---

### Task 12: API Tests

**Files:**
- Modify: `server/api_tests/mrql_api_test.go`

- [ ] **Step 1: Write API tests for GROUP BY responses**

Add tests that exercise the `/v1/mrql` endpoint with GROUP BY queries and verify the response shape:

```go
func TestMRQL_GroupByAggregated(t *testing.T) {
	// Create test resources first if needed via API
	body := map[string]interface{}{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
	}
	resp := postJSON(t, "/v1/mrql", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result["mode"] != "aggregated" {
		t.Errorf("expected mode 'aggregated', got %v", result["mode"])
	}
	if result["entityType"] != "resource" {
		t.Errorf("expected entityType 'resource', got %v", result["entityType"])
	}
	if result["rows"] == nil {
		t.Error("expected 'rows' in response")
	}
}

func TestMRQL_GroupByBucketed(t *testing.T) {
	body := map[string]interface{}{
		"query": `type = "resource" GROUP BY contentType LIMIT 5`,
	}
	resp := postJSON(t, "/v1/mrql", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result["mode"] != "bucketed" {
		t.Errorf("expected mode 'bucketed', got %v", result["mode"])
	}
	if result["groups"] == nil {
		t.Error("expected 'groups' in response")
	}
}

func TestMRQL_GroupByWithoutEntityTypeFails(t *testing.T) {
	body := map[string]interface{}{
		"query": `name ~ "test" GROUP BY name COUNT()`,
	}
	resp := postJSON(t, "/v1/mrql", body)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected error for GROUP BY without entity type")
	}
}
```

Adapt the test helpers (`postJSON`, server setup) to match the patterns already used in `mrql_api_test.go`.

- [ ] **Step 2: Run API tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./server/api_tests/... -run 'TestMRQL_GroupBy' -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add server/api_tests/mrql_api_test.go
git commit -m "test(mrql): add API tests for GROUP BY responses"
```

---

### Task 13: Documentation

**Files:**
- Modify: `docs-site/docs/features/mrql.md`

- [ ] **Step 1: Add GROUP BY section to MRQL docs**

Add a new section after the existing "Ordering and Pagination" section (or at an appropriate location):

```markdown
### GROUP BY and Aggregation

Group results by field values with optional aggregate functions. GROUP BY requires an explicit entity type.

**Two modes:**

| Mode | Trigger | Returns |
|------|---------|---------|
| Aggregated | GROUP BY with aggregates | Flat rows with computed values |
| Bucketed | GROUP BY without aggregates | Entity rows organized into groups |

#### Aggregate Functions

| Function | Argument | Field types | Output key |
|----------|----------|-------------|------------|
| `COUNT()` | none | n/a | `count` |
| `SUM(field)` | required | numeric, meta | `sum_{field}` |
| `AVG(field)` | required | numeric, meta | `avg_{field}` |
| `MIN(field)` | required | numeric, datetime, meta | `min_{field}` |
| `MAX(field)` | required | numeric, datetime, meta | `max_{field}` |

#### Aggregated Mode Examples

```
type = "resource" GROUP BY contentType COUNT()
type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)
type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC
type = "resource" GROUP BY meta.source COUNT()
type = "note" GROUP BY owner, noteType COUNT()
type = "resource" AND fileSize > 10mb GROUP BY contentType MIN(fileSize) MAX(fileSize)
```

#### Bucketed Mode Examples

```
type = "resource" GROUP BY contentType LIMIT 5
type = "resource" GROUP BY meta.camera_model LIMIT 10
type = "note" GROUP BY owner ORDER BY name ASC LIMIT 3
```

In bucketed mode, LIMIT applies per bucket (max items per group).

#### ORDER BY with GROUP BY

- **Aggregated mode:** ORDER BY can reference group fields or aggregate keys (`count`, `sum_fileSize`, etc.)
- **Bucketed mode:** ORDER BY applies to items within each bucket

#### Constraints

- GROUP BY requires `type = "resource|note|group"` — cross-entity grouping is not supported
- Traversal paths (e.g., `owner.name`) are not supported in GROUP BY — use direct fields
- Maximum 1000 buckets in bucketed mode
```

- [ ] **Step 2: Verify docs build** (if applicable)

Run: `cd /Users/egecan/Code/mahresources/docs-site && npm run build` (if docs-site has a build command)

- [ ] **Step 3: Commit**

```bash
git add docs-site/docs/features/mrql.md
git commit -m "docs: add GROUP BY and aggregation section to MRQL docs"
```

---

### Task 14: Full Integration Test Run

- [ ] **Step 1: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./... -count=1`
Expected: All PASS

- [ ] **Step 2: Build the application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Run E2E browser tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All PASS

- [ ] **Step 4: Run E2E CLI tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:cli`
Expected: All PASS

- [ ] **Step 5: Run Postgres tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: All PASS

- [ ] **Step 6: Run Postgres E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:postgres`
Expected: All PASS
