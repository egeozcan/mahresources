# MRQL Query Language Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a structured text-based query language (MRQL) to mahresources with a dedicated query page, CLI command, and documentation.

**Architecture:** Hand-written recursive descent parser in Go (`mrql/` package) translates MRQL syntax to GORM scopes. A CodeMirror 6 editor (already a dependency) provides syntax highlighting and autocompletion on a new `/v1/mrql` page. The `mr mrql` CLI command posts queries to the same API. Saved MRQL queries use a separate model from existing SQL queries.

**Tech Stack:** Go (parser, API), GORM (query translation), CodeMirror 6 (editor), Alpine.js (page interactivity), Cobra (CLI), Playwright (E2E tests)

**Spec:** `docs/superpowers/specs/2026-03-28-mrql-query-language-design.md`

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `mrql/token.go` | Token types and token struct |
| `mrql/lexer.go` | Tokenizer: string -> tokens with positions |
| `mrql/lexer_test.go` | Lexer unit tests |
| `mrql/ast.go` | AST node types |
| `mrql/parser.go` | Recursive descent parser: tokens -> AST |
| `mrql/parser_test.go` | Parser unit tests |
| `mrql/validator.go` | Field validation per entity type |
| `mrql/validator_test.go` | Validator unit tests |
| `mrql/translator.go` | AST -> GORM scopes |
| `mrql/translator_test.go` | Translator unit tests (against in-memory SQLite) |
| `mrql/completer.go` | Completion engine: partial query + cursor -> suggestions |
| `mrql/completer_test.go` | Completer unit tests |
| `mrql/fields.go` | Field definitions per entity type (name, type, valid operators) |
| `models/saved_mrql_query_model.go` | SavedMRQLQuery GORM model |
| `application_context/mrql_context.go` | Business logic for MRQL execution and saved queries |
| `server/api_handlers/mrql_api_handlers.go` | HTTP handlers: execute, validate, complete, saved CRUD |
| `server/interfaces/mrql_interfaces.go` | Interface definitions for MRQL handlers |
| `templates/mrql.tpl` | Pongo2 template for the MRQL query page |
| `server/template_handlers/template_context_providers/mrql_template_context.go` | Template context provider |
| `src/components/mrqlEditor.js` | CodeMirror integration with MRQL language mode |
| `cmd/mr/commands/mrql.go` | CLI `mr mrql` command with subcommands |
| `docs-site/docs/features/mrql.md` | Documentation site page |
| `e2e/tests/mrql.spec.ts` | Browser E2E tests |
| `e2e/tests/cli/cli-mrql.spec.ts` | CLI E2E tests |
| `e2e/tests/accessibility/mrql-a11y.spec.ts` | Accessibility tests |
| `e2e/pages/mrql.page.ts` | Page Object Model for MRQL page |

### Modified files

| File | Change |
|------|--------|
| `main.go` | Add `SavedMRQLQuery` to AutoMigrate, add `-mrql-query-timeout` flag |
| `server/routes.go` | Register MRQL API routes and template route |
| `src/main.js` | Import and register `mrqlEditor` Alpine component |
| `vite.config.js` | Add `mrql` manual chunk |
| `cmd/mr/main.go` | Register `mrql` command |
| `docs-site/sidebars.ts` | Add MRQL page to Advanced Features |
| `docs-site/docs/features/cli.md` | Add `mr mrql` section |

---

### Task 1: Token Types and AST Nodes

**Files:**
- Create: `mrql/token.go`
- Create: `mrql/ast.go`

- [ ] **Step 1: Create `mrql/token.go` with all token types**

```go
package mrql

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Literals
	TokenString    TokenType = iota // "quoted string"
	TokenNumber                     // 42, 10mb, 3.14
	TokenIdentifier                 // field names: name, contentType, etc.

	// Keywords
	TokenAnd       // AND
	TokenOr        // OR
	TokenNot       // NOT
	TokenIn        // IN
	TokenIs        // IS
	TokenEmpty     // EMPTY
	TokenNull      // NULL
	TokenOrderBy   // ORDER BY (two words, merged by lexer)
	TokenAsc       // ASC
	TokenDesc      // DESC
	TokenLimit     // LIMIT
	TokenOffset    // OFFSET
	TokenText      // TEXT (for TEXT ~)
	TokenType      // TYPE (also usable as field name via context)

	// Operators
	TokenEq        // =
	TokenNeq       // !=
	TokenGt        // >
	TokenGte       // >=
	TokenLt        // <
	TokenLte       // <=
	TokenLike      // ~
	TokenNotLike   // !~

	// Delimiters
	TokenLParen    // (
	TokenRParen    // )
	TokenComma     // ,
	TokenDot       // .

	// Special
	TokenRelDate   // -7d, -30d, -3m, -1y
	TokenFunc      // NOW(), START_OF_DAY(), etc.

	TokenEOF
	TokenIllegal
)

// Token represents a single lexical token with its position in the source.
type Token struct {
	Type    TokenType
	Value   string
	Pos     int // byte offset in the source string
	Length  int // length in bytes
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%v, %q, pos=%d)", t.Type, t.Value, t.Pos)
}
```

- [ ] **Step 2: Create `mrql/ast.go` with AST node types**

```go
package mrql

// Node is the interface implemented by all AST nodes.
type Node interface {
	nodeType() string
	Pos() int // start position in the source string
}

// BinaryExpr represents: left AND/OR right
type BinaryExpr struct {
	Left     Node
	Operator Token // AND, OR
	Right    Node
}

func (b *BinaryExpr) nodeType() string { return "BinaryExpr" }
func (b *BinaryExpr) Pos() int         { return b.Left.Pos() }

// NotExpr represents: NOT expr
type NotExpr struct {
	Token Token
	Expr  Node
}

func (n *NotExpr) nodeType() string { return "NotExpr" }
func (n *NotExpr) Pos() int         { return n.Token.Pos }

// ComparisonExpr represents: field op value
type ComparisonExpr struct {
	Field    *FieldExpr
	Operator Token
	Value    Node // StringLiteral, NumberLiteral, RelDate, FuncCall
}

func (c *ComparisonExpr) nodeType() string { return "ComparisonExpr" }
func (c *ComparisonExpr) Pos() int         { return c.Field.Pos() }

// InExpr represents: field IN ("a", "b") or field NOT IN ("a", "b")
type InExpr struct {
	Field    *FieldExpr
	Negated  bool
	Values   []Node // list of StringLiteral or NumberLiteral
	InToken  Token
}

func (i *InExpr) nodeType() string { return "InExpr" }
func (i *InExpr) Pos() int         { return i.Field.Pos() }

// IsExpr represents: field IS [NOT] EMPTY/NULL
type IsExpr struct {
	Field   *FieldExpr
	Negated bool
	IsNull  bool // true = IS [NOT] NULL, false = IS [NOT] EMPTY
	IsToken Token
}

func (e *IsExpr) nodeType() string { return "IsExpr" }
func (e *IsExpr) Pos() int         { return e.Field.Pos() }

// TextSearchExpr represents: TEXT ~ "query"
type TextSearchExpr struct {
	TextToken Token
	Value     *StringLiteral
}

func (t *TextSearchExpr) nodeType() string { return "TextSearchExpr" }
func (t *TextSearchExpr) Pos() int         { return t.TextToken.Pos }

// FieldExpr represents a field reference: name, meta.key, parent.name
type FieldExpr struct {
	Parts []Token // e.g., ["parent", "name"] or ["meta", "rating"] or ["name"]
}

func (f *FieldExpr) nodeType() string { return "FieldExpr" }
func (f *FieldExpr) Pos() int         { return f.Parts[0].Pos }

func (f *FieldExpr) Name() string {
	if len(f.Parts) == 1 {
		return f.Parts[0].Value
	}
	result := f.Parts[0].Value
	for _, p := range f.Parts[1:] {
		result += "." + p.Value
	}
	return result
}

// StringLiteral is a quoted string value.
type StringLiteral struct {
	Token Token
	Value string // unescaped value
}

func (s *StringLiteral) nodeType() string { return "StringLiteral" }
func (s *StringLiteral) Pos() int         { return s.Token.Pos }

// NumberLiteral is a numeric value, optionally with a unit (kb, mb, gb).
type NumberLiteral struct {
	Token Token
	Value float64
	Unit  string // "", "kb", "mb", "gb"
	Raw   int64  // value converted to base unit (bytes for file sizes)
}

func (n *NumberLiteral) nodeType() string { return "NumberLiteral" }
func (n *NumberLiteral) Pos() int         { return n.Token.Pos }

// RelDateLiteral is a relative date like -7d, -3m, -1y.
type RelDateLiteral struct {
	Token    Token
	Amount   int
	Unit     string // "d", "w", "m", "y"
}

func (r *RelDateLiteral) nodeType() string { return "RelDateLiteral" }
func (r *RelDateLiteral) Pos() int         { return r.Token.Pos }

// FuncCall represents a date function like NOW(), START_OF_DAY(), etc.
type FuncCall struct {
	Token Token
	Name  string
}

func (f *FuncCall) nodeType() string { return "FuncCall" }
func (f *FuncCall) Pos() int         { return f.Token.Pos }

// OrderByClause is a single ORDER BY column+direction.
type OrderByClause struct {
	Field     *FieldExpr
	Ascending bool // true = ASC, false = DESC
}

// Query is the top-level AST node for a complete MRQL query.
type Query struct {
	Where   Node             // the filter expression (may be nil)
	OrderBy []OrderByClause  // ORDER BY clauses (may be empty)
	Limit   int              // -1 if not specified
	Offset  int              // -1 if not specified
}
```

- [ ] **Step 3: Verify the package compiles**

Run: `cd /Users/egecan/Code/mahresources && go build ./mrql/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add mrql/token.go mrql/ast.go
git commit -m "feat(mrql): add token types and AST node definitions"
```

---

### Task 2: Lexer

**Files:**
- Create: `mrql/lexer.go`
- Create: `mrql/lexer_test.go`

- [ ] **Step 1: Write lexer tests**

```go
package mrql

import "testing"

func TestLexer_BasicTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{`name = "hello"`, []TokenType{TokenIdentifier, TokenEq, TokenString, TokenEOF}},
		{`name != "hello"`, []TokenType{TokenIdentifier, TokenNeq, TokenString, TokenEOF}},
		{`name ~ "sun*"`, []TokenType{TokenIdentifier, TokenLike, TokenString, TokenEOF}},
		{`name !~ "draft"`, []TokenType{TokenIdentifier, TokenNotLike, TokenString, TokenEOF}},
		{`fileSize > 10mb`, []TokenType{TokenIdentifier, TokenGt, TokenNumber, TokenEOF}},
		{`created >= "2024-01-01"`, []TokenType{TokenIdentifier, TokenGte, TokenString, TokenEOF}},
		{`width < 1920`, []TokenType{TokenIdentifier, TokenLt, TokenNumber, TokenEOF}},
		{`meta.rating <= 3`, []TokenType{TokenIdentifier, TokenDot, TokenIdentifier, TokenLte, TokenNumber, TokenEOF}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := l.Next()
				if tok.Type != expected {
					t.Errorf("token %d: expected %v, got %v (%q)", i, expected, tok.Type, tok.Value)
				}
			}
		})
	}
}

func TestLexer_Keywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{`AND OR NOT`, []TokenType{TokenAnd, TokenOr, TokenNot, TokenEOF}},
		{`IN IS EMPTY NULL`, []TokenType{TokenIn, TokenIs, TokenEmpty, TokenNull, TokenEOF}},
		{`ORDER BY name ASC`, []TokenType{TokenOrderBy, TokenIdentifier, TokenAsc, TokenEOF}},
		{`LIMIT 50 OFFSET 10`, []TokenType{TokenLimit, TokenNumber, TokenOffset, TokenNumber, TokenEOF}},
		{`TEXT ~ "search"`, []TokenType{TokenText, TokenLike, TokenString, TokenEOF}},
		// Keywords are case-insensitive
		{`and or not`, []TokenType{TokenAnd, TokenOr, TokenNot, TokenEOF}},
		{`And Or Not`, []TokenType{TokenAnd, TokenOr, TokenNot, TokenEOF}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := l.Next()
				if tok.Type != expected {
					t.Errorf("token %d: expected %v, got %v (%q)", i, expected, tok.Type, tok.Value)
				}
			}
		})
	}
}

func TestLexer_StringEscaping(t *testing.T) {
	tests := []struct {
		input         string
		expectedValue string
	}{
		{`"hello"`, "hello"},
		{`"said \"hello\""`, `said "hello"`},
		{`"file\\backup"`, `file\backup`},
		{`"no escapes"`, "no escapes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			tok := l.Next()
			if tok.Type != TokenString {
				t.Fatalf("expected TokenString, got %v", tok.Type)
			}
			if tok.Value != tt.expectedValue {
				t.Errorf("expected value %q, got %q", tt.expectedValue, tok.Value)
			}
		})
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		input         string
		expectedValue string
	}{
		{"42", "42"},
		{"3.14", "3.14"},
		{"10mb", "10mb"},
		{"5gb", "5gb"},
		{"100kb", "100kb"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			tok := l.Next()
			if tok.Type != TokenNumber {
				t.Fatalf("expected TokenNumber, got %v (%q)", tok.Type, tok.Value)
			}
			if tok.Value != tt.expectedValue {
				t.Errorf("expected value %q, got %q", tt.expectedValue, tok.Value)
			}
		})
	}
}

func TestLexer_RelativeDates(t *testing.T) {
	tests := []struct {
		input         string
		expectedValue string
	}{
		{"-7d", "-7d"},
		{"-30d", "-30d"},
		{"-3m", "-3m"},
		{"-1y", "-1y"},
		{"-2w", "-2w"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			tok := l.Next()
			if tok.Type != TokenRelDate {
				t.Fatalf("expected TokenRelDate, got %v (%q)", tok.Type, tok.Value)
			}
			if tok.Value != tt.expectedValue {
				t.Errorf("expected value %q, got %q", tt.expectedValue, tok.Value)
			}
		})
	}
}

func TestLexer_Functions(t *testing.T) {
	funcs := []string{"NOW()", "START_OF_DAY()", "START_OF_WEEK()", "START_OF_MONTH()", "START_OF_YEAR()"}
	for _, f := range funcs {
		t.Run(f, func(t *testing.T) {
			l := NewLexer(f)
			tok := l.Next()
			if tok.Type != TokenFunc {
				t.Fatalf("expected TokenFunc, got %v (%q)", tok.Type, tok.Value)
			}
		})
	}
}

func TestLexer_Positions(t *testing.T) {
	l := NewLexer(`name = "hello"`)
	tok1 := l.Next() // name
	if tok1.Pos != 0 || tok1.Length != 4 {
		t.Errorf("name: expected pos=0 len=4, got pos=%d len=%d", tok1.Pos, tok1.Length)
	}
	tok2 := l.Next() // =
	if tok2.Pos != 5 || tok2.Length != 1 {
		t.Errorf("=: expected pos=5 len=1, got pos=%d len=%d", tok2.Pos, tok2.Length)
	}
	tok3 := l.Next() // "hello"
	if tok3.Pos != 7 || tok3.Length != 7 {
		t.Errorf("string: expected pos=7 len=7, got pos=%d len=%d", tok3.Pos, tok3.Length)
	}
}

func TestLexer_Delimiters(t *testing.T) {
	l := NewLexer(`tags IN ("a", "b")`)
	expected := []TokenType{TokenIdentifier, TokenIn, TokenLParen, TokenString, TokenComma, TokenString, TokenRParen, TokenEOF}
	for i, exp := range expected {
		tok := l.Next()
		if tok.Type != exp {
			t.Errorf("token %d: expected %v, got %v (%q)", i, exp, tok.Type, tok.Value)
		}
	}
}

func TestLexer_IllegalToken(t *testing.T) {
	l := NewLexer(`name @ "hello"`)
	l.Next() // name
	tok := l.Next()
	if tok.Type != TokenIllegal {
		t.Errorf("expected TokenIllegal, got %v (%q)", tok.Type, tok.Value)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestLexer -v`
