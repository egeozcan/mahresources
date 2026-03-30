package mrql

import (
	"testing"
)

// helper: collect all tokens until EOF
func tokenize(t *testing.T, input string) []Token {
	t.Helper()
	l := NewLexer(input)
	var tokens []Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF || tok.Type == TokenIllegal {
			break
		}
	}
	return tokens
}

// helper: collect all tokens including EOF (for position tests)
func tokenizeAll(t *testing.T, input string) []Token {
	t.Helper()
	l := NewLexer(input)
	var tokens []Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens
}

// TestLexerOperators tests all comparison operators.
func TestLexerOperators(t *testing.T) {
	tests := []struct {
		input    string
		wantType TokenType
		wantVal  string
	}{
		{"=", TokenEq, "="},
		{"!=", TokenNeq, "!="},
		{">", TokenGt, ">"},
		{">=", TokenGte, ">="},
		{"<", TokenLt, "<"},
		{"<=", TokenLte, "<="},
		{"~", TokenLike, "~"},
		{"!~", TokenNotLike, "!~"},
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

// TestLexerDelimiters tests parentheses, comma, dot.
func TestLexerDelimiters(t *testing.T) {
	tests := []struct {
		ch      string
		want    TokenType
	}{
		{"(", TokenLParen},
		{")", TokenRParen},
		{",", TokenComma},
		{".", TokenDot},
	}
	for _, tc := range tests {
		t.Run(tc.ch, func(t *testing.T) {
			l := NewLexer(tc.ch)
			tok := l.Next()
			if tok.Type != tc.want {
				t.Errorf("input=%q: got %v, want %v", tc.ch, tok.Type, tc.want)
			}
			if tok.Value != tc.ch {
				t.Errorf("input=%q: got value %q, want %q", tc.ch, tok.Value, tc.ch)
			}
		})
	}
}

// TestLexerKeywords tests all keyword tokens (case-insensitive).
func TestLexerKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"AND", TokenAnd},
		{"and", TokenAnd},
		{"And", TokenAnd},
		{"OR", TokenOr},
		{"or", TokenOr},
		{"NOT", TokenNot},
		{"not", TokenNot},
		{"IN", TokenIn},
		{"in", TokenIn},
		{"IS", TokenIs},
		{"is", TokenIs},
		{"EMPTY", TokenEmpty},
		{"empty", TokenEmpty},
		{"NULL", TokenNull},
		{"null", TokenNull},
		{"ORDER BY", TokenOrderBy},
		{"order by", TokenOrderBy},
		{"Order By", TokenOrderBy},
		{"ASC", TokenAsc},
		{"asc", TokenAsc},
		{"DESC", TokenDesc},
		{"desc", TokenDesc},
		{"LIMIT", TokenLimit},
		{"limit", TokenLimit},
		{"OFFSET", TokenOffset},
		{"offset", TokenOffset},
		{"TEXT", TokenText},
		{"text", TokenText},
		{"TYPE", TokenKwType},
		{"type", TokenKwType},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLexer(tc.input)
			tok := l.Next()
			if tok.Type != tc.want {
				t.Errorf("input=%q: got type %v, want %v", tc.input, tok.Type, tc.want)
			}
		})
	}
}

// TestLexerOrderByTokenization ensures ORDER BY produces a single token.
func TestLexerOrderByTokenization(t *testing.T) {
	input := "ORDER BY name ASC"
	l := NewLexer(input)

	tok := l.Next()
	if tok.Type != TokenOrderBy {
		t.Fatalf("expected TokenOrderBy, got %v (value=%q)", tok.Type, tok.Value)
	}
	if tok.Value != "ORDER BY" {
		t.Errorf("expected value 'ORDER BY', got %q", tok.Value)
	}

	tok = l.Next()
	if tok.Type != TokenIdentifier {
		t.Fatalf("expected TokenIdentifier for 'name', got %v", tok.Type)
	}

	tok = l.Next()
	if tok.Type != TokenAsc {
		t.Fatalf("expected TokenAsc, got %v", tok.Type)
	}
}

// TestLexerIdentifier tests that unknown words become identifiers.
func TestLexerIdentifier(t *testing.T) {
	tests := []string{"name", "contentType", "fileSize", "myField123", "_private"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			l := NewLexer(input)
			tok := l.Next()
			if tok.Type != TokenIdentifier {
				t.Errorf("input=%q: got %v, want TokenIdentifier", input, tok.Type)
			}
			if tok.Value != input {
				t.Errorf("input=%q: got value %q, want %q", input, tok.Value, input)
			}
		})
	}
}

