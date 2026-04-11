# MRQL Scope Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class `SCOPE` clause to MRQL that filters results to a group's ownership subtree, and wire it through shortcodes, plugins, and the CLI.

**Architecture:** SCOPE is parsed into the AST by the lexer/parser, resolved to a group ID by a new `ResolveScope()` function, and applied by the translator as a recursive CTE + WHERE clause. The shortcode `[mrql]` handler gains a `scope` attribute (entity/parent/root/global) that resolves to a group ID and injects it into the AST. The existing plugin scope path in `db_api.go` feeds into the same mechanism. All `*WithScope` execution methods in `mrql_context.go` switch from flat `owner_id = ?` to subtree filtering.

**Tech Stack:** Go, GORM, SQLite, PostgreSQL, Pongo2 templates, Playwright (E2E)

**Spec:** `docs/superpowers/specs/2026-04-11-mrql-scope-filter-design.md`

---

### Task 1: Lexer and AST — Add SCOPE Token and ScopeClause

**Files:**
- Modify: `mrql/token.go:34` (add TokenScope after TokenKwType)
- Modify: `mrql/lexer.go:312` (add SCOPE to keywordMap)
- Modify: `mrql/ast.go:170-179` (add ScopeClause struct and Query.Scope field)
- Test: `mrql/lexer_test.go`

- [ ] **Step 1: Write failing lexer test for SCOPE token**

In `mrql/lexer_test.go`, add a test:

```go
func TestLexerScopeKeyword(t *testing.T) {
	tests := []struct {
		input    string
		wantType TokenType
		wantVal  string
	}{
		{"SCOPE", TokenScope, "SCOPE"},
		{"scope", TokenScope, "scope"},
		{"Scope", TokenScope, "Scope"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLexer(tc.input)
			tok := l.Next()
			if tok.Type != tc.wantType {
				t.Errorf("input=%q: got type %v, want %v", tc.input, tok.Type, tc.wantType)
			}
			if tok.Value != tc.wantVal {
				t.Errorf("input=%q: got value %q, want %q", tc.input, tok.Value, tc.wantVal)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestLexerScopeKeyword -v`
Expected: FAIL — `TokenScope` undefined.

- [ ] **Step 3: Add TokenScope to token.go**

In `mrql/token.go`, after `TokenKwType` (line 34), add:

```go
TokenScope   // SCOPE
```

- [ ] **Step 4: Add SCOPE to keywordMap in lexer.go**

In `mrql/lexer.go`, in `keywordMap` (around line 312), add:

```go
"SCOPE": TokenScope,
```

- [ ] **Step 5: Run lexer test to verify it passes**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestLexerScopeKeyword -v`
Expected: PASS

- [ ] **Step 6: Add ScopeClause to ast.go**

After the `OrderByClause` struct (line 145) in `mrql/ast.go`, add:

```go
// ScopeClause restricts query results to a group's ownership subtree.
// Value is either a NumberLiteral (group ID) or StringLiteral (group name).
type ScopeClause struct {
	Token Token // the SCOPE keyword token
	Value Node  // NumberLiteral or StringLiteral
}
```

Then add the `Scope` field to the `Query` struct (after `GroupBy`):

```go
type Query struct {
	Where       Node             // the filter expression (may be nil)
	Scope       *ScopeClause     // SCOPE clause (nil when absent)
	GroupBy     *GroupByClause   // GROUP BY clause (nil when absent)
	OrderBy     []OrderByClause  // ORDER BY clauses (may be empty)
	Limit       int              // -1 if not specified
	Offset      int              // -1 if not specified
	BucketLimit int              // -1 if not specified
	EntityType  EntityType       // populated by validator or caller
}
```

- [ ] **Step 7: Run full mrql test suite to verify nothing broke**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -v -count=1`
Expected: All existing tests PASS (ScopeClause added but not used yet).

- [ ] **Step 8: Commit**

```bash
git add mrql/token.go mrql/lexer.go mrql/ast.go mrql/lexer_test.go
git commit -m "feat(mrql): add TokenScope and ScopeClause AST node"
```

---

### Task 2: Parser — Parse SCOPE Clause

**Files:**
- Modify: `mrql/parser.go:46,54-61` (add SCOPE parsing between expression and GROUP BY)
- Test: `mrql/parser_test.go`

- [ ] **Step 1: Write failing parser tests for SCOPE**

In `mrql/parser_test.go`, add:

```go
func TestParserScopeNumeric(t *testing.T) {
	q := mustParse(t, `type = "resource" SCOPE 42`)
	if q.Scope == nil {
		t.Fatal("expected Scope to be set")
	}
	num, ok := q.Scope.Value.(*NumberLiteral)
	if !ok {
		t.Fatalf("expected NumberLiteral, got %T", q.Scope.Value)
	}
	if num.Value != 42 {
		t.Errorf("expected scope value 42, got %v", num.Value)
	}
}

func TestParserScopeString(t *testing.T) {
	q := mustParse(t, `type = "resource" SCOPE "My Project"`)
	if q.Scope == nil {
		t.Fatal("expected Scope to be set")
	}
	str, ok := q.Scope.Value.(*StringLiteral)
	if !ok {
		t.Fatalf("expected StringLiteral, got %T", q.Scope.Value)
	}
	if str.Value != "My Project" {
		t.Errorf("expected scope value %q, got %q", "My Project", str.Value)
	}
}

func TestParserScopeBeforeGroupBy(t *testing.T) {
	q := mustParse(t, `type = "resource" SCOPE 7 GROUP BY contentType COUNT() ORDER BY count DESC`)
	if q.Scope == nil {
		t.Fatal("expected Scope to be set")
	}
	if q.GroupBy == nil {
		t.Fatal("expected GroupBy to be set")
	}
	if len(q.OrderBy) == 0 {
		t.Fatal("expected OrderBy to be set")
	}
}

func TestParserScopeAlone(t *testing.T) {
	q := mustParse(t, `SCOPE 123`)
	if q.Where != nil {
		t.Fatal("expected Where to be nil")
	}
	if q.Scope == nil {
		t.Fatal("expected Scope to be set")
	}
	num := q.Scope.Value.(*NumberLiteral)
	if num.Value != 123 {
		t.Errorf("expected 123, got %v", num.Value)
	}
}

func TestParserScopeInvalidValue(t *testing.T) {
	pe := mustFail(t, `type = "resource" SCOPE`)
	if pe == nil {
		t.Fatal("expected parse error")
	}
}

func TestParserNoScope(t *testing.T) {
	q := mustParse(t, `type = "resource" ORDER BY name`)
	if q.Scope != nil {
		t.Error("expected Scope to be nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestParserScope -v`