Expected: Compilation failure — `NewLexer` not defined

- [ ] **Step 3: Implement the lexer**

```go
package mrql

import (
	"strings"
	"unicode"
)

// Lexer tokenizes an MRQL query string.
type Lexer struct {
	input string
	pos   int
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input}
}

// Next returns the next token from the input.
func (l *Lexer) Next() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Pos: l.pos}
	}

	ch := l.input[l.pos]

	// String literal
	if ch == '"' {
		return l.readString()
	}

	// Number or relative date (starting with -)
	if ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
		return l.readNegativeNumberOrRelDate()
	}

	if isDigit(ch) {
		return l.readNumber()
	}

	// Two-character operators
	if l.pos+1 < len(l.input) {
		two := l.input[l.pos : l.pos+2]
		switch two {
		case "!=":
			l.pos += 2
			return Token{Type: TokenNeq, Value: "!=", Pos: l.pos - 2, Length: 2}
		case ">=":
			l.pos += 2
			return Token{Type: TokenGte, Value: ">=", Pos: l.pos - 2, Length: 2}
		case "<=":
			l.pos += 2
			return Token{Type: TokenLte, Value: "<=", Pos: l.pos - 2, Length: 2}
		case "!~":
			l.pos += 2
			return Token{Type: TokenNotLike, Value: "!~", Pos: l.pos - 2, Length: 2}
		}
	}

	// Single-character operators and delimiters
	switch ch {
	case '=':
		l.pos++
		return Token{Type: TokenEq, Value: "=", Pos: l.pos - 1, Length: 1}
	case '>':
		l.pos++
		return Token{Type: TokenGt, Value: ">", Pos: l.pos - 1, Length: 1}
	case '<':
		l.pos++
		return Token{Type: TokenLt, Value: "<", Pos: l.pos - 1, Length: 1}
	case '~':
		l.pos++
		return Token{Type: TokenLike, Value: "~", Pos: l.pos - 1, Length: 1}
	case '(':
		l.pos++
		return Token{Type: TokenLParen, Value: "(", Pos: l.pos - 1, Length: 1}
	case ')':
		l.pos++
		return Token{Type: TokenRParen, Value: ")", Pos: l.pos - 1, Length: 1}
	case ',':
		l.pos++
		return Token{Type: TokenComma, Value: ",", Pos: l.pos - 1, Length: 1}
	case '.':
		l.pos++
		return Token{Type: TokenDot, Value: ".", Pos: l.pos - 1, Length: 1}
	}

	// Identifiers and keywords
	if isIdentStart(ch) {
		return l.readIdentOrKeyword()
	}

	// Illegal character
	l.pos++
	return Token{Type: TokenIllegal, Value: string(ch), Pos: l.pos - 1, Length: 1}
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	savedPos := l.pos
	tok := l.Next()
	l.pos = savedPos
	return tok
}

// Position returns the current byte position in the input.
func (l *Lexer) Position() int {
	return l.pos
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\n' || l.input[l.pos] == '\r') {
		l.pos++
	}
}

func (l *Lexer) readString() Token {
	startPos := l.pos
	l.pos++ // skip opening quote
	var sb strings.Builder

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			if next == '"' || next == '\\' {
				sb.WriteByte(next)
				l.pos += 2
				continue
			}
		}
		if ch == '"' {
			l.pos++ // skip closing quote
			return Token{Type: TokenString, Value: sb.String(), Pos: startPos, Length: l.pos - startPos}
		}
		sb.WriteByte(ch)
		l.pos++
	}

	// Unterminated string
	return Token{Type: TokenIllegal, Value: sb.String(), Pos: startPos, Length: l.pos - startPos}
}

func (l *Lexer) readNumber() Token {
	startPos := l.pos
	for l.pos < len(l.input) && (isDigit(l.input[l.pos]) || l.input[l.pos] == '.') {
		l.pos++
	}
	// Check for unit suffix (kb, mb, gb)
	if l.pos < len(l.input) && isLetter(l.input[l.pos]) {
		unitStart := l.pos
		for l.pos < len(l.input) && isLetter(l.input[l.pos]) {
			l.pos++
		}
		unit := strings.ToLower(l.input[unitStart:l.pos])
		if unit == "kb" || unit == "mb" || unit == "gb" {
			return Token{Type: TokenNumber, Value: l.input[startPos:l.pos], Pos: startPos, Length: l.pos - startPos}
		}
		// Not a valid unit — rewind
		l.pos = unitStart
	}
	return Token{Type: TokenNumber, Value: l.input[startPos:l.pos], Pos: startPos, Length: l.pos - startPos}
}

func (l *Lexer) readNegativeNumberOrRelDate() Token {
	startPos := l.pos
	l.pos++ // skip -
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	// Check for relative date suffix
	if l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == 'd' || ch == 'w' || ch == 'm' || ch == 'y' {
			l.pos++
			return Token{Type: TokenRelDate, Value: l.input[startPos:l.pos], Pos: startPos, Length: l.pos - startPos}
		}
	}
	// It's a negative number
	return Token{Type: TokenNumber, Value: l.input[startPos:l.pos], Pos: startPos, Length: l.pos - startPos}
}

// knownFunctions lists recognized function names (without parens).
var knownFunctions = map[string]bool{
	"NOW":            true,
	"START_OF_DAY":   true,
	"START_OF_WEEK":  true,
	"START_OF_MONTH": true,
	"START_OF_YEAR":  true,
}

func (l *Lexer) readIdentOrKeyword() Token {
	startPos := l.pos
	for l.pos < len(l.input) && isIdentPart(l.input[l.pos]) {
		l.pos++
	}
	word := l.input[startPos:l.pos]
	upper := strings.ToUpper(word)

	// Check for function call: WORD()
	if knownFunctions[upper] {
		savedPos := l.pos
		l.skipWhitespace()
		if l.pos+1 < len(l.input) && l.input[l.pos] == '(' && l.input[l.pos+1] == ')' {
			l.pos += 2
			return Token{Type: TokenFunc, Value: upper + "()", Pos: startPos, Length: l.pos - startPos}
		}
		l.pos = savedPos
	}

	// Check for ORDER BY (two words)
	if upper == "ORDER" {
		savedPos := l.pos
		l.skipWhitespace()
		if l.pos < len(l.input) && isIdentStart(l.input[l.pos]) {
			nextStart := l.pos
			for l.pos < len(l.input) && isIdentPart(l.input[l.pos]) {
				l.pos++
			}
			nextWord := strings.ToUpper(l.input[nextStart:l.pos])
			if nextWord == "BY" {
				return Token{Type: TokenOrderBy, Value: "ORDER BY", Pos: startPos, Length: l.pos - startPos}
			}
			l.pos = savedPos
		} else {
			l.pos = savedPos
		}
	}

	// Keywords (case-insensitive)
	switch upper {
	case "AND":
		return Token{Type: TokenAnd, Value: "AND", Pos: startPos, Length: l.pos - startPos}
	case "OR":
		return Token{Type: TokenOr, Value: "OR", Pos: startPos, Length: l.pos - startPos}
	case "NOT":
		return Token{Type: TokenNot, Value: "NOT", Pos: startPos, Length: l.pos - startPos}
	case "IN":
		return Token{Type: TokenIn, Value: "IN", Pos: startPos, Length: l.pos - startPos}
	case "IS":
		return Token{Type: TokenIs, Value: "IS", Pos: startPos, Length: l.pos - startPos}
	case "EMPTY":
		return Token{Type: TokenEmpty, Value: "EMPTY", Pos: startPos, Length: l.pos - startPos}
	case "NULL":
		return Token{Type: TokenNull, Value: "NULL", Pos: startPos, Length: l.pos - startPos}
	case "ASC":
		return Token{Type: TokenAsc, Value: "ASC", Pos: startPos, Length: l.pos - startPos}
	case "DESC":
		return Token{Type: TokenDesc, Value: "DESC", Pos: startPos, Length: l.pos - startPos}
	case "LIMIT":
		return Token{Type: TokenLimit, Value: "LIMIT", Pos: startPos, Length: l.pos - startPos}
	case "OFFSET":
		return Token{Type: TokenOffset, Value: "OFFSET", Pos: startPos, Length: l.pos - startPos}
	case "TEXT":
		return Token{Type: TokenText, Value: "TEXT", Pos: startPos, Length: l.pos - startPos}
	case "TYPE":
		return Token{Type: TokenType, Value: "type", Pos: startPos, Length: l.pos - startPos}
	}

	// Regular identifier — preserve original case
	return Token{Type: TokenIdentifier, Value: word, Pos: startPos, Length: l.pos - startPos}
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentStart(ch byte) bool {
	return isLetter(ch) || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isLetter(ch) || isDigit(ch) || ch == '_'
}

// IsKeyword returns true if the rune could start a keyword character.
// Used by unicode-aware checks elsewhere.
func IsKeyword(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestLexer -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add mrql/lexer.go mrql/lexer_test.go
git commit -m "feat(mrql): implement lexer with full token support"
```

---

### Task 3: Parser — Core Expression Parsing

**Files:**
- Create: `mrql/parser.go`
- Create: `mrql/parser_test.go`

- [ ] **Step 1: Write parser tests**

```go
package mrql

import "testing"

func TestParser_SimpleComparison(t *testing.T) {
	tests := []struct {
		input      string
		fieldName  string
		operator   TokenType
		valueType  string
	}{
		{`name = "hello"`, "name", TokenEq, "StringLiteral"},
		{`fileSize > 10mb`, "fileSize", TokenGt, "NumberLiteral"},
		{`created > -7d`, "created", TokenGt, "RelDateLiteral"},
		{`created >= NOW()`, "created", TokenGte, "FuncCall"},
		{`name ~ "sun*"`, "name", TokenLike, "StringLiteral"},
		{`name !~ "*draft*"`, "name", TokenNotLike, "StringLiteral"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			comp, ok := q.Where.(*ComparisonExpr)
			if !ok {
				t.Fatalf("expected ComparisonExpr, got %T", q.Where)
			}
			if comp.Field.Name() != tt.fieldName {
				t.Errorf("field: expected %q, got %q", tt.fieldName, comp.Field.Name())
			}
			if comp.Operator.Type != tt.operator {
				t.Errorf("operator: expected %v, got %v", tt.operator, comp.Operator.Type)
			}
			if comp.Value.nodeType() != tt.valueType {
				t.Errorf("value type: expected %s, got %s", tt.valueType, comp.Value.nodeType())
			}
		})
	}
}

func TestParser_DottedFields(t *testing.T) {
	q, err := Parse(`meta.rating = 5`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	comp := q.Where.(*ComparisonExpr)
	if comp.Field.Name() != "meta.rating" {
		t.Errorf("expected meta.rating, got %s", comp.Field.Name())
	}
}

func TestParser_BooleanLogic(t *testing.T) {
	q, err := Parse(`name = "a" AND tags = "b"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenAnd {
		t.Errorf("expected AND, got %v", bin.Operator.Type)
	}
}