// TestLexerStringLiterals tests quoted strings with escape handling.
func TestLexerStringLiterals(t *testing.T) {
	tests := []struct {
		input   string
		wantVal string
		illegal bool
	}{
		{`"hello"`, "hello", false},
		{`"with spaces"`, "with spaces", false},
		{`"escape \"quote\""`, `escape "quote"`, false},
		{`"back\\slash"`, `back\slash`, false},
		{`"mixed \"and\" \\back"`, `mixed "and" \back`, false},
		{`"unterminated`, "", true}, // unterminated → TokenIllegal
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLexer(tc.input)
			tok := l.Next()
			if tc.illegal {
				if tok.Type != TokenIllegal {
					t.Errorf("input=%q: expected TokenIllegal, got %v", tc.input, tok.Type)
				}
				return
			}
			if tok.Type != TokenString {
				t.Errorf("input=%q: got %v, want TokenString", tc.input, tok.Type)
			}
			if tok.Value != tc.wantVal {
				t.Errorf("input=%q: got value %q, want %q", tc.input, tok.Value, tc.wantVal)
			}
		})
	}
}

// TestLexerNumbers tests integer, float, and unit-suffixed numbers.
func TestLexerNumbers(t *testing.T) {
	tests := []struct {
		input   string
		wantVal string
	}{
		{"42", "42"},
		{"3.14", "3.14"},
		{"100", "100"},
		{"10mb", "10mb"},
		{"5gb", "5gb"},
		{"100kb", "100kb"},
		{"1.5mb", "1.5mb"},
		{"2GB", "2GB"},
		{"512KB", "512KB"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLexer(tc.input)
			tok := l.Next()
			if tok.Type != TokenNumber {
				t.Errorf("input=%q: got %v, want TokenNumber", tc.input, tok.Type)
			}
			if tok.Value != tc.wantVal {
				t.Errorf("input=%q: got value %q, want %q", tc.input, tok.Value, tc.wantVal)
			}
		})
	}
}

// TestLexerRelativeDates tests relative date tokens.
func TestLexerRelativeDates(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"-7d"},
		{"-30d"},
		{"-3m"},
		{"-1y"},
		{"-2w"},
		{"-14d"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLexer(tc.input)
			tok := l.Next()
			if tok.Type != TokenRelDate {
				t.Errorf("input=%q: got %v, want TokenRelDate", tc.input, tok.Type)
			}
			if tok.Value != tc.input {
				t.Errorf("input=%q: got value %q, want %q", tc.input, tok.Value, tc.input)
			}
		})
	}
}

// TestLexerFunctions tests built-in function tokens.
func TestLexerFunctions(t *testing.T) {
	tests := []string{
		"NOW()",
		"START_OF_DAY()",
		"START_OF_WEEK()",
		"START_OF_MONTH()",
		"START_OF_YEAR()",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			l := NewLexer(input)
			tok := l.Next()
			if tok.Type != TokenFunc {
				t.Errorf("input=%q: got %v, want TokenFunc", input, tok.Type)
			}
			if tok.Value != input {
				t.Errorf("input=%q: got value %q, want %q", input, tok.Value, input)
			}
		})
	}
}

// TestLexerFunctionsLowercase tests function tokens are case-insensitive.
func TestLexerFunctionsLowercase(t *testing.T) {
	l := NewLexer("now()")
	tok := l.Next()
	if tok.Type != TokenFunc {
		t.Errorf("expected TokenFunc for 'now()', got %v", tok.Type)
	}
}

// TestLexerPositionTracking tests that Pos and Length fields are correct.
func TestLexerPositionTracking(t *testing.T) {
	input := `name = "Alice"`
	tokens := tokenizeAll(t, input)

	// tokens: Identifier("name"), Eq("="), String("Alice"), EOF
	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}

	// "name" starts at 0, length 4
	if tokens[0].Pos != 0 {
		t.Errorf("token[0].Pos = %d, want 0", tokens[0].Pos)
	}
	if tokens[0].Length != 4 {
		t.Errorf("token[0].Length = %d, want 4", tokens[0].Length)
	}

	// "=" starts at 5, length 1
	if tokens[1].Pos != 5 {
		t.Errorf("token[1].Pos = %d, want 5", tokens[1].Pos)
	}
	if tokens[1].Length != 1 {
		t.Errorf("token[1].Length = %d, want 1", tokens[1].Length)
	}

	// "Alice" (the string token) starts at 7, length 7 (including quotes)
	if tokens[2].Pos != 7 {
		t.Errorf("token[2].Pos = %d, want 7", tokens[2].Pos)
	}
	if tokens[2].Length != 7 {
		t.Errorf("token[2].Length = %d, want 7 (for %q)", tokens[2].Length, `"Alice"`)
	}
}