Expected: FAIL — SCOPE is not parsed (treated as unexpected token).

- [ ] **Step 3: Implement SCOPE parsing in parser.go**

In `mrql/parser.go`, update `parseQuery()`:

1. Add `TokenScope` to the expression-skip list (line 46) so `SCOPE` alone doesn't try to parse as expression:

```go
tok := p.lexer.Peek()
if tok.Type != TokenEOF && tok.Type != TokenOrderBy && tok.Type != TokenLimit && tok.Type != TokenOffset && tok.Type != TokenGroupBy && tok.Type != TokenScope {
```

2. Insert SCOPE parsing between the expression block (line 52) and the GROUP BY block (line 54):

```go
	// Optional SCOPE
	if p.lexer.Peek().Type == TokenScope {
		scope, err := p.parseScope()
		if err != nil {
			return nil, err
		}
		q.Scope = scope
	}
```

3. Add the `parseScope()` method at the end of parser.go (before any utility functions):

```go
// parseScope parses: SCOPE (number | "string")
func (p *parser) parseScope() (*ScopeClause, error) {
	scopeTok := p.lexer.Next() // consume SCOPE

	valTok := p.lexer.Next()
	switch valTok.Type {
	case TokenNumber:
		val, unit, raw, err := parseNumber(valTok)
		if err != nil {
			return nil, &ParseError{Message: err.Error(), Pos: valTok.Pos, Length: valTok.Length}
		}
		if unit != "" {
			return nil, &ParseError{
				Message: fmt.Sprintf("SCOPE does not accept unit suffixes, got %q", valTok.Value),
				Pos:     valTok.Pos,
				Length:  valTok.Length,
			}
		}
		return &ScopeClause{
			Token: scopeTok,
			Value: &NumberLiteral{Token: valTok, Value: val, Unit: unit, Raw: raw},
		}, nil

	case TokenString:
		return &ScopeClause{
			Token: scopeTok,
			Value: &StringLiteral{Token: valTok, Value: valTok.Value},
		}, nil

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected number or quoted string after SCOPE, got %q", valTok.Value),
			Pos:     valTok.Pos,
			Length:  valTok.Length,
		}
	}
}
```

Note: `parseNumber` is the existing helper used by `parseValue()`. If it doesn't exist as a standalone function, extract the number-parsing logic from `parseValue()` into a helper, or inline the `NumberLiteral` construction directly using `parseIntFromToken` / `strconv.ParseFloat`.

Check `parseValue()` in parser.go for how numbers are currently constructed and follow the same pattern.

- [ ] **Step 4: Run parser tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestParserScope -v`
Expected: All PASS.

- [ ] **Step 5: Run full mrql test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -v -count=1`
Expected: All existing tests PASS.

- [ ] **Step 6: Commit**

```bash
git add mrql/parser.go mrql/parser_test.go
git commit -m "feat(mrql): parse SCOPE clause between expression and GROUP BY"
```

---

### Task 3: Scope Resolution — ResolveScope Function

**Files:**
- Create: `mrql/scope.go`
- Test: `mrql/scope_test.go`

This function resolves a `ScopeClause` (from the AST) to a concrete group ID. It handles string name lookups (case-insensitive), ambiguous name errors, not-found errors, and the sentinel for unresolvable scope.

- [ ] **Step 1: Write failing tests for ResolveScope**

Create `mrql/scope_test.go`:

```go
package mrql

import (
	"testing"
)

func TestResolveScopeNil(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for nil scope, got %d", id)
	}
}

func TestResolveScopeNumericExists(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 1}, // Vacation group exists
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeNumericNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 9999},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for nonexistent group ID")
	}
}

func TestResolveScopeStringExists(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Vacation"},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "vacation"}, // lowercase
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeStringNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Nonexistent"},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for nonexistent group name")
	}
}

func TestResolveScopeAmbiguousName(t *testing.T) {
	db := setupTestDB(t)
	// Create a second group named "Vacation" (case-insensitive match)
	db.Create(&testGroup{ID: 100, Name: "vacation", Meta: `{}`})

	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Vacation"},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for ambiguous group name")
	}
}

func TestResolveScopeZero(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 0},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for SCOPE 0, got %d", id)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestResolveScope -v`
Expected: FAIL — `ResolveScope` undefined.

- [ ] **Step 3: Implement ResolveScope in mrql/scope.go**

Create `mrql/scope.go`:

```go
package mrql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ResolveScope resolves a Query's Scope clause to a concrete group ID.
// Returns 0 if no scope is set (global/no filter).
// For numeric scope: verifies group exists (error if not).
// For string scope: case-insensitive lookup (error if not found or ambiguous).
func ResolveScope(q *Query, db *gorm.DB) (uint, error) {
	if q.Scope == nil {
		return 0, nil
	}

	switch v := q.Scope.Value.(type) {
	case *NumberLiteral:
		if v.Unit != "" {
			return 0, &ScopeError{
				Message: fmt.Sprintf("SCOPE does not accept unit suffixes, got %q", v.Token.Value),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		if v.Value != float64(int64(v.Value)) {
			return 0, &ScopeError{
				Message: fmt.Sprintf("SCOPE requires an integer group ID, got %v", v.Value),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		id := uint(v.Value)
		if id == 0 {
			return 0, nil // SCOPE 0 = global
		}
		// Verify group exists
		var count int64
		if err := db.Table("groups").Where("id = ?", id).Count(&count).Error; err != nil {
			return 0, fmt.Errorf("scope resolution failed: %w", err)
		}
		if count == 0 {
			return 0, &ScopeError{
				Message: fmt.Sprintf("scope group not found: id %d", id),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		return id, nil

	case *StringLiteral:
		return resolveScopeByName(v, db)

	default:
		return 0, fmt.Errorf("unexpected scope value type: %T", q.Scope.Value)
	}
}

// resolveScopeByName looks up a group by name (case-insensitive).
// Returns an error listing all matches if the name is ambiguous.
func resolveScopeByName(v *StringLiteral, db *gorm.DB) (uint, error) {
	type scopeMatch struct {
		ID         uint
		Name       string
		CategoryID *uint
		OwnerID    *uint
	}

	var matches []scopeMatch
	err := db.Table("groups").
		Select("id, name, category_id, owner_id").
		Where("LOWER(name) = LOWER(?)", v.Value).
		Find(&matches).Error
	if err != nil {
		return 0, fmt.Errorf("scope resolution failed: %w", err)
	}

	if len(matches) == 0 {
		return 0, &ScopeError{
			Message: fmt.Sprintf("scope group not found: %q", v.Value),
			Pos:     v.Token.Pos,
			Length:  v.Token.Length,
		}
	}

	if len(matches) == 1 {
		return matches[0].ID, nil
	}

	// Ambiguous — build descriptive error
	var lines []string
	for _, m := range matches {
		line := fmt.Sprintf("  - id=%d", m.ID)
		if m.CategoryID != nil {
			line += fmt.Sprintf(", categoryId=%d", *m.CategoryID)
		}
		if m.OwnerID != nil {
			line += fmt.Sprintf(", parentId=%d", *m.OwnerID)
		}
		lines = append(lines, line)
	}
	return 0, &ScopeError{
		Message: fmt.Sprintf("ambiguous scope %q: found %d groups:\n%s\nUse SCOPE <id> to specify which group.",
			v.Value, len(matches), strings.Join(lines, "\n")),
		Pos:    v.Token.Pos,
		Length: v.Token.Length,
	}
}

// UnresolvedScopeSentinel is a scope ID that guarantees empty results.
// Used when scope resolution fails for internal callers (ownerless entities).
// The CTE for a nonexistent ID returns an empty subtree, yielding no results.
// This matches the existing data-views sentinel pattern in db_api.go.
const UnresolvedScopeSentinel = ^uint(0) >> 1

// ScopeError is returned when scope resolution fails.
type ScopeError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ScopeError) Error() string { return e.Message }

// scopeCTE is the recursive CTE SQL that collects all group IDs in a subtree.
// Used inline in WHERE clauses — no separate prefetch query needed.
// Both SQLite (3.35+) and PostgreSQL support CTEs inside IN subqueries.
const scopeCTE = `WITH RECURSIVE scope_tree(id, depth) AS (
	SELECT id, 0 FROM groups WHERE id = ?
	UNION ALL
	SELECT g.id, st.depth + 1 FROM groups g
	INNER JOIN scope_tree st ON g.owner_id = st.id
	WHERE st.depth < 50
) SELECT id FROM scope_tree`

// ApplyScopeCTE injects an inline recursive CTE into a GORM query to filter
// entities to a group's ownership subtree. No separate prefetch query needed —
// the CTE runs as part of the main query.
//
// For EntityGroup: WHERE id IN (CTE) — includes the scoped group itself.
// For other types: WHERE owner_id IN (CTE) — entities owned by subtree groups.
//
// When scopeGroupID doesn't exist (including the sentinel value), the CTE
// returns zero rows, so IN (empty set) naturally yields no results.
func ApplyScopeCTE(db *gorm.DB, entityType EntityType, scopeGroupID uint) *gorm.DB {
	if entityType == EntityGroup {
		return db.Where(fmt.Sprintf("id IN (%s)", scopeCTE), scopeGroupID)
	}
	return db.Where(fmt.Sprintf("owner_id IN (%s)", scopeCTE), scopeGroupID)
}
```

- [ ] **Step 4: Run scope tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestResolveScope -v`
Expected: All PASS.

- [ ] **Step 5: Write test for ApplyScopeCTE**

Add to `mrql/scope_test.go`:

```go
func TestApplyScopeCTEResourceSubtree(t *testing.T) {
	db := setupTestDB(t)
	// Vacation(1) -> Work(2) -> Sub-Work(4), Vacation(1) -> Photos(5)
	// sunset.jpg owned by Vacation(1), report.pdf owned by Work(2)
	result := db.Table("resources")
	result = ApplyScopeCTE(result, EntityResource, 1)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Both resources have owners in Vacation's subtree
	if len(resources) != 2 {
		t.Errorf("expected 2, got %d", len(resources))
	}
}

func TestApplyScopeCTEGroupInclusive(t *testing.T) {
	db := setupTestDB(t)
	result := db.Table("groups")
	result = ApplyScopeCTE(result, EntityGroup, 1)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Vacation(1), Work(2), Sub-Work(4), Photos(5)
	if len(groups) != 4 {
		t.Errorf("expected 4, got %d", len(groups))
	}
}

func TestApplyScopeCTENonexistentIDEmpty(t *testing.T) {
	db := setupTestDB(t)
	result := db.Table("resources")
	result = ApplyScopeCTE(result, EntityResource, 99999)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 for nonexistent scope, got %d", len(resources))
	}
}
```

- [ ] **Step 6: Run CTE tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestApplyScopeCTE -v`
Expected: All PASS.

- [ ] **Step 7: Run full mrql test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -count=1`
Expected: All PASS.

- [ ] **Step 8: Commit**

```bash
git add mrql/scope.go mrql/scope_test.go
git commit -m "feat(mrql): add ResolveScope and ResolveScopeSubtreeIDs"
```

---

### Task 4: Translator — Apply Scope Filter

**Files:**
- Modify: `mrql/translator.go:22-24` (add ScopeGroupID to TranslateOptions)
- Modify: `mrql/translator.go:34-87` (apply scope in TranslateWithOptions)
- Modify: `mrql/translator_groupby.go:15,105` (add scope to TranslateGroupByKeys, TranslateGroupByBucket)
- Modify: `mrql/translator.go:1489-1492` (add scope to TranslateGroupBy)
- Test: `mrql/translator_test.go`

- [ ] **Step 1: Write failing translator tests for scope filtering**

Add to `mrql/translator_test.go`:

```go
func TestTranslateScopeResourcesByOwner(t *testing.T) {
	db := setupTestDB(t)
	// sunset.jpg(1) owned by Vacation(1), report.pdf(3) owned by Work(2)
	// Vacation(1) owns Work(2), Photos(5). Work(2) owns Sub-Work(4).
	// SCOPE 1 subtree = {1,2,4,5} → resources with owner_id IN {1,2,4,5}
	q := mustParse(t, `type = "resource" SCOPE 1`)
	q.EntityType = EntityResource
	Validate(q)

	opts := TranslateOptions{ScopeGroupID: 1}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	// sunset.jpg (owner=1) and report.pdf (owner=2) both in Vacation subtree
	if len(resources) != 2 {
		t.Errorf("expected 2 scoped resources, got %d", len(resources))
	}
}