func TestParser_Precedence(t *testing.T) {
	// a OR b AND c should parse as a OR (b AND c)
	q, err := Parse(`name = "a" OR name = "b" AND name = "c"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenOr {
		t.Errorf("top level should be OR, got %v", bin.Operator.Type)
	}
	// Right side should be AND
	rightBin, ok := bin.Right.(*BinaryExpr)
	if !ok {
		t.Fatalf("right side should be BinaryExpr, got %T", bin.Right)
	}
	if rightBin.Operator.Type != TokenAnd {
		t.Errorf("right side should be AND, got %v", rightBin.Operator.Type)
	}
}

func TestParser_NotExpr(t *testing.T) {
	q, err := Parse(`NOT name = "draft"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	notExpr, ok := q.Where.(*NotExpr)
	if !ok {
		t.Fatalf("expected NotExpr, got %T", q.Where)
	}
	if _, ok := notExpr.Expr.(*ComparisonExpr); !ok {
		t.Errorf("inner should be ComparisonExpr, got %T", notExpr.Expr)
	}
}

func TestParser_Parentheses(t *testing.T) {
	q, err := Parse(`(name = "a" OR name = "b") AND tags = "c"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	bin := q.Where.(*BinaryExpr)
	if bin.Operator.Type != TokenAnd {
		t.Errorf("top level should be AND, got %v", bin.Operator.Type)
	}
	leftBin, ok := bin.Left.(*BinaryExpr)
	if !ok {
		t.Fatalf("left side should be BinaryExpr, got %T", bin.Left)
	}
	if leftBin.Operator.Type != TokenOr {
		t.Errorf("left side should be OR, got %v", leftBin.Operator.Type)
	}
}

func TestParser_InExpr(t *testing.T) {
	q, err := Parse(`tags IN ("a", "b", "c")`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	inExpr, ok := q.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected InExpr, got %T", q.Where)
	}
	if inExpr.Negated {
		t.Error("should not be negated")
	}
	if len(inExpr.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(inExpr.Values))
	}
}

func TestParser_NotInExpr(t *testing.T) {
	q, err := Parse(`category NOT IN ("Archive", "Trash")`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	inExpr := q.Where.(*InExpr)
	if !inExpr.Negated {
		t.Error("should be negated")
	}
}

func TestParser_IsEmpty(t *testing.T) {
	q, err := Parse(`tags IS EMPTY`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	isExpr, ok := q.Where.(*IsExpr)
	if !ok {
		t.Fatalf("expected IsExpr, got %T", q.Where)
	}
	if isExpr.IsNull {
		t.Error("should not be IS NULL")
	}
	if isExpr.Negated {
		t.Error("should not be negated")
	}
}

func TestParser_IsNotNull(t *testing.T) {
	q, err := Parse(`meta.rating IS NOT NULL`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	isExpr := q.Where.(*IsExpr)
	if !isExpr.IsNull {
		t.Error("should be IS NULL variant")
	}
	if !isExpr.Negated {
		t.Error("should be negated")
	}
}

func TestParser_TextSearch(t *testing.T) {
	q, err := Parse(`TEXT ~ "quarterly review"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	ts, ok := q.Where.(*TextSearchExpr)
	if !ok {
		t.Fatalf("expected TextSearchExpr, got %T", q.Where)
	}
	if ts.Value.Value != "quarterly review" {
		t.Errorf("expected %q, got %q", "quarterly review", ts.Value.Value)
	}
}

func TestParser_OrderByLimitOffset(t *testing.T) {
	q, err := Parse(`name = "a" ORDER BY created DESC, name ASC LIMIT 50 OFFSET 10`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.OrderBy) != 2 {
		t.Fatalf("expected 2 order clauses, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Field.Name() != "created" || q.OrderBy[0].Ascending {
		t.Error("first order should be created DESC")
	}
	if q.OrderBy[1].Field.Name() != "name" || !q.OrderBy[1].Ascending {
		t.Error("second order should be name ASC")
	}
	if q.Limit != 50 {
		t.Errorf("limit: expected 50, got %d", q.Limit)
	}
	if q.Offset != 10 {
		t.Errorf("offset: expected 10, got %d", q.Offset)
	}
}

func TestParser_MultiLevelTraversalRejected(t *testing.T) {
	_, err := Parse(`parent.parent.name = "a"`)
	if err == nil {
		t.Fatal("expected error for multi-level traversal")
	}
}

func TestParser_EmptyQuery(t *testing.T) {
	q, err := Parse(``)
	if err != nil {
		t.Fatalf("empty query should not error: %v", err)
	}
	if q.Where != nil {
		t.Error("empty query should have nil Where")
	}
}

func TestParser_TypeField(t *testing.T) {
	q, err := Parse(`type = resource AND name = "a"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	bin := q.Where.(*BinaryExpr)
	left := bin.Left.(*ComparisonExpr)
	if left.Field.Name() != "type" {
		t.Errorf("expected field 'type', got %q", left.Field.Name())
	}
}

func TestParser_ErrorPositions(t *testing.T) {
	_, err := Parse(`name = "a" AND AND`)
	if err == nil {
		t.Fatal("expected error")
	}
	pErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pErr.Pos == 0 {
		t.Error("error position should be non-zero")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestParser -v`
Expected: Compilation failure — `Parse` not defined

- [ ] **Step 3: Implement the parser**

```go
package mrql

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseError represents a parse error with source position.
type ParseError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at position %d: %s", e.Pos, e.Message)
}

// Parse parses an MRQL query string into a Query AST.
func Parse(input string) (*Query, error) {
	p := &parser{lexer: NewLexer(input), input: input}
	return p.parse()
}

type parser struct {
	lexer *Lexer
	input string
}

func (p *parser) parse() (*Query, error) {
	q := &Query{Limit: -1, Offset: -1}

	tok := p.lexer.Peek()
	if tok.Type == TokenEOF {
		return q, nil
	}

	// Parse WHERE clause (everything before ORDER BY / LIMIT / OFFSET)
	if tok.Type != TokenOrderBy && tok.Type != TokenLimit && tok.Type != TokenOffset {
		where, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		q.Where = where
	}

	// Parse ORDER BY
	tok = p.lexer.Peek()
	if tok.Type == TokenOrderBy {
		p.lexer.Next() // consume ORDER BY
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		q.OrderBy = orderBy
	}

	// Parse LIMIT
	tok = p.lexer.Peek()
	if tok.Type == TokenLimit {
		p.lexer.Next()
		numTok := p.lexer.Next()
		if numTok.Type != TokenNumber {
			return nil, &ParseError{Message: fmt.Sprintf("expected number after LIMIT, got %q", numTok.Value), Pos: numTok.Pos, Length: numTok.Length}
		}
		limit, err := strconv.Atoi(numTok.Value)
		if err != nil {
			return nil, &ParseError{Message: fmt.Sprintf("invalid LIMIT value: %q", numTok.Value), Pos: numTok.Pos, Length: numTok.Length}
		}
		q.Limit = limit
	}

	// Parse OFFSET
	tok = p.lexer.Peek()
	if tok.Type == TokenOffset {
		p.lexer.Next()
		numTok := p.lexer.Next()
		if numTok.Type != TokenNumber {
			return nil, &ParseError{Message: fmt.Sprintf("expected number after OFFSET, got %q", numTok.Value), Pos: numTok.Pos, Length: numTok.Length}
		}
		offset, err := strconv.Atoi(numTok.Value)
		if err != nil {
			return nil, &ParseError{Message: fmt.Sprintf("invalid OFFSET value: %q", numTok.Value), Pos: numTok.Pos, Length: numTok.Length}
		}
		q.Offset = offset
	}

	// Should be EOF now
	tok = p.lexer.Peek()
	if tok.Type != TokenEOF {
		return nil, &ParseError{Message: fmt.Sprintf("unexpected token %q after query", tok.Value), Pos: tok.Pos, Length: tok.Length}
	}

	return q, nil
}

// parseOr handles: expr (OR expr)*
func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.lexer.Peek().Type == TokenOr {
		op := p.lexer.Next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left, nil
}

// parseAnd handles: expr (AND expr)*
func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.lexer.Peek().Type == TokenAnd {
		op := p.lexer.Next()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left, nil
}

// parseNot handles: NOT expr | primary
func (p *parser) parseNot() (Node, error) {
	tok := p.lexer.Peek()
	if tok.Type == TokenNot {
		notTok := p.lexer.Next()
		expr, err := p.parseNot() // NOT binds tighter, allows NOT NOT
		if err != nil {
			return nil, err
		}
		return &NotExpr{Token: notTok, Expr: expr}, nil
	}
	return p.parsePrimary()
}

// parsePrimary handles: grouped expression, TEXT search, field-based expressions
func (p *parser) parsePrimary() (Node, error) {
	tok := p.lexer.Peek()

	// Parenthesized group
	if tok.Type == TokenLParen {
		p.lexer.Next() // consume (
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return nil, &ParseError{Message: "expected closing ')'", Pos: rp.Pos, Length: rp.Length}
		}
		return expr, nil
	}

	// TEXT ~ "..."
	if tok.Type == TokenText {
		return p.parseTextSearch()
	}

	// Field-based expression
	if tok.Type == TokenIdentifier || tok.Type == TokenType {
		return p.parseFieldExpr()
	}

	return nil, &ParseError{
		Message: fmt.Sprintf("expected field name or '(', got %q", tok.Value),
		Pos:     tok.Pos,
		Length:  tok.Length,
	}
}

func (p *parser) parseTextSearch() (Node, error) {
	textTok := p.lexer.Next() // TEXT

	likeTok := p.lexer.Next()
	if likeTok.Type != TokenLike {
		return nil, &ParseError{Message: fmt.Sprintf("expected '~' after TEXT, got %q", likeTok.Value), Pos: likeTok.Pos, Length: likeTok.Length}
	}

	strTok := p.lexer.Next()
	if strTok.Type != TokenString {
		return nil, &ParseError{Message: fmt.Sprintf("expected string after TEXT ~, got %q", strTok.Value), Pos: strTok.Pos, Length: strTok.Length}
	}

	return &TextSearchExpr{
		TextToken: textTok,
		Value:     &StringLiteral{Token: strTok, Value: strTok.Value},
	}, nil
}

func (p *parser) parseFieldExpr() (Node, error) {
	field, err := p.parseField()
	if err != nil {
		return nil, err
	}

	tok := p.lexer.Peek()

	// IS [NOT] EMPTY/NULL
	if tok.Type == TokenIs {
		return p.parseIsExpr(field)
	}

	// [NOT] IN (...)
	if tok.Type == TokenNot {
		// Peek further to see if this is NOT IN
		savedPos := p.lexer.Position()
		p.lexer.Next() // consume NOT
		nextTok := p.lexer.Peek()
		if nextTok.Type == TokenIn {
			return p.parseInExpr(field, true)
		}
		// Not "NOT IN" — rewind
		p.lexer.pos = savedPos
	}

	if tok.Type == TokenIn {
		return p.parseInExpr(field, false)
	}

	// Comparison operators: =, !=, >, >=, <, <=, ~, !~
	switch tok.Type {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte, TokenLike, TokenNotLike:
		op := p.lexer.Next()
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return &ComparisonExpr{Field: field, Operator: op, Value: value}, nil
	}

	return nil, &ParseError{
		Message: fmt.Sprintf("expected operator after %q, got %q", field.Name(), tok.Value),
		Pos:     tok.Pos,
		Length:  tok.Length,
	}
}

func (p *parser) parseField() (*FieldExpr, error) {
	tok := p.lexer.Next()
	if tok.Type != TokenIdentifier && tok.Type != TokenType {
		return nil, &ParseError{Message: fmt.Sprintf("expected field name, got %q", tok.Value), Pos: tok.Pos, Length: tok.Length}
	}

	parts := []Token{tok}

	for p.lexer.Peek().Type == TokenDot {
		p.lexer.Next() // consume .
		next := p.lexer.Next()
		if next.Type != TokenIdentifier && next.Type != TokenType {
			return nil, &ParseError{Message: fmt.Sprintf("expected field name after '.', got %q", next.Value), Pos: next.Pos, Length: next.Length}
		}
		parts = append(parts, next)

		// Enforce one-level traversal: max 2 parts for parent/children, max 2 for meta
		if len(parts) > 2 {
			return nil, &ParseError{
				Message: "Multi-level traversal is not supported. Use 'parent.<field>' for one level. Recursive traversal (ancestors/descendants) is planned for v2.",
				Pos:     parts[0].Pos,
				Length:  next.Pos + next.Length - parts[0].Pos,
			}
		}
	}

	return &FieldExpr{Parts: parts}, nil
}

func (p *parser) parseIsExpr(field *FieldExpr) (Node, error) {
	isTok := p.lexer.Next() // IS

	negated := false
	tok := p.lexer.Peek()
	if tok.Type == TokenNot {
		p.lexer.Next() // consume NOT
		negated = true
		tok = p.lexer.Peek()
	}

	switch tok.Type {
	case TokenEmpty:
		p.lexer.Next()
		return &IsExpr{Field: field, Negated: negated, IsNull: false, IsToken: isTok}, nil
	case TokenNull:
		p.lexer.Next()
		return &IsExpr{Field: field, Negated: negated, IsNull: true, IsToken: isTok}, nil
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected EMPTY or NULL after IS%s, got %q", map[bool]string{true: " NOT", false: ""}[negated], tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}
}

func (p *parser) parseInExpr(field *FieldExpr, negated bool) (Node, error) {
	inTok := p.lexer.Next() // IN

	lparen := p.lexer.Next()
	if lparen.Type != TokenLParen {
		return nil, &ParseError{Message: "expected '(' after IN", Pos: lparen.Pos, Length: lparen.Length}
	}

	var values []Node
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, val)

		tok := p.lexer.Peek()
		if tok.Type == TokenComma {
			p.lexer.Next() // consume comma
			continue
		}
		break
	}

	rparen := p.lexer.Next()
	if rparen.Type != TokenRParen {
		return nil, &ParseError{Message: "expected ')' to close IN list", Pos: rparen.Pos, Length: rparen.Length}
	}

	return &InExpr{Field: field, Negated: negated, Values: values, InToken: inTok}, nil
}

func (p *parser) parseValue() (Node, error) {
	tok := p.lexer.Next()
	switch tok.Type {
	case TokenString:
		return &StringLiteral{Token: tok, Value: tok.Value}, nil

	case TokenNumber:
		return p.parseNumberLiteral(tok)

	case TokenRelDate:
		return p.parseRelDateLiteral(tok)

	case TokenFunc:
		return &FuncCall{Token: tok, Name: tok.Value}, nil

	case TokenIdentifier:
		// Bare identifiers as values (e.g., type = resource)
		return &StringLiteral{Token: tok, Value: tok.Value}, nil

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected value (string, number, date, or function), got %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}
}

func (p *parser) parseNumberLiteral(tok Token) (*NumberLiteral, error) {
	raw := tok.Value
	var unit string
	numStr := raw

	// Extract unit suffix
	for _, u := range []string{"gb", "mb", "kb"} {
		if strings.HasSuffix(strings.ToLower(raw), u) {
			unit = strings.ToLower(raw[len(raw)-2:])
			numStr = raw[:len(raw)-2]
			break
		}
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, &ParseError{Message: fmt.Sprintf("invalid number: %q", raw), Pos: tok.Pos, Length: tok.Length}
	}

	rawBytes := int64(val)
	switch unit {
	case "kb":
		rawBytes = int64(val * 1024)
	case "mb":
		rawBytes = int64(val * 1024 * 1024)
	case "gb":
		rawBytes = int64(val * 1024 * 1024 * 1024)
	}

	return &NumberLiteral{Token: tok, Value: val, Unit: unit, Raw: rawBytes}, nil
}

func (p *parser) parseRelDateLiteral(tok Token) (*RelDateLiteral, error) {
	raw := tok.Value // e.g., "-7d"
	unitChar := raw[len(raw)-1:]
	amountStr := raw[1 : len(raw)-1]

	amount, err := strconv.Atoi(amountStr)
	if err != nil {
		return nil, &ParseError{Message: fmt.Sprintf("invalid relative date: %q", raw), Pos: tok.Pos, Length: tok.Length}
	}

	return &RelDateLiteral{Token: tok, Amount: amount, Unit: unitChar}, nil
}

func (p *parser) parseOrderBy() ([]OrderByClause, error) {
	var clauses []OrderByClause

	for {
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}

		ascending := true // default ASC
		tok := p.lexer.Peek()
		if tok.Type == TokenAsc {
			p.lexer.Next()
			ascending = true
		} else if tok.Type == TokenDesc {
			p.lexer.Next()
			ascending = false
		}

		clauses = append(clauses, OrderByClause{Field: field, Ascending: ascending})

		// Check for comma (more columns)
		if p.lexer.Peek().Type == TokenComma {
			p.lexer.Next()
			continue
		}
		break
	}

	return clauses, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestParser -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add mrql/parser.go mrql/parser_test.go
git commit -m "feat(mrql): implement recursive descent parser"
```

---

### Task 4: Field Definitions and Validator

**Files:**
- Create: `mrql/fields.go`
- Create: `mrql/validator.go`
- Create: `mrql/validator_test.go`

- [ ] **Step 1: Write validator tests**

```go
package mrql

import "testing"

func TestValidator_ValidQueries(t *testing.T) {
	tests := []string{
		`type = resource AND name = "a"`,
		`type = note AND noteType = "journal"`,
		`type = group AND parent.name = "Projects"`,
		`tags = "photo" AND created > -7d`,
		`type = resource AND fileSize > 10mb`,
		`type = resource AND contentType ~ "image/*"`,
		`type = group AND children.tags = "active"`,
		`meta.rating > 3`,
		`name = "test" ORDER BY created DESC LIMIT 50`,
		`TEXT ~ "search query"`,
		`tags IS EMPTY`,
		`category IS NULL`,
		`type = resource AND tags IN ("a", "b")`,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			q, err := Parse(input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := Validate(q); err != nil {
				t.Errorf("validation error: %v", err)
			}
		})
	}
}

func TestValidator_InvalidField(t *testing.T) {
	q, err := Parse(`type = resource AND nonexistent = "a"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Error("expected validation error for unknown field")
	}
}

func TestValidator_FieldNotOnEntity(t *testing.T) {
	q, err := Parse(`type = note AND contentType = "image/png"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Error("expected error: contentType not valid for notes")
	}
}

func TestValidator_TraversalOnNonGroup(t *testing.T) {
	q, err := Parse(`type = resource AND parent.name = "a"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Error("expected error: parent traversal not valid for resources")
	}
}

func TestValidator_InvalidEntityType(t *testing.T) {
	q, err := Parse(`type = foobar`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Error("expected error: invalid entity type")
	}
}

func TestValidator_CrossEntityAllowsCommonFields(t *testing.T) {
	q, err := Parse(`name = "test" AND tags = "photo"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("cross-entity common fields should be valid: %v", err)
	}
}

func TestValidator_CrossEntityRejectsSpecificFields(t *testing.T) {
	q, err := Parse(`contentType = "image/png"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Error("expected error: contentType requires type = resource")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestValidator -v`
Expected: Compilation failure

- [ ] **Step 3: Create `mrql/fields.go` with field definitions**

```go
package mrql

// FieldType represents the data type of a field.
type FieldType int

const (
	FieldString   FieldType = iota
	FieldNumber
	FieldDateTime
	FieldRelation // tags, groups, notes — supports IS EMPTY/IS NOT EMPTY
	FieldMeta     // meta.* — dynamic, any type
)

// FieldDef defines a field's properties.
type FieldDef struct {
	Name      string
	Type      FieldType
	Column    string // DB column name (e.g., "content_type" for contentType)
	TablePfx  string // table prefix for ambiguous queries (e.g., "resources.")
}

// EntityType is one of resource, note, group.
type EntityType string

const (
	EntityResource EntityType = "resource"
	EntityNote     EntityType = "note"
	EntityGroup    EntityType = "group"
)

// ValidEntityTypes lists accepted entity type values.
var ValidEntityTypes = map[string]EntityType{
	"resource": EntityResource,
	"note":     EntityNote,
	"group":    EntityGroup,
}

// commonFields are available on all entity types.
var commonFields = map[string]FieldDef{
	"name":        {Name: "name", Type: FieldString, Column: "name"},
	"description": {Name: "description", Type: FieldString, Column: "description"},
	"created":     {Name: "created", Type: FieldDateTime, Column: "created_at"},
	"updated":     {Name: "updated", Type: FieldDateTime, Column: "updated_at"},
	"tags":        {Name: "tags", Type: FieldRelation},
	"id":          {Name: "id", Type: FieldNumber, Column: "id"},
}

// entityFields maps entity type to fields specific to that entity.
var entityFields = map[EntityType]map[string]FieldDef{
	EntityResource: {
		"groups":       {Name: "groups", Type: FieldRelation},
		"group":        {Name: "group", Type: FieldRelation},
		"category":     {Name: "category", Type: FieldString, Column: "resource_category_id"},
		"contentType":  {Name: "contentType", Type: FieldString, Column: "content_type"},
		"fileSize":     {Name: "fileSize", Type: FieldNumber, Column: "file_size"},
		"width":        {Name: "width", Type: FieldNumber, Column: "width"},
		"height":       {Name: "height", Type: FieldNumber, Column: "height"},
		"originalName": {Name: "originalName", Type: FieldString, Column: "original_name"},
		"hash":         {Name: "hash", Type: FieldString, Column: "hash"},
	},
	EntityNote: {
		"groups":   {Name: "groups", Type: FieldRelation},
		"group":    {Name: "group", Type: FieldRelation},
		"noteType": {Name: "noteType", Type: FieldString, Column: "note_type_id"},
	},
	EntityGroup: {
		"category": {Name: "category", Type: FieldString, Column: "category_id"},
		"parent":   {Name: "parent", Type: FieldRelation},
		"children": {Name: "children", Type: FieldRelation},
	},
}

// LookupField finds a field definition for the given entity type.
// If entityType is empty (cross-entity query), only common fields are allowed.
func LookupField(entityType EntityType, fieldName string) (FieldDef, bool) {
	// meta.* fields are always valid
	if fieldName == "meta" {
		return FieldDef{Name: "meta", Type: FieldMeta}, true
	}

	// Check common fields
	if f, ok := commonFields[fieldName]; ok {
		return f, true
	}

	// If entity type specified, check entity-specific fields
	if entityType != "" {
		if fields, ok := entityFields[entityType]; ok {
			if f, ok := fields[fieldName]; ok {
				return f, true
			}
		}
	}

	return FieldDef{}, false
}

// IsCommonField returns true if the field is available on all entity types.
func IsCommonField(fieldName string) bool {
	_, ok := commonFields[fieldName]
	return ok || fieldName == "meta"
}
```

- [ ] **Step 4: Implement the validator**

```go
package mrql

import "fmt"

// ValidationError represents a semantic validation error with source position.
type ValidationError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error at position %d: %s", e.Pos, e.Message)
}

// Validate checks semantic correctness of a parsed query.
// It verifies field names exist for the target entity type and
// type-checks operators against field types.
func Validate(q *Query) error {
	entityType := extractEntityType(q.Where)

	if q.Where != nil {
		if err := validateNode(q.Where, entityType); err != nil {
			return err
		}
	}

	for _, ob := range q.OrderBy {
		if err := validateFieldAccess(ob.Field, entityType); err != nil {
			return err
		}
	}

	return nil
}

// extractEntityType walks the AST looking for a `type = <entity>` comparison.
func extractEntityType(node Node) EntityType {
	if node == nil {
		return ""
	}

	switch n := node.(type) {
	case *ComparisonExpr:
		if n.Field.Name() == "type" && n.Operator.Type == TokenEq {
			if sl, ok := n.Value.(*StringLiteral); ok {
				if et, ok := ValidEntityTypes[sl.Value]; ok {
					return et
				}
			}
		}
	case *BinaryExpr:
		if et := extractEntityType(n.Left); et != "" {
			return et
		}
		return extractEntityType(n.Right)
	case *NotExpr:
		return extractEntityType(n.Expr)
	}
	return ""
}

func validateNode(node Node, entityType EntityType) error {
	switch n := node.(type) {
	case *BinaryExpr:
		if err := validateNode(n.Left, entityType); err != nil {
			return err
		}
		return validateNode(n.Right, entityType)

	case *NotExpr:
		return validateNode(n.Expr, entityType)

	case *ComparisonExpr:
		if n.Field.Name() == "type" {
			// Validate entity type value
			if sl, ok := n.Value.(*StringLiteral); ok {
				if _, ok := ValidEntityTypes[sl.Value]; !ok {
					return &ValidationError{
						Message: fmt.Sprintf("invalid entity type %q; valid types are: resource, note, group", sl.Value),
						Pos:     sl.Token.Pos,
						Length:  sl.Token.Length,
					}
				}
			}
			return nil
		}
		return validateFieldAccess(n.Field, entityType)

	case *InExpr:
		return validateFieldAccess(n.Field, entityType)

	case *IsExpr:
		return validateFieldAccess(n.Field, entityType)

	case *TextSearchExpr:
		return nil // TEXT ~ always valid
	}

	return nil
}

func validateFieldAccess(field *FieldExpr, entityType EntityType) error {
	rootName := field.Parts[0].Value

	// Traversal (parent.X, children.X)
	if rootName == "parent" || rootName == "children" {
		if entityType != "" && entityType != EntityGroup {
			return &ValidationError{
				Message: fmt.Sprintf("%q traversal is only valid for groups", rootName),
				Pos:     field.Parts[0].Pos,
				Length:  field.Parts[0].Length,
			}
		}
		if len(field.Parts) > 1 {
			subField := field.Parts[1].Value
			if subField == "meta" {
				return nil // parent.meta.* handled dynamically
			}
			if _, ok := LookupField(EntityGroup, subField); !ok && !IsCommonField(subField) {
				return &ValidationError{
					Message: fmt.Sprintf("unknown field %q for group traversal", subField),
					Pos:     field.Parts[1].Pos,
					Length:  field.Parts[1].Length,
				}
			}
		}
		return nil
	}

	// Meta fields (meta.key)
	if rootName == "meta" {
		return nil // meta.* is always valid
	}

	// Regular field
	if entityType != "" {
		// Entity-specific validation
		if _, ok := LookupField(entityType, rootName); !ok {
			return &ValidationError{
				Message: fmt.Sprintf("unknown field %q for entity type %q", rootName, entityType),
				Pos:     field.Parts[0].Pos,
				Length:  field.Parts[0].Length,
			}
		}
	} else {
		// Cross-entity: only common fields allowed
		if !IsCommonField(rootName) {
			return &ValidationError{
				Message: fmt.Sprintf("field %q is not available in cross-entity queries; specify 'type = resource|note|group' first", rootName),
				Pos:     field.Parts[0].Pos,
				Length:  field.Parts[0].Length,
			}
		}
	}

	return nil
}

// ExtractEntityType is a public wrapper for extracting the entity type from a query.
func ExtractEntityType(q *Query) EntityType {
	return extractEntityType(q.Where)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestValidator -v`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add mrql/fields.go mrql/validator.go mrql/validator_test.go
git commit -m "feat(mrql): add field definitions and query validator"
```

---

### Task 5: Translator — AST to GORM Scopes

**Files:**
- Create: `mrql/translator.go`
- Create: `mrql/translator_test.go`

This is the largest task. The translator converts the validated AST into GORM query scopes, reusing existing `database_scopes` functions where possible.

- [ ] **Step 1: Write translator tests**

These tests use an in-memory SQLite database with test data.

```go
package mrql

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mahresources/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.Exec("PRAGMA foreign_keys = OFF")

	err = db.AutoMigrate(
		&models.Resource{},
		&models.Note{},
		&models.Group{},
		&models.Tag{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
	)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed test data
	tag1 := models.Tag{Name: "photo"}
	tag2 := models.Tag{Name: "video"}
	db.Create(&tag1)
	db.Create(&tag2)

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	rc := models.ResourceCategory{Name: "Images"}
	db.Create(&rc)

	r1 := models.Resource{Name: "sunset.jpg", ContentType: "image/jpeg", OriginalName: "sunset.jpg"}
	r1.CreatedAt = now
	r2 := models.Resource{Name: "old_photo.png", ContentType: "image/png", OriginalName: "old_photo.png"}
	r2.CreatedAt = weekAgo
	r3 := models.Resource{Name: "video.mp4", ContentType: "video/mp4", OriginalName: "video.mp4"}
	r3.CreatedAt = now
	db.Create(&r1)
	db.Create(&r2)
	db.Create(&r3)

	// Associate tags
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", r1.ID, tag1.ID)
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", r3.ID, tag2.ID)

	nt := models.NoteType{Name: "journal"}
	db.Create(&nt)

	n1 := models.Note{Name: "Meeting notes"}
	n1.NoteTypeId = &nt.ID
	db.Create(&n1)

	g1 := models.Group{Name: "Vacation"}
	db.Create(&g1)

	return db
}

func TestTranslator_SimpleNameFilter(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND name = "sunset.jpg"`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	if len(results) != 1 || results[0].Name != "sunset.jpg" {
		t.Errorf("expected 1 result 'sunset.jpg', got %d results", len(results))
	}
}

func TestTranslator_LikePattern(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND name ~ "*photo*"`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	if len(results) != 1 || results[0].Name != "old_photo.png" {
		t.Errorf("expected 1 result 'old_photo.png', got %v", results)
	}
}

func TestTranslator_ContentTypeFilter(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND contentType ~ "image/*"`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	if len(results) != 2 {
		t.Errorf("expected 2 image results, got %d", len(results))
	}
}

func TestTranslator_OrderByAndLimit(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource ORDER BY name ASC LIMIT 2`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "old_photo.png" {
		t.Errorf("first result should be 'old_photo.png', got %q", results[0].Name)
	}
}

func TestTranslator_TagsIsEmpty(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND tags IS EMPTY`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	// r2 (old_photo.png) has no tags
	if len(results) != 1 || results[0].Name != "old_photo.png" {
		t.Errorf("expected 1 untagged result, got %d", len(results))
	}
}

func TestTranslator_NotExpr(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND NOT contentType ~ "video/*"`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	if len(results) != 2 {
		t.Errorf("expected 2 non-video results, got %d", len(results))
	}
}

func TestTranslator_RelativeDate(t *testing.T) {
	db := setupTestDB(t)
	q, _ := Parse(`type = resource AND created > -3d`)
	Validate(q)

	var results []models.Resource
	tx, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	tx.Find(&results)
	// r1 (sunset) and r3 (video) were created now; r2 was created 7 days ago
	if len(results) != 2 {
		t.Errorf("expected 2 recent results, got %d", len(results))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestTranslator -v`
Expected: Compilation failure — `Translate` not defined

- [ ] **Step 3: Implement the translator**

```go
package mrql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TranslateOptions configures query translation.
type TranslateOptions struct {
	Timeout time.Duration // query execution timeout (default: 10s)
}

// Translate converts a parsed and validated MRQL Query into a GORM *gorm.DB
// ready for .Find() or .Count(). The returned *gorm.DB has all WHERE, ORDER BY,
// LIMIT, and OFFSET clauses applied.
func Translate(q *Query, db *gorm.DB) (*gorm.DB, error) {
	return TranslateWithOptions(q, db, TranslateOptions{Timeout: 10 * time.Second})
}

// TranslateWithOptions is like Translate but with configurable options.
func TranslateWithOptions(q *Query, db *gorm.DB, opts TranslateOptions) (*gorm.DB, error) {
	entityType := ExtractEntityType(q)

	// Set the base table
	tx := tableForEntity(db, entityType)

	// Apply timeout via context
	if opts.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		_ = cancel // caller should handle; we attach it to the query
		tx = tx.WithContext(ctx)
	}

	// Translate WHERE
	if q.Where != nil {
		var err error
		tx, err = translateNode(q.Where, tx, entityType, db)
		if err != nil {
			return nil, err
		}
	}

	// ORDER BY
	for _, ob := range q.OrderBy {
		col := fieldToColumn(ob.Field, entityType)
		dir := "ASC"
		if !ob.Ascending {
			dir = "DESC"
		}
		tx = tx.Order(col + " " + dir)
	}

	// LIMIT
	if q.Limit >= 0 {
		tx = tx.Limit(q.Limit)
	}

	// OFFSET
	if q.Offset >= 0 {
		tx = tx.Offset(q.Offset)
	}

	return tx, nil
}

func tableForEntity(db *gorm.DB, entityType EntityType) *gorm.DB {
	switch entityType {
	case EntityResource:
		return db.Model(&models_stub_resource{}).Table("resources")
	case EntityNote:
		return db.Model(&models_stub_note{}).Table("notes")
	case EntityGroup:
		return db.Model(&models_stub_group{}).Table("groups")
	default:
		// Cross-entity: caller will handle fan-out
		return db
	}
}

// Stub types to avoid importing models in the mrql package.
// The translator works with generic gorm queries.
type models_stub_resource struct{}
type models_stub_note struct{}
type models_stub_group struct{}

func translateNode(node Node, tx *gorm.DB, entityType EntityType, originalDB *gorm.DB) (*gorm.DB, error) {
	switch n := node.(type) {
	case *BinaryExpr:
		if n.Operator.Type == TokenAnd {
			var err error
			tx, err = translateNode(n.Left, tx, entityType, originalDB)
			if err != nil {
				return nil, err
			}
			return translateNode(n.Right, tx, entityType, originalDB)
		}
		// OR
		leftDB := originalDB.Session(&gorm.Session{NewDB: true})
		leftDB = tableForEntity(leftDB, entityType)
		leftDB, err := translateNode(n.Left, leftDB, entityType, originalDB)
		if err != nil {
			return nil, err
		}

		rightDB := originalDB.Session(&gorm.Session{NewDB: true})
		rightDB = tableForEntity(rightDB, entityType)
		rightDB, err = translateNode(n.Right, rightDB, entityType, originalDB)
		if err != nil {
			return nil, err
		}

		// Use GORM's Or with subqueries
		tx = tx.Where(leftDB).Or(rightDB)
		return tx, nil

	case *NotExpr:
		innerDB := originalDB.Session(&gorm.Session{NewDB: true})
		innerDB = tableForEntity(innerDB, entityType)
		innerDB, err := translateNode(n.Expr, innerDB, entityType, originalDB)
		if err != nil {
			return nil, err
		}
		tx = tx.Not(innerDB)
		return tx, nil

	case *ComparisonExpr:
		return translateComparison(n, tx, entityType, originalDB)

	case *InExpr:
		return translateIn(n, tx, entityType)

	case *IsExpr:
		return translateIs(n, tx, entityType, originalDB)

	case *TextSearchExpr:
		return translateTextSearch(n, tx)
	}

	return tx, nil
}

func translateComparison(n *ComparisonExpr, tx *gorm.DB, entityType EntityType, originalDB *gorm.DB) (*gorm.DB, error) {
	fieldName := n.Field.Name()

	// Skip type = ... (already handled by ExtractEntityType)
	if fieldName == "type" {
		return tx, nil
	}

	// Handle tag/group relationship comparisons
	if fieldName == "tags" || fieldName == "tag" {
		return translateTagComparison(n, tx, entityType, originalDB)
	}
	if fieldName == "groups" || fieldName == "group" {
		return translateGroupComparison(n, tx, entityType, originalDB)
	}

	col := fieldToColumn(n.Field, entityType)
	value, err := resolveValue(n.Value)
	if err != nil {
		return nil, err
	}

	likeOp := getLikeOperator(tx)

	switch n.Operator.Type {
	case TokenEq:
		tx = tx.Where("LOWER("+col+") = LOWER(?)", value)
	case TokenNeq:
		tx = tx.Where("LOWER("+col+") != LOWER(?)", value)
	case TokenGt:
		tx = tx.Where(col+" > ?", value)
	case TokenGte:
		tx = tx.Where(col+" >= ?", value)
	case TokenLt:
		tx = tx.Where(col+" < ?", value)
	case TokenLte:
		tx = tx.Where(col+" <= ?", value)
	case TokenLike:
		pattern := wildcardToLike(fmt.Sprintf("%v", value))
		tx = tx.Where(col+" "+likeOp+" ?", pattern)
	case TokenNotLike:
		pattern := wildcardToLike(fmt.Sprintf("%v", value))
		tx = tx.Where(col+" NOT "+likeOp+" ?", pattern)
	}

	return tx, nil
}

func translateTagComparison(n *ComparisonExpr, tx *gorm.DB, entityType EntityType, originalDB *gorm.DB) (*gorm.DB, error) {
	tagName, err := resolveStringValue(n.Value)
	if err != nil {
		return nil, err
	}

	junctionTable := tagJunctionTable(entityType)
	entityCol := tagEntityColumn(entityType)
	likeOp := getLikeOperator(tx)

	switch n.Operator.Type {
	case TokenEq:
		tx = tx.Where(entityCol+" IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN tags ON tags.id = jt.tag_id WHERE LOWER(tags.name) = LOWER(?))", tagName)
	case TokenNeq:
		tx = tx.Where(entityCol+" NOT IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN tags ON tags.id = jt.tag_id WHERE LOWER(tags.name) = LOWER(?))", tagName)
	case TokenLike:
		pattern := wildcardToLike(tagName)
		tx = tx.Where(entityCol+" IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN tags ON tags.id = jt.tag_id WHERE tags.name "+likeOp+" ?)", pattern)
	case TokenNotLike:
		pattern := wildcardToLike(tagName)
		tx = tx.Where(entityCol+" NOT IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN tags ON tags.id = jt.tag_id WHERE tags.name "+likeOp+" ?)", pattern)
	}

	return tx, nil
}

func translateGroupComparison(n *ComparisonExpr, tx *gorm.DB, entityType EntityType, originalDB *gorm.DB) (*gorm.DB, error) {
	groupName, err := resolveStringValue(n.Value)
	if err != nil {
		return nil, err
	}

	junctionTable := groupJunctionTable(entityType)
	entityCol := groupEntityColumn(entityType)
	likeOp := getLikeOperator(tx)

	switch n.Operator.Type {
	case TokenEq:
		tx = tx.Where(entityCol+" IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN groups ON groups.id = jt.group_id WHERE LOWER(groups.name) = LOWER(?))", groupName)
	case TokenNeq:
		tx = tx.Where(entityCol+" NOT IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN groups ON groups.id = jt.group_id WHERE LOWER(groups.name) = LOWER(?))", groupName)
	case TokenLike:
		pattern := wildcardToLike(groupName)
		tx = tx.Where(entityCol+" IN (SELECT "+entityCol+" FROM "+junctionTable+" jt JOIN groups ON groups.id = jt.group_id WHERE groups.name "+likeOp+" ?)", pattern)
	}

	return tx, nil
}

func translateIn(n *InExpr, tx *gorm.DB, entityType EntityType) (*gorm.DB, error) {
	col := fieldToColumn(n.Field, entityType)
	var values []any
	for _, v := range n.Values {
		val, err := resolveValue(v)
		if err != nil {
			return nil, err
		}
		values = append(values, val)
	}

	if n.Negated {
		tx = tx.Where(col+" NOT IN ?", values)
	} else {
		tx = tx.Where(col+" IN ?", values)
	}

	return tx, nil
}

func translateIs(n *IsExpr, tx *gorm.DB, entityType EntityType, originalDB *gorm.DB) (*gorm.DB, error) {
	fieldName := n.Field.Name()

	if n.IsNull {
		// IS NULL / IS NOT NULL — for scalar fields
		col := fieldToColumn(n.Field, entityType)
		if n.Negated {
			tx = tx.Where(col + " IS NOT NULL")
		} else {
			tx = tx.Where(col + " IS NULL")
		}
	} else {
		// IS EMPTY / IS NOT EMPTY — for relationship fields
		switch fieldName {
		case "tags":
			jt := tagJunctionTable(entityType)
			ec := tagEntityColumn(entityType)
			if n.Negated {
				tx = tx.Where(ec + " IN (SELECT " + ec + " FROM " + jt + ")")
			} else {
				tx = tx.Where(ec + " NOT IN (SELECT " + ec + " FROM " + jt + ")")
			}
		case "groups", "group":
			jt := groupJunctionTable(entityType)
			ec := groupEntityColumn(entityType)
			if n.Negated {
				tx = tx.Where(ec + " IN (SELECT " + ec + " FROM " + jt + ")")
			} else {
				tx = tx.Where(ec + " NOT IN (SELECT " + ec + " FROM " + jt + ")")
			}
		case "parent":
			if n.Negated {
				tx = tx.Where("owner_id IS NOT NULL")
			} else {
				tx = tx.Where("owner_id IS NULL")
			}
		case "children":
			if n.Negated {
				tx = tx.Where("id IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL)")
			} else {
				tx = tx.Where("id NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL)")
			}
		}
	}

	return tx, nil
}

func translateTextSearch(n *TextSearchExpr, tx *gorm.DB) (*gorm.DB, error) {
	// Sanitize FTS5 input: strip special operators
	searchTerm := sanitizeFTS5Input(n.Value.Value)
	if searchTerm == "" {
		return tx, nil
	}

	// Use FTS5 MATCH via a subquery on the appropriate FTS table
	tableName := tx.Statement.Table
	ftsTable := tableName + "_fts"
	tx = tx.Where("id IN (SELECT rowid FROM "+ftsTable+" WHERE "+ftsTable+" MATCH ?)", searchTerm)

	return tx, nil
}

// sanitizeFTS5Input strips FTS5-specific operators from user input.
func sanitizeFTS5Input(input string) string {
	// Remove FTS5 operators: NEAR, AND, OR, NOT, *, ^, "
	// Treat input as a plain phrase
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '"', '*', '^', '{', '}':
			return -1
		default:
			return r
		}
	}, input)

	// Remove FTS5 keywords when they appear as standalone words
	words := strings.Fields(cleaned)
	var filtered []string
	ftsKeywords := map[string]bool{"NEAR": true, "AND": true, "OR": true, "NOT": true}
	for _, w := range words {
		if !ftsKeywords[strings.ToUpper(w)] {
			filtered = append(filtered, w)
		}
	}
	return strings.Join(filtered, " ")
}

// resolveValue converts an AST value node to a Go value suitable for GORM parameters.
func resolveValue(node Node) (any, error) {
	switch n := node.(type) {
	case *StringLiteral:
		return n.Value, nil
	case *NumberLiteral:
		if n.Unit != "" {
			return n.Raw, nil // use byte-converted value
		}
		return n.Value, nil
	case *RelDateLiteral:
		return resolveRelDate(n), nil
	case *FuncCall:
		return resolveFunc(n), nil
	}
	return nil, fmt.Errorf("unsupported value type: %T", node)
}

func resolveStringValue(node Node) (string, error) {
	switch n := node.(type) {
	case *StringLiteral:
		return n.Value, nil
	}
	return "", fmt.Errorf("expected string value, got %T", node)
}

func resolveRelDate(n *RelDateLiteral) time.Time {
	now := time.Now()
	switch n.Unit {
	case "d":
		return now.AddDate(0, 0, -n.Amount)
	case "w":
		return now.AddDate(0, 0, -n.Amount*7)
	case "m":
		return now.AddDate(0, -n.Amount, 0)
	case "y":
		return now.AddDate(-n.Amount, 0, 0)
	}
	return now
}

func resolveFunc(n *FuncCall) time.Time {
	now := time.Now()
	switch n.Name {
	case "NOW()":
		return now
	case "START_OF_DAY()":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "START_OF_WEEK()":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location())
	case "START_OF_MONTH()":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case "START_OF_YEAR()":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	}
	return now
}

// wildcardToLike converts MRQL wildcards (* and ?) to SQL LIKE wildcards (% and _).
func wildcardToLike(pattern string) string {
	// First escape existing SQL wildcards
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	// Then convert MRQL wildcards
	pattern = strings.ReplaceAll(pattern, "*", "%")
	pattern = strings.ReplaceAll(pattern, "?", "_")
	return pattern
}

func fieldToColumn(field *FieldExpr, entityType EntityType) string {
	name := field.Name()
	prefix := tablePrefix(entityType)

	// Handle meta fields
	if strings.HasPrefix(name, "meta.") {
		key := strings.TrimPrefix(name, "meta.")
		return "json_extract(" + prefix + "meta, '$." + key + "')"
	}

	// Handle dotted traversal (parent.X, children.X) — handled separately in translator
	if strings.HasPrefix(name, "parent.") || strings.HasPrefix(name, "children.") {
		// These are handled as subqueries, not direct column access
		return name
	}

	// Look up field definition for column name
	if f, ok := LookupField(entityType, name); ok && f.Column != "" {
		return prefix + f.Column
	}

	// Fallback: snake_case the field name
	return prefix + toSnakeCase(name)
}

func tablePrefix(entityType EntityType) string {
	switch entityType {
	case EntityResource:
		return "resources."
	case EntityNote:
		return "notes."
	case EntityGroup:
		return "groups."
	}
	return ""
}

func tagJunctionTable(entityType EntityType) string {
	switch entityType {
	case EntityResource:
		return "resource_tags"
	case EntityNote:
		return "note_tags"
	case EntityGroup:
		return "group_tags"
	}
	return "resource_tags"
}

func tagEntityColumn(entityType EntityType) string {
	switch entityType {
	case EntityResource:
		return "resources.id"
	case EntityNote:
		return "notes.id"
	case EntityGroup:
		return "groups.id"
	}
	return "id"
}

func groupJunctionTable(entityType EntityType) string {
	switch entityType {
	case EntityResource:
		return "groups_related_resources"
	case EntityNote:
		return "group_notes"
	}
	return ""
}

func groupEntityColumn(entityType EntityType) string {
	switch entityType {
	case EntityResource:
		return "resources.id"
	case EntityNote:
		return "notes.id"
	}
	return "id"
}

func getLikeOperator(tx *gorm.DB) string {
	if tx.Config.Dialector.Name() == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestTranslator -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add mrql/translator.go mrql/translator_test.go
git commit -m "feat(mrql): implement AST-to-GORM translator"
```

---

### Task 6: Completion Engine

**Files:**
- Create: `mrql/completer.go`
- Create: `mrql/completer_test.go`

- [ ] **Step 1: Write completer tests**

```go
package mrql

import "testing"

func TestCompleter_AfterEmpty(t *testing.T) {
	suggestions := Complete("", 0)
	// Should suggest field names and TYPE keyword
	hasType := false
	hasName := false
	for _, s := range suggestions {
		if s.Value == "type" { hasType = true }
		if s.Value == "name" { hasName = true }
	}
	if !hasType || !hasName {
		t.Error("empty query should suggest 'type' and 'name'")
	}
}

func TestCompleter_AfterField(t *testing.T) {
	suggestions := Complete("name ", 5)
	// Should suggest operators
	hasEq := false
	for _, s := range suggestions {
		if s.Value == "=" { hasEq = true }
	}
	if !hasEq {
		t.Error("after field name, should suggest '='")
	}
}

func TestCompleter_AfterAND(t *testing.T) {
	suggestions := Complete("name = \"a\" AND ", 15)
	// Should suggest field names
	hasName := false
	for _, s := range suggestions {
		if s.Value == "name" { hasName = true }
	}
	if !hasName {
		t.Error("after AND, should suggest field names")
	}
}

func TestCompleter_AfterTypeEquals(t *testing.T) {
	suggestions := Complete("type = ", 7)
	// Should suggest entity types
	hasResource := false
	for _, s := range suggestions {
		if s.Value == "resource" { hasResource = true }
	}
	if !hasResource {
		t.Error("after 'type =', should suggest entity types")
	}
}

func TestCompleter_EntitySpecificFields(t *testing.T) {
	suggestions := Complete("type = resource AND ", 20)
	hasContentType := false
	for _, s := range suggestions {
		if s.Value == "contentType" { hasContentType = true }
	}
	if !hasContentType {
		t.Error("after 'type = resource AND', should suggest resource-specific fields like contentType")
	}
}

func TestCompleter_DateContext(t *testing.T) {
	suggestions := Complete("created >= ", 11)
	hasRelDate := false
	hasFunc := false
	for _, s := range suggestions {
		if s.Value == "-7d" { hasRelDate = true }
		if s.Value == "NOW()" { hasFunc = true }
	}
	if !hasRelDate || !hasFunc {
		t.Error("after date field operator, should suggest relative dates and functions")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestCompleter -v`
Expected: Compilation failure

- [ ] **Step 3: Implement the completer**

```go
package mrql

import "strings"

// Suggestion represents a single autocompletion suggestion.
type Suggestion struct {
	Value string `json:"value"`
	Type  string `json:"type"` // "field", "operator", "keyword", "entity_type", "value", "function", "rel_date"
	Label string `json:"label,omitempty"` // human-readable label
}

// Complete returns completion suggestions for the given query at the cursor position.
// This handles structural suggestions (fields, operators, keywords).
// Dynamic value suggestions (tag names, group names) are handled by the API layer.
func Complete(query string, cursor int) []Suggestion {
	// Parse what we can up to the cursor
	prefix := query[:cursor]
	trimmed := strings.TrimRight(prefix, " ")

	// Detect the entity type if set
	entityType := detectEntityTypeFromPrefix(prefix)

	// Determine context
	tokens := tokenizeForCompletion(trimmed)

	if len(tokens) == 0 {
		return fieldSuggestions(entityType)
	}

	last := tokens[len(tokens)-1]

	// After a boolean keyword (AND, OR, NOT) -> suggest fields
	if last.Type == TokenAnd || last.Type == TokenOr || last.Type == TokenNot {
		return fieldSuggestions(entityType)
	}

	// After an opening paren -> suggest fields
	if last.Type == TokenLParen {
		return fieldSuggestions(entityType)
	}

	// After a field name -> suggest operators
	if last.Type == TokenIdentifier || last.Type == TokenType {
		return operatorSuggestions()
	}

	// After a dot (meta., parent.) -> suggest sub-fields
	if last.Type == TokenDot && len(tokens) >= 2 {
		prev := tokens[len(tokens)-2]
		return dotFieldSuggestions(prev.Value, entityType)
	}

	// After an operator -> suggest values
	if isOperatorToken(last.Type) {
		return valueSuggestions(tokens, entityType)
	}

	// After a value -> suggest AND, OR, ORDER BY, LIMIT
	if last.Type == TokenString || last.Type == TokenNumber || last.Type == TokenRelDate ||
		last.Type == TokenFunc || last.Type == TokenRParen || last.Type == TokenEmpty || last.Type == TokenNull {
		return keywordSuggestions()
	}

	return fieldSuggestions(entityType)
}

func detectEntityTypeFromPrefix(prefix string) EntityType {
	lower := strings.ToLower(prefix)
	if strings.Contains(lower, "type = resource") || strings.Contains(lower, "type= resource") {
		return EntityResource
	}
	if strings.Contains(lower, "type = note") || strings.Contains(lower, "type= note") {
		return EntityNote
	}
	if strings.Contains(lower, "type = group") || strings.Contains(lower, "type= group") {
		return EntityGroup
	}
	return ""
}

func tokenizeForCompletion(input string) []Token {
	l := NewLexer(input)
	var tokens []Token
	for {
		tok := l.Next()
		if tok.Type == TokenEOF || tok.Type == TokenIllegal {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

func fieldSuggestions(entityType EntityType) []Suggestion {
	var suggestions []Suggestion

	// Common fields
	for name := range commonFields {
		suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
	}
	suggestions = append(suggestions, Suggestion{Value: "meta", Type: "field", Label: "meta.<key>"})
	suggestions = append(suggestions, Suggestion{Value: "type", Type: "field"})
	suggestions = append(suggestions, Suggestion{Value: "TEXT", Type: "keyword", Label: "TEXT ~ (full-text search)"})

	// Entity-specific fields
	if entityType != "" {
		if fields, ok := entityFields[entityType]; ok {
			for name := range fields {
				suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
			}
		}
	}

	return suggestions
}

func operatorSuggestions() []Suggestion {
	return []Suggestion{
		{Value: "=", Type: "operator"},
		{Value: "!=", Type: "operator"},
		{Value: ">", Type: "operator"},
		{Value: ">=", Type: "operator"},
		{Value: "<", Type: "operator"},
		{Value: "<=", Type: "operator"},
		{Value: "~", Type: "operator", Label: "~ (LIKE pattern)"},
		{Value: "!~", Type: "operator", Label: "!~ (NOT LIKE)"},
		{Value: "IN", Type: "keyword"},
		{Value: "NOT IN", Type: "keyword"},
		{Value: "IS", Type: "keyword", Label: "IS EMPTY/NULL"},
	}
}

func valueSuggestions(tokens []Token, entityType EntityType) []Suggestion {
	var suggestions []Suggestion

	// Check if the field before the operator is a date field
	fieldName := findFieldBeforeOperator(tokens)
	if fieldName == "created" || fieldName == "updated" {
		suggestions = append(suggestions,
			Suggestion{Value: "-7d", Type: "rel_date", Label: "7 days ago"},
			Suggestion{Value: "-30d", Type: "rel_date", Label: "30 days ago"},
			Suggestion{Value: "-3m", Type: "rel_date", Label: "3 months ago"},
			Suggestion{Value: "-1y", Type: "rel_date", Label: "1 year ago"},
			Suggestion{Value: "NOW()", Type: "function"},
			Suggestion{Value: "START_OF_DAY()", Type: "function"},
			Suggestion{Value: "START_OF_WEEK()", Type: "function"},
			Suggestion{Value: "START_OF_MONTH()", Type: "function"},
			Suggestion{Value: "START_OF_YEAR()", Type: "function"},
		)
		return suggestions
	}

	// After "type =", suggest entity types
	if fieldName == "type" {
		return []Suggestion{
			{Value: "resource", Type: "entity_type"},
			{Value: "note", Type: "entity_type"},
			{Value: "group", Type: "entity_type"},
		}
	}

	// Default: suggest format hint
	suggestions = append(suggestions, Suggestion{Value: "\"", Type: "value", Label: "string value"})
	return suggestions
}

func dotFieldSuggestions(parentField string, entityType EntityType) []Suggestion {
	switch parentField {
	case "meta":
		// Dynamic — API will provide actual keys
		return []Suggestion{{Value: "<key>", Type: "field", Label: "metadata key name"}}
	case "parent", "children":
		var suggestions []Suggestion
		for name := range commonFields {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
		}
		if fields, ok := entityFields[EntityGroup]; ok {
			for name := range fields {
				if name != "parent" && name != "children" {
					suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
				}
			}
		}
		return suggestions
	}
	return nil
}

func keywordSuggestions() []Suggestion {
	return []Suggestion{
		{Value: "AND", Type: "keyword"},
		{Value: "OR", Type: "keyword"},
		{Value: "ORDER BY", Type: "keyword"},
		{Value: "LIMIT", Type: "keyword"},
	}
}

func findFieldBeforeOperator(tokens []Token) string {
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Type == TokenIdentifier || tokens[i].Type == TokenType {
			return tokens[i].Value
		}
	}
	return ""
}

func isOperatorToken(tt TokenType) bool {
	switch tt {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte, TokenLike, TokenNotLike:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./mrql/ -run TestCompleter -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add mrql/completer.go mrql/completer_test.go
git commit -m "feat(mrql): implement completion engine for autocompletion"
```

---

### Task 7: SavedMRQLQuery Model and Application Context

**Files:**
- Create: `models/saved_mrql_query_model.go`
- Create: `application_context/mrql_context.go`
- Modify: `main.go` (add to AutoMigrate, add flag)

- [ ] **Step 1: Create the model**

```go
package models

import "time"

// SavedMRQLQuery stores a named MRQL query for later re-use.
type SavedMRQLQuery struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"index" json:"updatedAt"`
	Name        string    `gorm:"uniqueIndex:unique_mrql_query_name" json:"name"`
	Query       string    `json:"query"`
	Description string    `json:"description"`
}

func (q SavedMRQLQuery) GetId() uint        { return q.ID }
func (q SavedMRQLQuery) GetName() string     { return q.Name }
func (q SavedMRQLQuery) GetDescription() string { return q.Description }
```

- [ ] **Step 2: Create the application context**

```go
package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/mrql"
)

// MRQLQueryTimeout is the configurable timeout for MRQL query execution.
var MRQLQueryTimeout = 10 * time.Second

// MRQLExecuteResult holds the results of an MRQL query execution.
type MRQLExecuteResult struct {
	EntityType string        `json:"entityType"` // "resource", "note", "group", or "mixed"
	Results    []interface{} `json:"results"`
	Total      int           `json:"total"`
}

// ExecuteMRQL parses, validates, and executes an MRQL query string.
func (ctx *MahresourcesContext) ExecuteMRQL(queryStr string) (*MRQLExecuteResult, error) {
	q, err := mrql.Parse(queryStr)
	if err != nil {
		return nil, err
	}

	if err := mrql.Validate(q); err != nil {
		return nil, err
	}

	entityType := mrql.ExtractEntityType(q)

	opts := mrql.TranslateOptions{Timeout: MRQLQueryTimeout}

	if entityType == "" {
		return ctx.executeCrossEntity(q, opts)
	}

	return ctx.executeSingleEntity(q, entityType, opts)
}

func (ctx *MahresourcesContext) executeSingleEntity(q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions) (*MRQLExecuteResult, error) {
	db := ctx.GetDB()
	tx, err := mrql.TranslateWithOptions(q, db, opts)
	if err != nil {
		return nil, err
	}

	result := &MRQLExecuteResult{EntityType: string(entityType)}

	switch entityType {
	case mrql.EntityResource:
		var items []models.Resource
		if err := tx.Find(&items).Error; err != nil {
			return nil, checkTimeout(err)
		}
		for _, item := range items {
			result.Results = append(result.Results, item)
		}
		result.Total = len(items)
	case mrql.EntityNote:
		var items []models.Note
		if err := tx.Find(&items).Error; err != nil {
			return nil, checkTimeout(err)
		}
		for _, item := range items {
			result.Results = append(result.Results, item)
		}
		result.Total = len(items)
	case mrql.EntityGroup:
		var items []models.Group
		if err := tx.Find(&items).Error; err != nil {
			return nil, checkTimeout(err)
		}
		for _, item := range items {
			result.Results = append(result.Results, item)
		}
		result.Total = len(items)
	}

	return result, nil
}

func (ctx *MahresourcesContext) executeCrossEntity(q *mrql.Query, opts mrql.TranslateOptions) (*MRQLExecuteResult, error) {
	result := &MRQLExecuteResult{EntityType: "mixed"}
	db := ctx.GetDB()

	for _, et := range []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup} {
		tx, err := mrql.TranslateWithOptions(q, db, opts)
		if err != nil {
			continue
		}

		switch et {
		case mrql.EntityResource:
			tx = tx.Table("resources")
			var items []models.Resource
			if err := tx.Find(&items).Error; err == nil {
				for _, item := range items {
					result.Results = append(result.Results, map[string]any{
						"type":    "resource",
						"id":      item.ID,
						"name":    item.Name,
						"created": item.CreatedAt,
					})
				}
			}
		case mrql.EntityNote:
			tx = tx.Table("notes")
			var items []models.Note
			if err := tx.Find(&items).Error; err == nil {
				for _, item := range items {
					result.Results = append(result.Results, map[string]any{
						"type":    "note",
						"id":      item.ID,
						"name":    item.Name,
						"created": item.CreatedAt,
					})
				}
			}
		case mrql.EntityGroup:
			tx = tx.Table("groups")
			var items []models.Group
			if err := tx.Find(&items).Error; err == nil {
				for _, item := range items {
					result.Results = append(result.Results, map[string]any{
						"type":    "group",
						"id":      item.ID,
						"name":    item.Name,
						"created": item.CreatedAt,
					})
				}
			}
		}
	}

	result.Total = len(result.Results)
	return result, nil
}

// ValidateMRQL parses and validates an MRQL query, returning errors with positions.
func (ctx *MahresourcesContext) ValidateMRQL(queryStr string) []mrql.ValidationError {
	q, err := mrql.Parse(queryStr)
	if err != nil {
		if pErr, ok := err.(*mrql.ParseError); ok {
			return []mrql.ValidationError{{Message: pErr.Message, Pos: pErr.Pos, Length: pErr.Length}}
		}
		return []mrql.ValidationError{{Message: err.Error()}}
	}

	if err := mrql.Validate(q); err != nil {
		if vErr, ok := err.(*mrql.ValidationError); ok {
			return []mrql.ValidationError{*vErr}
		}
		return []mrql.ValidationError{{Message: err.Error()}}
	}

	return nil
}

// CompleteMRQL returns completion suggestions for the given query and cursor position.
func (ctx *MahresourcesContext) CompleteMRQL(queryStr string, cursor int) []mrql.Suggestion {
	suggestions := mrql.Complete(queryStr, cursor)

	// Augment with dynamic values from the database
	// (tag names, group names, etc. based on context)
	// This is done by checking what kind of value is expected

	return suggestions
}

// --- Saved MRQL Queries ---

func (ctx *MahresourcesContext) CreateSavedMRQLQuery(name, query, description string) (*models.SavedMRQLQuery, error) {
	saved := &models.SavedMRQLQuery{
		Name:        name,
		Query:       query,
		Description: description,
	}
	if err := ctx.GetDB().Create(saved).Error; err != nil {
		return nil, err
	}
	return saved, nil
}

func (ctx *MahresourcesContext) GetSavedMRQLQueries() ([]models.SavedMRQLQuery, error) {
	var queries []models.SavedMRQLQuery
	if err := ctx.GetDB().Order("created_at desc").Find(&queries).Error; err != nil {
		return nil, err
	}
	return queries, nil
}

func (ctx *MahresourcesContext) GetSavedMRQLQuery(id uint) (*models.SavedMRQLQuery, error) {
	var query models.SavedMRQLQuery
	if err := ctx.GetDB().First(&query, id).Error; err != nil {
		return nil, err
	}
	return &query, nil
}

func (ctx *MahresourcesContext) UpdateSavedMRQLQuery(id uint, name, queryStr, description string) (*models.SavedMRQLQuery, error) {
	var query models.SavedMRQLQuery
	if err := ctx.GetDB().First(&query, id).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	if queryStr != "" {
		updates["query"] = queryStr
	}
	if description != "" {
		updates["description"] = description
	}

	if err := ctx.GetDB().Model(&query).Updates(updates).Error; err != nil {
		return nil, err
	}

	return &query, nil
}

func (ctx *MahresourcesContext) DeleteSavedMRQLQuery(id uint) error {
	return ctx.GetDB().Delete(&models.SavedMRQLQuery{}, id).Error
}

func checkTimeout(err error) error {
	if err == context.DeadlineExceeded {
		return fmt.Errorf("query timed out: the query was too expensive; try adding more filters or using LIMIT")
	}
	return err
}

// GetDB returns the GORM database handle. This method must already exist on MahresourcesContext.
// If not, it should be added. Verify by checking application_context/context.go.
func (ctx *MahresourcesContext) getDB() *gorm.DB {
	return ctx.db
}
```

Note: The implementer should check how `ctx.db` or `ctx.GetDB()` is accessed in the existing codebase. The existing pattern uses `ctx.db` as a private field — look at other context files (e.g., `resource_context.go`) to see the exact access pattern and adjust accordingly.

- [ ] **Step 3: Add SavedMRQLQuery to AutoMigrate in main.go**

In `main.go`, add `&models.SavedMRQLQuery{}` to the `db.AutoMigrate(...)` call, after `&models.PluginKV{}`:

```go
&models.PluginKV{},
&models.SavedMRQLQuery{},
```

Also add the `-mrql-query-timeout` flag near other flag definitions:

```go
mrqlTimeout := flag.Duration("mrql-query-timeout", 10*time.Second, "Maximum execution time for MRQL queries")
```

And after flags are parsed, set:

```go
application_context.MRQLQueryTimeout = *mrqlTimeout
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add models/saved_mrql_query_model.go application_context/mrql_context.go main.go
git commit -m "feat(mrql): add saved query model and application context"
```

---

### Task 8: API Handlers

**Files:**
- Create: `server/api_handlers/mrql_api_handlers.go`
- Create: `server/interfaces/mrql_interfaces.go`
- Modify: `server/routes.go`

- [ ] **Step 1: Create MRQL interfaces**

```go
package interfaces

import "mahresources/mrql"

// MRQLExecutor executes MRQL queries.
type MRQLExecutor interface {
	ExecuteMRQL(query string) (interface{}, error)
	ValidateMRQL(query string) []mrql.ValidationError
	CompleteMRQL(query string, cursor int) []mrql.Suggestion
}

// MRQLSavedQueryManager manages saved MRQL queries.
type MRQLSavedQueryManager interface {
	CreateSavedMRQLQuery(name, query, description string) (interface{}, error)
	GetSavedMRQLQueries() (interface{}, error)
	GetSavedMRQLQuery(id uint) (interface{}, error)
	UpdateSavedMRQLQuery(id uint, name, query, description string) (interface{}, error)
	DeleteSavedMRQLQuery(id uint) error
}
```

- [ ] **Step 2: Create MRQL API handlers**

```go
package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

type mrqlExecuteRequest struct {
	Query string `json:"query"`
}

type mrqlCompleteRequest struct {
	Query  string `json:"query"`
	Cursor int    `json:"cursor"`
}

type mrqlValidateRequest struct {
	Query string `json:"query"`
}

type mrqlSaveRequest struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description"`
}

func GetMRQLExecuteHandler(ctx interfaces.MRQLExecutor) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mrqlExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		result, err := ctx.ExecuteMRQL(req.Query)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(result)
	}
}