// TestLexerPositionTrackingOperators tests positions for multi-char operators.
func TestLexerPositionTrackingOperators(t *testing.T) {
	input := "age >= 18"
	tokens := tokenizeAll(t, input)

	// tokens: Identifier("age"), Gte(">="), Number("18"), EOF
	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}

	// ">=" starts at 4, length 2
	if tokens[1].Pos != 4 {
		t.Errorf(">= token Pos = %d, want 4", tokens[1].Pos)
	}
	if tokens[1].Length != 2 {
		t.Errorf(">= token Length = %d, want 2", tokens[1].Length)
	}

	// "18" starts at 7, length 2
	if tokens[2].Pos != 7 {
		t.Errorf("18 token Pos = %d, want 7", tokens[2].Pos)
	}
	if tokens[2].Length != 2 {
		t.Errorf("18 token Length = %d, want 2", tokens[2].Length)
	}
}

// TestLexerInList tests tokenizing an IN expression with a list.
func TestLexerInList(t *testing.T) {
	input := `status IN ("active", "pending")`
	tokens := tokenizeAll(t, input)

	// Expected: Identifier, IN, LParen, String, Comma, String, RParen, EOF
	wantTypes := []TokenType{
		TokenIdentifier, TokenIn, TokenLParen,
		TokenString, TokenComma, TokenString,
		TokenRParen, TokenEOF,
	}
	if len(tokens) != len(wantTypes) {
		t.Fatalf("got %d tokens, want %d\ntokens: %v", len(tokens), len(wantTypes), tokens)
	}
	for i, want := range wantTypes {
		if tokens[i].Type != want {
			t.Errorf("token[%d]: got %v, want %v (value=%q)", i, tokens[i].Type, want, tokens[i].Value)
		}
	}

	// Verify string values
	if tokens[3].Value != "active" {
		t.Errorf("tokens[3].Value = %q, want %q", tokens[3].Value, "active")
	}
	if tokens[5].Value != "pending" {
		t.Errorf("tokens[5].Value = %q, want %q", tokens[5].Value, "pending")
	}
}

// TestLexerIllegalToken tests that unrecognized characters become TokenIllegal.
func TestLexerIllegalToken(t *testing.T) {
	tests := []string{"@", "#", "$", "^", "&", "*", "|"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			l := NewLexer(input)
			tok := l.Next()
			if tok.Type != TokenIllegal {
				t.Errorf("input=%q: got %v, want TokenIllegal", input, tok.Type)
			}
		})
	}
}

// TestLexerWhitespaceSkipping tests that whitespace is skipped between tokens.
func TestLexerWhitespaceSkipping(t *testing.T) {
	input := "  name   =   42  "
	tokens := tokenize(t, input)
	wantTypes := []TokenType{TokenIdentifier, TokenEq, TokenNumber, TokenEOF}
	if len(tokens) != len(wantTypes) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(wantTypes))
	}
	for i, want := range wantTypes {
		if tokens[i].Type != want {
			t.Errorf("token[%d]: got %v, want %v", i, tokens[i].Type, want)
		}
	}
}

// TestLexerPeek tests that Peek returns the next token without consuming it.
func TestLexerPeek(t *testing.T) {
	l := NewLexer("AND OR")

	peek1 := l.Peek()
	if peek1.Type != TokenAnd {
		t.Errorf("Peek() = %v, want TokenAnd", peek1.Type)
	}

	// Second peek should return the same token
	peek2 := l.Peek()
	if peek2.Type != TokenAnd {
		t.Errorf("second Peek() = %v, want TokenAnd (should not advance)", peek2.Type)
	}

	// Next() should return the peeked token
	next1 := l.Next()
	if next1.Type != TokenAnd {
		t.Errorf("Next() after Peek() = %v, want TokenAnd", next1.Type)
	}

	// Now should see OR
	next2 := l.Next()
	if next2.Type != TokenOr {
		t.Errorf("second Next() = %v, want TokenOr", next2.Type)
	}
}

// TestLexerPosition tests the Position() method.
func TestLexerPosition(t *testing.T) {
	l := NewLexer("ab cd")

	if l.Position() != 0 {
		t.Errorf("initial Position() = %d, want 0", l.Position())
	}

	l.Next() // consume "ab"
	// after consuming "ab" (2 chars), position should be at the space or next token start
	pos := l.Position()
	if pos < 2 {
		t.Errorf("Position() after first token = %d, want >= 2", pos)
	}
}