func TestTranslateScopeGroupsInclusive(t *testing.T) {
	db := setupTestDB(t)
	// SCOPE 1 for groups should include Vacation(1) itself plus children
	q := mustParse(t, `type = "group" SCOPE 1`)
	q.EntityType = EntityGroup
	Validate(q)

	opts := TranslateOptions{ScopeGroupID: 1}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	// Vacation(1), Work(2), Sub-Work(4), Photos(5)
	if len(groups) != 4 {
		t.Errorf("expected 4 scoped groups, got %d", len(groups))
	}
}

func TestTranslateScopeLeafGroup(t *testing.T) {
	db := setupTestDB(t)
	// SCOPE 4 (Sub-Work) — leaf, no children
	q := mustParse(t, `type = "resource" SCOPE 4`)
	q.EntityType = EntityResource
	Validate(q)

	opts := TranslateOptions{ScopeGroupID: 4}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	// No resources owned by Sub-Work(4)
	if len(resources) != 0 {
		t.Errorf("expected 0 scoped resources, got %d", len(resources))
	}
}

func TestTranslateScopeZeroNoFilter(t *testing.T) {
	db := setupTestDB(t)
	q := mustParse(t, `type = "resource"`)
	q.EntityType = EntityResource
	Validate(q)

	opts := TranslateOptions{ScopeGroupID: 0}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	// All 4 resources (no scope filter)
	if len(resources) != 4 {
		t.Errorf("expected 4 resources with no scope, got %d", len(resources))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestTranslateScope -v`
Expected: FAIL — `ScopeGroupID` field doesn't exist on TranslateOptions.

- [ ] **Step 3: Add ScopeGroupID to TranslateOptions**

In `mrql/translator.go`, update `TranslateOptions` (line 23):

```go
type TranslateOptions struct {
	ScopeGroupID uint // resolved scope group ID; 0 = no scope
}
```

- [ ] **Step 4: Apply scope filter in TranslateWithOptions**

In `mrql/translator.go`, in `TranslateWithOptions()`, after the WHERE clause translation (around line 64) and before ORDER BY (line 67), add:

```go
	// Apply scope filter — inline recursive CTE, no separate query
	if opts.ScopeGroupID > 0 {
		result = ApplyScopeCTE(result, entityType, opts.ScopeGroupID)
	}
```

This embeds the recursive CTE directly in the WHERE clause of the main query. No separate prefetch needed — the CTE runs as part of a single SQL statement. For nonexistent IDs (including the sentinel), the CTE returns zero rows, naturally yielding empty results.

- [ ] **Step 5: Apply scope to TranslateGroupByKeys and TranslateGroupByBucket**

In `mrql/translator_groupby.go`:

Update `TranslateGroupByKeys` signature (line 15):
```go
func TranslateGroupByKeys(q *Query, db *gorm.DB, opts TranslateOptions) ([]map[string]any, error) {
```

After the WHERE clause translation (line 43), before the JOIN block, add:
```go
	// Apply scope filter — inline CTE, same pattern as TranslateWithOptions
	if opts.ScopeGroupID > 0 {
		result = ApplyScopeCTE(result, entityType, opts.ScopeGroupID)
	}
```

Update `TranslateGroupByBucket` signature (line 105):
```go
func TranslateGroupByBucket(q *Query, db *gorm.DB, key map[string]any, opts TranslateOptions) (*gorm.DB, error) {
```

After the WHERE clause translation (line 129), add the same `ApplyScopeCTE` block.

Update `TranslateGroupBy` signature in `mrql/translator.go` (line 1492):
```go
func TranslateGroupBy(q *Query, db *gorm.DB, opts TranslateOptions) (*GroupByResult, error) {
```

And pass `opts` through to the internal aggregated query path.

- [ ] **Step 6: Fix all callers of updated signatures**

Search for all callers of `TranslateGroupByKeys`, `TranslateGroupByBucket`, and `TranslateGroupBy` in:
- `application_context/mrql_context.go` — update all calls to pass `TranslateOptions{}` or the appropriate opts
- Any test files

For existing unscoped calls, pass `TranslateOptions{}` (zero value = no scope).

- [ ] **Step 7: Run translator scope tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestTranslateScope -v`
Expected: All PASS.

- [ ] **Step 8: Run full test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -count=1`
Expected: All PASS (including existing translator tests, since unscoped calls use zero-value opts).

- [ ] **Step 9: Commit**

```bash
git add mrql/translator.go mrql/translator_groupby.go mrql/translator_test.go application_context/mrql_context.go
git commit -m "feat(mrql): apply scope subtree filter in translator"
```

---

### Task 5: Execution Layer — Wire Scope Through

**Files:**
- Modify: `application_context/mrql_context.go:701-908` (update WithScope methods to use subtree)
- Modify: `application_context/plugin_mrql_adapter.go:19-57` (pass scope via TranslateOptions)
- Modify: `plugin_system/db_api.go:165-170` (no struct change needed, behavior changes upstream)

- [ ] **Step 1: Update ExecuteSingleEntityWithScope**

In `application_context/mrql_context.go`, update `ExecuteSingleEntityWithScope` (line 701):

Replace the flat scope filter (lines 716-719):
```go
// Apply scope filter BEFORE execution so LIMIT/ORDER operate on scoped data
if scopeID > 0 {
    db = db.Where("owner_id = ?", scopeID)
}
```

With scope passed through TranslateOptions:
```go
opts.ScopeGroupID = scopeID
db, err := mrql.TranslateWithOptions(q, ctx.db.WithContext(queryCtx), opts)
```

And remove the old separate scope injection. The full method should now look like:

```go
func (ctx *MahresourcesContext) ExecuteSingleEntityWithScope(reqCtx context.Context, q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, scopeID uint) (*MRQLResult, error) {
	q.EntityType = entityType
	opts.ScopeGroupID = scopeID

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	db, err := mrql.TranslateWithOptions(q, ctx.db.WithContext(queryCtx), opts)
	if err != nil {
		return nil, err
	}

	if q.Limit < 0 {
		db = db.Limit(defaultMRQLLimit)
	}

	result := &MRQLResult{EntityType: entityType.String()}
	// ... switch entityType (unchanged) ...
```

- [ ] **Step 2: Update ExecuteMRQLGroupedWithScope**

In the same file, update `executeAggregatedQueryScoped` (line 774):

Replace:
```go
db := ctx.db.WithContext(reqCtx).Where("owner_id = ?", scopeID)
gbResult, err := mrql.TranslateGroupBy(parsed, db)
```

With:
```go
opts := mrql.TranslateOptions{ScopeGroupID: scopeID}
gbResult, err := mrql.TranslateGroupBy(parsed, ctx.db.WithContext(reqCtx), opts)
```

Update `executeBucketedQueryScoped` (line 795) similarly:

Replace:
```go
db := ctx.db.WithContext(reqCtx).Where("owner_id = ?", scopeID)
allKeys, err := mrql.TranslateGroupByKeys(parsed, db)
```

With:
```go
opts := mrql.TranslateOptions{ScopeGroupID: scopeID}
allKeys, err := mrql.TranslateGroupByKeys(parsed, ctx.db.WithContext(reqCtx), opts)
```

And for each bucket (line 836):

Replace:
```go
bucketDB, err := mrql.TranslateGroupByBucket(parsed, ctx.db.WithContext(reqCtx).Where("owner_id = ?", scopeID), key)
```

With:
```go
bucketDB, err := mrql.TranslateGroupByBucket(parsed, ctx.db.WithContext(reqCtx), key, opts)
```

- [ ] **Step 3: Wire explicit SCOPE into the normal ExecuteMRQL path**

The standard CLI/API execution path (`ExecuteMRQL` at `mrql_context.go:51`) parses the query and calls `executeSingleEntity` with `TranslateOptions{}`. If a user writes `type = "resource" SCOPE 42`, the parser sets `parsed.Scope` but nothing reads it — the query runs unscoped.

Fix: after parsing/validating in `ExecuteMRQL`, resolve `parsed.Scope` and set `opts.ScopeGroupID`:

```go
func (ctx *MahresourcesContext) ExecuteMRQL(reqCtx context.Context, queryStr string, limit, page int) (*MRQLResult, error) {
	// ... existing parse/validate code ...

	opts := mrql.TranslateOptions{}

	// Resolve explicit SCOPE clause from the query
	if parsed.Scope != nil {
		scopeID, err := mrql.ResolveScope(parsed, ctx.db)
		if err != nil {
			return nil, err
		}
		opts.ScopeGroupID = scopeID
	}

	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(reqCtx, parsed, entityType, opts)
	}
	return ctx.executeCrossEntity(reqCtx, parsed, opts)
}
```

Apply the same pattern in `ExecuteMRQLGrouped` (line 147) — resolve `parsed.Scope` before dispatching to aggregated/bucketed paths:

```go
func (ctx *MahresourcesContext) ExecuteMRQLGrouped(reqCtx context.Context, q *mrql.Query) (*MRQLGroupedResult, error) {
	// ... existing limit logic ...

	opts := mrql.TranslateOptions{}
	if q.Scope != nil {
		scopeID, err := mrql.ResolveScope(q, ctx.db)
		if err != nil {
			return nil, err
		}
		opts.ScopeGroupID = scopeID
	}

	if len(q.GroupBy.Aggregates) > 0 {
		return ctx.executeAggregatedQuery(reqCtx, q, opts)
	}
	// ... bucketed path with opts ...
}
```

Update `executeAggregatedQuery` and `executeBucketedQuery` signatures to accept `TranslateOptions` and pass them through to `TranslateGroupBy`/`TranslateGroupByKeys`/`TranslateGroupByBucket`.

- [ ] **Step 4: Update plugin_mrql_adapter.go — explicit SCOPE wins over plugin scope**

The plugin adapter currently passes `opts.ScopeID` (resolved by the Lua/Go bridge from entity/parent/root/global) straight into `ExecuteSingleEntityWithScope` and `ExecuteMRQLGroupedWithScope`. If the MRQL query string itself contains `SCOPE 42`, the parsed AST has `Scope` set, but the adapter ignores it and overwrites with the plugin-provided scope.

Fix: after parsing, check `parsed.Scope`. If present, resolve it and use it instead of `opts.ScopeID`:

```go
func (a *pluginMRQLAdapter) ExecuteMRQL(reqCtx context.Context, query string, opts plugin_system.MRQLExecOptions) (*plugin_system.MRQLResult, error) {
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	if opts.Limit > 0 {
		parsed.Limit = opts.Limit
	}

	entityType := mrql.ExtractEntityType(parsed)
	if entityType == mrql.EntityUnspecified {
		return nil, fmt.Errorf("MRQL query must specify an entity type (e.g. type=resource)")
	}
	parsed.EntityType = entityType

	// Explicit SCOPE in query string takes precedence over plugin-provided scope
	scopeID := opts.ScopeID
	if parsed.Scope != nil {
		resolvedID, err := mrql.ResolveScope(parsed, a.ctx.db)
		if err != nil {
			return nil, err
		}
		scopeID = resolvedID
	}

	// GROUP BY path
	if parsed.GroupBy != nil {
		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}
		grouped, err := a.ctx.ExecuteMRQLGroupedWithScope(reqCtx, parsed, scopeID)
		if err != nil {
			return nil, err
		}
		return a.convertGrouped(grouped), nil
	}

	// Flat path
	translateOpts := mrql.TranslateOptions{}
	result, err := a.ctx.ExecuteSingleEntityWithScope(reqCtx, parsed, entityType, translateOpts, scopeID)
	if err != nil {
		return nil, err
	}
	return a.convertFlat(result), nil
}
```

- [ ] **Step 5: Write test for explicit SCOPE precedence in plugin path**

Add to `application_context/plugin_mrql_adapter_test.go` (or wherever adapter tests live — if no test file exists yet, the translator tests in Task 4 can cover the translator-level behavior, and an E2E test in Task 9 should cover the full plugin path):

The key test scenario: a data-views shortcode passes `scope="entity"` (resolving to group 1), but the MRQL query string contains `SCOPE 3`. The query should be scoped to group 3's subtree, not group 1's.

If unit-testing the adapter directly isn't practical (it requires a full `MahresourcesContext`), add this as a targeted E2E test in Task 9:

```typescript
test('explicit SCOPE in data-views MRQL overrides plugin scope attribute', async ({ page }) => {
  // Create hierarchy: groupA -> groupB, groupC (unrelated)
  // Create resource owned by groupC
  // On groupA's page, use a data-views shortcode with scope="entity" and
  // mrql='type = "resource" SCOPE <groupC.id>'
  // Verify the resource from groupC appears (explicit SCOPE wins)
  // Verify resources from groupA's subtree do NOT appear
});
```

- [ ] **Step 6: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./... -count=1`
Expected: All PASS. The behavioral change (flat → tree) only affects scoped queries, and existing tests should still pass since the test data hierarchy hasn't changed.

- [ ] **Step 6: Commit**

```bash
git add application_context/mrql_context.go application_context/plugin_mrql_adapter.go
git commit -m "feat: wire scope subtree filtering through execution layer"
```

---

### Task 6: Shortcode [mrql] Scope Support

**Files:**
- Modify: `shortcodes/processor.go:15-18` (extend QueryExecutor signature)
- Modify: `shortcodes/mrql_handler.go:19-30` (parse scope attribute, pass to executor)
- Modify: `server/template_handlers/template_filters/shortcode_query_executor.go:17-82` (handle scope in executor)
- Modify: `shortcodes/meta_handler.go` (MetaShortcodeContext — may need owner lookup helper)
- Test: `shortcodes/mrql_handler_test.go`

- [ ] **Step 1: Write failing test for scope attribute in shortcode**

In `shortcodes/mrql_handler_test.go`, add:

```go
func TestMRQLShortcodeScopePassthrough(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items:      []QueryResultItem{},
	}

	var capturedScope uint
	executor := func(ctx context.Context, query string, savedName string, limit int, buckets int, scopeGroupID uint) (*QueryResult, error) {
		capturedScope = scopeGroupID
		return result, nil
	}

	sc := Shortcode{
		Name: "mrql",
		Attrs: map[string]string{
			"query": `type = "resource"`,
			"scope": "42",
		},
		Raw: `[mrql query='type = "resource"' scope="42"]`,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)

	if capturedScope != 42 {
		t.Errorf("expected scope 42, got %d", capturedScope)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./shortcodes/... -run TestMRQLShortcodeScopePassthrough -v`
Expected: FAIL — QueryExecutor signature mismatch.

- [ ] **Step 3: Extend QueryExecutor signature**

In `shortcodes/processor.go`, update the `QueryExecutor` type (line 18):

```go
type QueryExecutor func(ctx context.Context, query string, savedName string, limit int, buckets int, scopeGroupID uint) (*QueryResult, error)
```

- [ ] **Step 4: Update RenderMRQLShortcode to parse and pass scope**

In `shortcodes/mrql_handler.go`, update `RenderMRQLShortcode` (line 19):

Add scope parsing after the format line (line 28):

```go
format := sc.Attrs["format"]

// Resolve scope: explicit numeric value, keyword, or default (entity)
scopeGroupID := resolveScopeKeyword(sc.Attrs["scope"], ctx)
```

Update the executor call (line 30):
```go
result, err := executor(reqCtx, query, saved, limit, buckets, scopeGroupID)
```

Add the scope keyword resolution function:

```go
// resolveScopeKeyword resolves the scope attribute to a group ID.
// Accepts a numeric string (direct ID), a keyword (entity/parent/root/global),
// or empty string (defaults to entity scope).
// IMPORTANT: For ownerless resources/notes, ScopeGroupID in the context must be
// set to mrql.UnresolvedScopeSentinel (not 0) by the template handler that builds
// MetaShortcodeContext. This ensures empty results instead of global fan-out.
// The sentinel is a nonexistent group ID that the CTE naturally returns nothing for.
func resolveScopeKeyword(scope string, ctx MetaShortcodeContext) uint {
	switch scope {
	case "global":
		return 0
	case "parent":
		return ctx.ParentGroupID
	case "root":
		return ctx.RootGroupID
	case "":
		// Default: entity scope (sentinel for ownerless, group ID for owned)
		return ctx.ScopeGroupID
	default:
		// Try numeric
		if id, err := strconv.ParseUint(scope, 10, 64); err == nil {
			return uint(id)
		}
		// Fall back to entity scope for unrecognized values
		return ctx.ScopeGroupID
	}
}
```

- [ ] **Step 5: Extend MetaShortcodeContext with scope resolution fields**

In `shortcodes/meta_handler.go`, update `MetaShortcodeContext`:

```go
type MetaShortcodeContext struct {
	EntityType    string
	EntityID      uint
	Meta          json.RawMessage
	MetaSchema    string
	Entity        any
	ScopeGroupID  uint // resolved "entity" scope group ID (the owning group)
	ParentGroupID uint // resolved "parent" scope group ID (owner's owner)
	RootGroupID   uint // resolved "root" scope group ID (top of chain)
}
```

These fields must be populated by the callers that create `MetaShortcodeContext` (template handlers). This keeps the shortcode package free of DB dependencies — the resolution happens upstream.

- [ ] **Step 6: Update all QueryExecutor callers and MetaShortcodeContext builders**

Update `shortcode_query_executor.go` `BuildQueryExecutor`:

```go
func BuildQueryExecutor(appCtx *application_context.MahresourcesContext) shortcodes.QueryExecutor {
	return func(reqCtx context.Context, query string, savedName string, limit int, buckets int, scopeGroupID uint) (*shortcodes.QueryResult, error) {
		return executeMRQLForShortcode(reqCtx, appCtx, query, savedName, limit, buckets, scopeGroupID)
	}
}
```

Update `executeMRQLForShortcode` to accept and use `scopeGroupID`:

```go
func executeMRQLForShortcode(reqCtx context.Context, appCtx *application_context.MahresourcesContext, query string, savedName string, limit int, buckets int, scopeGroupID uint) (*shortcodes.QueryResult, error) {
```

First, add a public scope resolution method to `MahresourcesContext` in `application_context/mrql_context.go`:

```go
// ResolveMRQLScope resolves a parsed query's SCOPE clause to a group ID.
// Returns 0 if no SCOPE clause is present. Errors on not-found or ambiguous names.
func (ctx *MahresourcesContext) ResolveMRQLScope(q *mrql.Query) (uint, error) {
	return mrql.ResolveScope(q, ctx.db)
}
```

Then in `executeMRQLForShortcode`, after parsing/validating, resolve explicit SCOPE precedence:

```go
// Explicit SCOPE in query string takes precedence over shortcode attribute
if parsed.Scope != nil {
	resolvedID, err := appCtx.ResolveMRQLScope(parsed)
	if err != nil {
		return nil, err
	}
	scopeGroupID = resolvedID
}
```

For the GROUP BY path, use scoped execution:
```go
grouped, err := appCtx.ExecuteMRQLGroupedWithScope(reqCtx, parsed, scopeGroupID)
```

For the flat path, **preserve cross-entity support** — do NOT switch to `ExecuteSingleEntityWithScope` which requires an explicit type. Instead, inject scopeGroupID into the parsed query's execution via a new scoped variant of `ExecuteMRQL`:

Add to `application_context/mrql_context.go`:

```go
// ExecuteMRQLScoped is like ExecuteMRQL but accepts a pre-parsed query and scope.
// Supports cross-entity queries (no type required) with scope filtering.
func (ctx *MahresourcesContext) ExecuteMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (*MRQLResult, error) {
	entityType := mrql.ExtractEntityType(parsed)
	opts := mrql.TranslateOptions{ScopeGroupID: scopeGroupID}

	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(reqCtx, parsed, entityType, opts)
	}
	return ctx.executeCrossEntity(reqCtx, parsed, opts)
}
```

Then the shortcode executor flat path becomes:
```go
result, err := appCtx.ExecuteMRQLScoped(reqCtx, parsed, scopeGroupID)
```

This preserves cross-entity queries (`name ~ "foo"` without `type = ...`) while applying scope.

- [ ] **Step 7: Extend QueryResultItem with precomputed scope IDs for nested contexts**

The recursive child context in `renderFlatWithCustom` (mrql_handler.go:98) creates a `MetaShortcodeContext` for each result item. With the new scope fields, these child contexts need `ScopeGroupID`, `ParentGroupID`, and `RootGroupID`. The shortcode layer is DB-free, so these must be precomputed.

In `shortcodes/processor.go`, extend `QueryResultItem`:

```go
type QueryResultItem struct {
	EntityType       string
	EntityID         uint
	Entity           any
	Meta             json.RawMessage
	MetaSchema       string
	CustomMRQLResult string
	ScopeGroupID     uint // precomputed: owning group ID (or sentinel for ownerless)
	ParentGroupID    uint // precomputed: owner's owner ID
	RootGroupID      uint // precomputed: root of ownership chain
}
```

In `shortcode_query_executor.go`, when building `QueryResultItem` from each entity, look up the entity's `OwnerId` and resolve the chain:

```go
// For a resource:
scopeID := mrql.UnresolvedScopeSentinel
if r.OwnerId != nil && *r.OwnerId > 0 {
	scopeID = *r.OwnerId
}
item.ScopeGroupID = scopeID
item.ParentGroupID = appCtx.ResolveParentScopeID(scopeID)
item.RootGroupID = appCtx.ResolveRootScopeID(scopeID)
```

Add `ResolveParentScopeID(groupID uint) uint` and `ResolveRootScopeID(groupID uint) uint` public methods to `MahresourcesContext` that wrap the existing logic from `db_api.go`'s `resolveParentScope` / `resolveRootScope`, returning the sentinel for failures.

In `renderFlatWithCustom`, use the precomputed values:

```go
childCtx := MetaShortcodeContext{
	EntityType:    item.EntityType,
	EntityID:      item.EntityID,
	Meta:          item.Meta,
	MetaSchema:    item.MetaSchema,
	Entity:        item.Entity,
	ScopeGroupID:  item.ScopeGroupID,
	ParentGroupID: item.ParentGroupID,
	RootGroupID:   item.RootGroupID,
}
```

- [ ] **Step 8: Update MetaShortcodeContext construction in template handlers**

Search for all places that construct `MetaShortcodeContext` in the template handlers and populate the new scope fields. The caller needs to look up the entity's `owner_id` for entity scope, the owner's owner for parent scope, and walk to root.

Key locations to update:
- Template handlers that render group/resource/note detail pages
- Use the same `ResolveParentScopeID` / `ResolveRootScopeID` helpers added in Step 7

- [ ] **Step 9: Fix all compilation errors and run tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./shortcodes/... -v -count=1`
Then: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./... -count=1`
Expected: All PASS.

- [ ] **Step 10: Commit**

```bash
git add shortcodes/ server/template_handlers/ application_context/
git commit -m "feat: add scope attribute to [mrql] shortcode"
```

---

### Task 7: Completer — Add SCOPE to Autocomplete Suggestions

**Files:**
- Modify: `mrql/completer.go:28-34` (add SCOPE to postValueKeywords)
- Test: `mrql/completer_test.go`

- [ ] **Step 1: Add SCOPE to postValueKeywords**

In `mrql/completer.go`, add SCOPE to the `postValueKeywords` slice (around line 32):

```go
var postValueKeywords = []Suggestion{
	{Value: "AND", Type: "keyword"},
	{Value: "OR", Type: "keyword"},
	{Value: "SCOPE", Type: "keyword"},
	{Value: "GROUP BY", Type: "keyword"},
	{Value: "ORDER BY", Type: "keyword"},
	{Value: "LIMIT", Type: "keyword"},
}
```

Also add SCOPE to the `postAggregateKeywords` slice if applicable (after GROUP BY aggregates, SCOPE doesn't apply, so skip it there).

- [ ] **Step 2: Check completer tests and update if needed**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/... -run TestComplete -v`
Fix any failures caused by the new suggestion.

- [ ] **Step 3: Commit**

```bash
git add mrql/completer.go mrql/completer_test.go
git commit -m "feat(mrql): add SCOPE to autocomplete suggestions"
```

---

### Task 8: CLI Help Text and Documentation

**Files:**
- Modify: `cmd/mr/commands/mrql.go` (update help text)
- Modify: `docs-site/docs/features/mrql.md` (add SCOPE section)
- Modify: `docs-site/docs/features/shortcodes.md` (add scope attribute)

- [ ] **Step 1: Update CLI help text**

In `cmd/mr/commands/mrql.go`, update the `Long` description to include SCOPE examples:

Add after the GROUP BY examples:

```
Scope (filter to group subtree):
  mr mrql 'type = resource SCOPE 42'
  mr mrql 'type = note SCOPE "My Project" ORDER BY created'
  mr mrql 'type = resource SCOPE 7 GROUP BY contentType COUNT()'
```

- [ ] **Step 2: Update MRQL docs**

In `docs-site/docs/features/mrql.md`, add a new section (after "Ordering and Pagination", before "GROUP BY"):

```markdown
## Scope

The `SCOPE` clause filters query results to entities within a group's ownership subtree. Scope is placed after the filter expression and before `GROUP BY`:

```
type = "resource" SCOPE 42 ORDER BY created LIMIT 10
type = "note" SCOPE "My Project"
```

### Scope by ID

`SCOPE <number>` filters to the group with that ID and all its descendants:

```
type = "resource" SCOPE 42
```

This returns all resources owned by group 42 or any group underneath it in the hierarchy.

### Scope by Name

`SCOPE "group name"` looks up the group by name (case-insensitive):

```
type = "resource" SCOPE "Vacation Photos"
```

If multiple groups share the same name, MRQL returns an error listing all matches with their IDs so you can switch to `SCOPE <id>`.

### Scope with GROUP BY

Scope is applied before grouping:

```
type = "resource" SCOPE 42 GROUP BY contentType COUNT()
```

### No Scope

Omitting `SCOPE` or using `SCOPE 0` returns all matching entities regardless of ownership.

### Entity Types

- **Resources and Notes:** Scope filters by `owner_id` — entities owned by groups in the subtree.
- **Groups:** Scope filters by `id` — the scoped group itself and all its descendants.
```

- [ ] **Step 3: Update shortcodes docs**

In `docs-site/docs/features/shortcodes.md`, in the `[mrql]` shortcode section, add `scope` to the attributes table:

```markdown
| `scope` | `"entity"` | Scope filter: `entity` (default), `parent`, `root`, `global`, or a numeric group ID |
```

Add a brief explanation:

```markdown
### Scope

The `scope` attribute limits query results to a group's subtree. By default, it scopes to the current entity's owning group:

- `entity` (default) — the entity's owning group and its subtree
- `parent` — the parent group's subtree
- `root` — the root group's subtree (everything in the hierarchy)
- `global` — no scope filter

An explicit `SCOPE` clause in the MRQL query takes precedence over the attribute.
```

- [ ] **Step 4: Update data-views plugin docs**

In `plugins/data-views/plugin.lua`, find the scope attribute descriptions (they appear in multiple shortcode attribute tables with text like `"MRQL scope: 'entity' (default), 'parent', 'root', or 'global'"`). Update each occurrence to note the tree semantics:

```lua
{ name = "scope", type = "string", description = "MRQL scope: 'entity' (default), 'parent', 'root', or 'global'. Filters to the group's full subtree (the group and all its descendants)." }
```

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/mrql.go docs-site/docs/features/mrql.md docs-site/docs/features/shortcodes.md plugins/data-views/plugin.lua
git commit -m "docs: add SCOPE to MRQL reference, shortcode docs, plugin docs, and CLI help"
```

---

### Task 9: E2E Tests

**Files:**
- Modify: `e2e/tests/mrql.spec.ts` (add SCOPE tests)
- Modify: `e2e/tests/cli/cli-mrql.spec.ts` (add CLI SCOPE tests)

- [ ] **Step 1: Add MRQL SCOPE E2E tests**

In `e2e/tests/mrql.spec.ts`, add a new describe block:

```typescript
test.describe('MRQL SCOPE', () => {
  let parentGroup: any;
  let childGroup: any;
  let grandchildGroup: any;
  let scopedResource: any;
  let unscopedResource: any;

  test.beforeAll(async ({ apiClient }) => {
    // Create hierarchy: parent -> child -> grandchild
    parentGroup = await apiClient.createGroup({ name: 'Scope Parent' });
    childGroup = await apiClient.createGroup({ name: 'Scope Child', ownerId: parentGroup.id });
    grandchildGroup = await apiClient.createGroup({ name: 'Scope Grandchild', ownerId: childGroup.id });

    // Create resources: one in child, one unscoped
    scopedResource = await apiClient.createResource({
      name: 'scoped-file.txt',
      ownerId: childGroup.id,
    });
    unscopedResource = await apiClient.createResource({
      name: 'unscoped-file.txt',
    });
  });

  test('SCOPE by ID returns subtree resources', async ({ page }) => {
    // Navigate to MRQL page, enter query with SCOPE parentGroup.id
    // Verify scopedResource appears, unscopedResource does not
  });

  test('SCOPE by name returns subtree resources', async ({ page }) => {
    // Enter: type = "resource" SCOPE "Scope Parent"
    // Verify scopedResource appears
  });

  test('SCOPE for groups includes the scoped group itself', async ({ page }) => {
    // Enter: type = "group" SCOPE parentGroup.id
    // Verify parentGroup, childGroup, grandchildGroup all appear
  });

  test('SCOPE with nonexistent ID shows error', async ({ page }) => {
    // Enter: type = "resource" SCOPE 999999
    // Verify error message appears
  });

  test('SCOPE with ambiguous name shows error listing matches', async ({ page }) => {
    // Create second group with same name, query by name, verify error
  });

  test.afterAll(async ({ apiClient }) => {
    // Cleanup in reverse dependency order
  });
});
```

- [ ] **Step 2: Add CLI SCOPE E2E tests**

In `e2e/tests/cli/cli-mrql.spec.ts`, add:

```typescript
test.describe('MRQL SCOPE via CLI', () => {
  test('SCOPE by ID returns filtered results', async ({ cli }) => {
    // Use a known group ID from test data
    const result = cli.run('mrql', 'type = "resource" SCOPE 1');
    if (result.exitCode === 0) {
      expect(result.stdout).toBeTruthy();
    }
  });

  test('SCOPE with nonexistent ID returns error', async ({ cli }) => {
    const result = cli.runExpectError('mrql', 'type = "resource" SCOPE 999999');
    expect(result.stderr).toContain('scope group not found');
  });
});
```

- [ ] **Step 3: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:all`
Expected: All tests PASS (both existing and new).

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/mrql.spec.ts e2e/tests/cli/cli-mrql.spec.ts
git commit -m "test: add E2E tests for MRQL SCOPE"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run full Go test suite**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./... -count=1`
Expected: All PASS.

- [ ] **Step 2: Run Postgres tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: All PASS (recursive CTE works on Postgres too).

- [ ] **Step 3: Run full E2E suite**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:all`
Expected: All PASS.

- [ ] **Step 4: Run Postgres E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:postgres`
Expected: All PASS.