func GetMRQLValidateHandler(ctx interfaces.MRQLExecutor) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mrqlValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		errors := ctx.ValidateMRQL(req.Query)
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(map[string]any{
			"valid":  len(errors) == 0,
			"errors": errors,
		})
	}
}

func GetMRQLCompleteHandler(ctx interfaces.MRQLExecutor) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mrqlCompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		suggestions := ctx.CompleteMRQL(req.Query, req.Cursor)
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(map[string]any{
			"suggestions": suggestions,
		})
	}
}

func GetMRQLSavedListHandler(ctx interfaces.MRQLSavedQueryManager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		queries, err := ctx.GetSavedMRQLQueries()
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(queries)
	}
}

func GetMRQLSavedGetHandler(ctx interfaces.MRQLSavedQueryManager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(r, "id", 0))
		query, err := ctx.GetSavedMRQLQuery(id)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(query)
	}
}

func GetMRQLSavedCreateHandler(ctx interfaces.MRQLSavedQueryManager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mrqlSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		query, err := ctx.CreateSavedMRQLQuery(req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(query)
	}
}

func GetMRQLSavedUpdateHandler(ctx interfaces.MRQLSavedQueryManager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(r, "id", 0))
		var req mrqlSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		query, err := ctx.UpdateSavedMRQLQuery(id, req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(query)
	}
}