// TestLexerComplexExpression tests a complex real-world MRQL expression.
func TestLexerComplexExpression(t *testing.T) {
	input := `name = "Alice" AND age >= 18 OR fileSize < 10mb`
	l := NewLexer(input)

	wantTypes := []TokenType{
		TokenIdentifier, // name
		TokenEq,         // =
		TokenString,     // "Alice"
		TokenAnd,        // AND
		TokenIdentifier, // age
		TokenGte,        // >=
		TokenNumber,     // 18
		TokenOr,         // OR
		TokenIdentifier, // fileSize
		TokenLt,         // <
		TokenNumber,     // 10mb
		TokenEOF,
	}

	for i, wantType := range wantTypes {
		tok := l.Next()
		if tok.Type != wantType {
			t.Errorf("token[%d]: got %v (%q), want %v", i, tok.Type, tok.Value, wantType)
		}
	}
}

// TestLexerEOF tests repeated EOF tokens.
func TestLexerEOF(t *testing.T) {
	l := NewLexer("")
	tok1 := l.Next()
	if tok1.Type != TokenEOF {
		t.Errorf("expected TokenEOF on empty input, got %v", tok1.Type)
	}
	// Calling Next again on empty lexer should still return EOF
	tok2 := l.Next()
	if tok2.Type != TokenEOF {
		t.Errorf("expected repeated TokenEOF, got %v", tok2.Type)
	}
}

// TestLexerNegativeNumberVsRelDate tests disambiguation between -7d (RelDate) and negative numbers.
func TestLexerNegativeNumberVsRelDate(t *testing.T) {
	// -7d should be a relative date
	l1 := NewLexer("-7d")
	tok := l1.Next()
	if tok.Type != TokenRelDate {
		t.Errorf("-7d: got %v, want TokenRelDate", tok.Type)
	}
	if tok.Value != "-7d" {
		t.Errorf("-7d: value = %q, want \"-7d\"", tok.Value)
	}

	// -3m should be a relative date (month)
	l2 := NewLexer("-3m")
	tok = l2.Next()
	if tok.Type != TokenRelDate {
		t.Errorf("-3m: got %v, want TokenRelDate", tok.Type)
	}

	// -1y should be a relative date (year)
	l3 := NewLexer("-1y")
	tok = l3.Next()
	if tok.Type != TokenRelDate {
		t.Errorf("-1y: got %v, want TokenRelDate", tok.Type)
	}
}

// TestLexerStringPositionLength tests string token position includes quotes.
func TestLexerStringPositionLength(t *testing.T) {
	input := `"hello"`
	l := NewLexer(input)
	tok := l.Next()
	if tok.Type != TokenString {
		t.Fatalf("got %v, want TokenString", tok.Type)
	}
	if tok.Pos != 0 {
		t.Errorf("Pos = %d, want 0", tok.Pos)
	}
	// length should cover the entire quoted string including quotes: 7 bytes
	if tok.Length != 7 {
		t.Errorf("Length = %d, want 7", tok.Length)
	}
	// but Value should NOT include the quotes
	if tok.Value != "hello" {
		t.Errorf("Value = %q, want %q", tok.Value, "hello")
	}
}

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
	l := NewLexer("count = 5")
	tok := l.Next()
	if tok.Type != TokenIdentifier {
		t.Errorf("expected TokenIdentifier for bare 'count', got %v", tok.Type)
	}
}

func TestLexer_GroupWithoutByIsIdentifier(t *testing.T) {
	l := NewLexer("group = \"Photos\"")
	tok := l.Next()
	if tok.Type != TokenIdentifier {
		t.Errorf("expected TokenIdentifier for bare 'group', got %v", tok.Type)
	}
}

// TestLexerDotSeparated tests dot-separated identifiers (e.g. relation paths).
func TestLexerDotSeparated(t *testing.T) {
	input := "group.name"
	tokens := tokenizeAll(t, input)

	// Expected: Identifier("group"), Dot("."), Identifier("name"), EOF
	wantTypes := []TokenType{TokenIdentifier, TokenDot, TokenIdentifier, TokenEOF}
	if len(tokens) != len(wantTypes) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(wantTypes), tokens)
	}
	for i, want := range wantTypes {
		if tokens[i].Type != want {
			t.Errorf("token[%d]: got %v (%q), want %v", i, tokens[i].Type, tokens[i].Value, want)
		}
	}
}