func GetMRQLSavedDeleteHandler(ctx interfaces.MRQLSavedQueryManager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(r, "id", 0))
		if err := ctx.DeleteSavedMRQLQuery(id); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(map[string]any{"id": id})
	}
}

func GetMRQLSavedRunHandler(ctx interface {
	interfaces.MRQLSavedQueryManager
	interfaces.MRQLExecutor
}) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(r, "id", 0))
		saved, err := ctx.GetSavedMRQLQuery(id)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		// The saved query has a .Query field — extract it
		queryBytes, _ := json.Marshal(saved)
		var sq struct{ Query string }
		json.Unmarshal(queryBytes, &sq)

		result, err := ctx.ExecuteMRQL(sq.Query)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(w).Encode(result)
	}
}
```

- [ ] **Step 3: Register routes in `server/routes.go`**

Add after the existing query routes section (around line 350):

```go
// MRQL routes
router.Methods(http.MethodPost).Path("/v1/mrql").HandlerFunc(api_handlers.GetMRQLExecuteHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/mrql/validate").HandlerFunc(api_handlers.GetMRQLValidateHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/mrql/complete").HandlerFunc(api_handlers.GetMRQLCompleteHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetMRQLSavedListHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/mrql/saved/{id}").HandlerFunc(api_handlers.GetMRQLSavedGetHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetMRQLSavedCreateHandler(appContext))
router.Methods(http.MethodPut).Path("/v1/mrql/saved/{id}").HandlerFunc(api_handlers.GetMRQLSavedUpdateHandler(appContext))
router.Methods(http.MethodDelete).Path("/v1/mrql/saved/{id}").HandlerFunc(api_handlers.GetMRQLSavedDeleteHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/mrql/saved/{id}/run").HandlerFunc(api_handlers.GetMRQLSavedRunHandler(appContext))
```

Also add the MRQL template page route to the `templates` map:

```go
"/mrql": {template_context_providers.MRQLContextProvider, "mrql.tpl", http.MethodGet},
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: No errors (may need to create template context provider stub — see Task 9)

- [ ] **Step 5: Commit**

```bash
git add server/api_handlers/mrql_api_handlers.go server/interfaces/mrql_interfaces.go server/routes.go
git commit -m "feat(mrql): add API handlers and route registration"
```

---

### Task 9: Template and Frontend Page

**Files:**
- Create: `server/template_handlers/template_context_providers/mrql_template_context.go`
- Create: `templates/mrql.tpl`
- Create: `src/components/mrqlEditor.js`
- Modify: `src/main.js`
- Modify: `vite.config.js`

This task creates the MRQL query page with CodeMirror editor. The exact template HTML and CodeMirror integration code should follow the patterns in `templates/listResources.tpl` and `src/components/codeEditor.js`.

- [ ] **Step 1: Create template context provider**

```go
package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

func MRQLContextProvider(ctx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		savedQueries, _ := ctx.GetSavedMRQLQueries()
		return pongo2.Context{
			"title":        "MRQL Query",
			"savedQueries": savedQueries,
		}
	}
}
```

- [ ] **Step 2: Create the MRQL template**

Create `templates/mrql.tpl` following the existing template patterns. The template should include:
- A CodeMirror editor container (`x-data="mrqlEditor()"`)
- Run and Save buttons
- A results area
- A collapsible syntax help panel
- Saved queries section

The implementer should look at `templates/listResources.tpl` and `templates/createQuery.tpl` for the exact layout patterns, header includes, and pongo2 syntax used in the project.

- [ ] **Step 3: Create the mrqlEditor Alpine component**

Create `src/components/mrqlEditor.js` following the pattern in `src/components/codeEditor.js`:
- Lazy-load CodeMirror modules
- Create a custom language mode for MRQL (keyword highlighting for AND, OR, NOT, etc.)
- Set up autocompletion that calls `/v1/mrql/complete`
- Set up validation that calls `/v1/mrql/validate` on change (debounced)
- Add Cmd/Ctrl+Enter keybinding to execute
- Handle results display and pagination

- [ ] **Step 4: Register the component in `src/main.js`**

Add import:
```javascript
import { mrqlEditor } from './components/mrqlEditor.js';
```

Add registration:
```javascript
Alpine.data('mrqlEditor', mrqlEditor);
```

- [ ] **Step 5: Add mrql chunk to vite.config.js**

In the `manualChunks` function:
```javascript
if (id.includes('/diff/')) return 'diff';
if (id.includes('mrqlEditor')) return 'mrql';
```

- [ ] **Step 6: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add server/template_handlers/template_context_providers/mrql_template_context.go templates/mrql.tpl src/components/mrqlEditor.js src/main.js vite.config.js
git commit -m "feat(mrql): add query page with CodeMirror editor"
```

---

### Task 10: CLI Command

**Files:**
- Create: `cmd/mr/commands/mrql.go`
- Modify: `cmd/mr/main.go`

- [ ] **Step 1: Create the MRQL CLI command**

Create `cmd/mr/commands/mrql.go` following the patterns in `cmd/mr/commands/search.go` and `cmd/mr/commands/queries.go`. The command should support:

- `mr mrql '<query>'` — execute inline query
- `mr mrql -f <file>` — read query from file
- `mr mrql -` — read query from stdin
- `mr mrql save <name> <query>` — save a query
- `mr mrql list` — list saved queries
- `mr mrql run <name-or-id>` — run a saved query
- `mr mrql delete <id>` — delete a saved query

Use the existing `client.Post()` method to call `/v1/mrql` and `output.Print()` for table output.

- [ ] **Step 2: Register in `cmd/mr/main.go`**

Add after the existing `rootCmd.AddCommand(commands.NewSearchCmd(c, opts))`:

```go
rootCmd.AddCommand(commands.NewMRQLCmd(c, opts, &page))
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build -o mr ./cmd/mr/`
Expected: Binary builds successfully

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/mrql.go cmd/mr/main.go
git commit -m "feat(mrql): add mr mrql CLI command"
```

---

### Task 11: Documentation Site

**Files:**
- Create: `docs-site/docs/features/mrql.md`
- Modify: `docs-site/docs/features/cli.md`
- Modify: `docs-site/sidebars.ts`

- [ ] **Step 1: Create MRQL documentation page**

Create `docs-site/docs/features/mrql.md` with:
1. Overview — what MRQL is and when to use it
2. Syntax reference — fields, operators, wildcards, dates, functions, traversal
3. Full-text search — TEXT ~ vs ~
4. Cross-entity queries
5. Saved queries
6. Examples cookbook (15-20 real-world queries)

Use the spec document as the source of truth for all syntax details.

- [ ] **Step 2: Update CLI docs**

Add `mr mrql` section to `docs-site/docs/features/cli.md` with command syntax, output modes, and piping examples.

- [ ] **Step 3: Update sidebar**

Add MRQL to the Advanced Features section in `docs-site/sidebars.ts`.

- [ ] **Step 4: Verify docs build**

Run: `cd /Users/egecan/Code/mahresources/docs-site && npm run build`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add docs-site/docs/features/mrql.md docs-site/docs/features/cli.md docs-site/sidebars.ts
git commit -m "docs: add MRQL query language documentation"
```

---

### Task 12: E2E Browser Tests

**Files:**
- Create: `e2e/pages/mrql.page.ts`
- Create: `e2e/tests/mrql.spec.ts`

- [ ] **Step 1: Create page object model**

Create `e2e/pages/mrql.page.ts` with methods for interacting with the MRQL page: entering queries in the CodeMirror editor, clicking Run, reading results, saving queries, etc. Follow the pattern of existing page objects in `e2e/pages/`.

- [ ] **Step 2: Create browser E2E tests**

Create `e2e/tests/mrql.spec.ts` covering:
- Page loads and CodeMirror editor renders
- Enter and execute a simple query, verify results
- Enter an invalid query, verify error marker appears
- Save a query, verify it appears in saved list
- Run a saved query
- Pagination works
- Cmd+Enter keyboard shortcut executes

- [ ] **Step 3: Run the tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server -- --grep mrql`
Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add e2e/pages/mrql.page.ts e2e/tests/mrql.spec.ts
git commit -m "test: add MRQL browser E2E tests"
```

---

### Task 13: E2E CLI Tests

**Files:**
- Create: `e2e/tests/cli/cli-mrql.spec.ts`

- [ ] **Step 1: Create CLI E2E tests**

Create `e2e/tests/cli/cli-mrql.spec.ts` following the pattern in existing CLI tests (e.g., `cli-search.spec.ts`, `cli-queries.spec.ts`). Cover:

- `mr mrql 'name ~ "*test*"'` returns results
- `mr mrql ... --json` returns valid JSON
- `mr mrql ... --quiet` returns only IDs
- `mr mrql -f query.mrql` reads from file
- `echo '...' | mr mrql -` reads from stdin
- Save/list/run/delete lifecycle
- Error handling for invalid syntax

- [ ] **Step 2: Run the tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:cli -- --grep mrql`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-mrql.spec.ts
git commit -m "test: add MRQL CLI E2E tests"
```

---

### Task 14: Accessibility Tests

**Files:**
- Create: `e2e/tests/accessibility/mrql-a11y.spec.ts`

- [ ] **Step 1: Create accessibility tests**

Create `e2e/tests/accessibility/mrql-a11y.spec.ts` following the pattern of existing a11y tests:
- axe-core scan on the MRQL page (no critical/serious violations)
- CodeMirror editor has proper ARIA label
- Results table has proper heading/scope attributes
- Error messages are accessible

- [ ] **Step 2: Run the tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:a11y -- --grep mrql`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/accessibility/mrql-a11y.spec.ts
git commit -m "test: add MRQL accessibility tests"
```

---

### Task 15: Integration Verification

**Files:** None new — this is a verification task.

- [ ] **Step 1: Run all Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./...`
Expected: All tests PASS

- [ ] **Step 2: Build the full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds (CSS + JS + Go binary)

- [ ] **Step 3: Run all E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:all`
Expected: All tests PASS (browser + CLI)

- [ ] **Step 4: Verify the MRQL page manually**

Start the server in ephemeral mode and verify:
1. Navigate to `/mrql`
2. Type a query in the editor
3. Execute it and see results
4. Save a query
5. Run the saved query

- [ ] **Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: integration fixes for MRQL feature"
```
