package mrql

import (
	"strings"
	"unicode"
)

// Lexer tokenizes an MRQL query string.
type Lexer struct {
	input   string
	pos     int  // current byte position
	peeked  bool // whether we have a peeked token
	peekTok Token

	maxTokens  int
	tokenCount int
	limitErr   *ParseError
}

// NewLexer creates a new Lexer for the given input string.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input}
}

func newBoundedLexer(input string) *Lexer {
	return &Lexer{input: input, maxTokens: MaxQueryTokens}
}

// Position returns the current byte offset in the source string.
func (l *Lexer) Position() int {
	return l.pos
}

// Peek returns the next token without consuming it.
// Repeated calls to Peek return the same token.
func (l *Lexer) Peek() Token {
	if !l.peeked {
		l.peekTok = l.next()
		l.peeked = true
	}
	return l.peekTok
}

// Next returns the next token and advances the position.
func (l *Lexer) Next() Token {
	if l.peeked {
		l.peeked = false
		return l.peekTok
	}
	return l.next()
}

// next is the budget-aware token scanner.
func (l *Lexer) next() Token {
	tok := l.scan()
	if tok.Type == TokenEOF || l.maxTokens <= 0 {
		return tok
	}
	l.tokenCount++
	if l.tokenCount > l.maxTokens {
		if l.limitErr == nil {
			l.limitErr = tokenLimitError(tok)
		}
		return Token{Type: TokenIllegal, Value: tok.Value, Pos: tok.Pos, Length: tok.Length}
	}
	return tok
}

// scan is the internal unbounded token scanner.
func (l *Lexer) scan() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Pos: l.pos, Length: 0}
	}

	start := l.pos
	ch := l.input[l.pos]

	// String literals
	if ch == '"' {
		return l.readString(start)
	}

	// Numbers (and unit-suffixed numbers)
	if isDigit(ch) {
		return l.readNumber(start)
	}

	// Relative dates: -Nd, -Nw, -Nm, -Ny
	if ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
		return l.readRelDate(start)
	}

	// Parameter placeholders: $name where name is [a-zA-Z_][a-zA-Z0-9_]*.
	// `$name` inside a quoted string stays literal text (strings are read above).
	if ch == '$' {
		return l.readParam(start)
	}

	// Identifiers, keywords, and functions
	if isLetter(ch) || ch == '_' {
		return l.readWord(start)
	}

	// Operators and delimiters
	switch ch {
	case '=':
		l.pos++
		return Token{Type: TokenEq, Value: "=", Pos: start, Length: 1}
	case '~':
		// ~* is the PostgreSQL case-insensitive regex operator; longest match first
		// so plain ~ (contains) stays unaffected. A space (`~ *`) is not a match.
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '*' {
			l.pos += 2
			return Token{Type: TokenRegex, Value: "~*", Pos: start, Length: 2}
		}
		l.pos++
		return Token{Type: TokenLike, Value: "~", Pos: start, Length: 1}
	case '>':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return Token{Type: TokenGte, Value: ">=", Pos: start, Length: 2}
		}
		l.pos++
		return Token{Type: TokenGt, Value: ">", Pos: start, Length: 1}
	case '<':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return Token{Type: TokenLte, Value: "<=", Pos: start, Length: 2}
		}
		l.pos++
		return Token{Type: TokenLt, Value: "<", Pos: start, Length: 1}
	case '!':
		if l.pos+1 < len(l.input) {
			if l.input[l.pos+1] == '=' {
				l.pos += 2
				return Token{Type: TokenNeq, Value: "!=", Pos: start, Length: 2}
			}
			if l.input[l.pos+1] == '~' {
				// !~* is the negated PostgreSQL case-insensitive regex operator.
				if l.pos+2 < len(l.input) && l.input[l.pos+2] == '*' {
					l.pos += 3
					return Token{Type: TokenNotRegex, Value: "!~*", Pos: start, Length: 3}
				}
				l.pos += 2
				return Token{Type: TokenNotLike, Value: "!~", Pos: start, Length: 2}
			}
		}
		// Illegal lone '!'
		l.pos++
		return Token{Type: TokenIllegal, Value: "!", Pos: start, Length: 1}
	case '(':
		l.pos++
		return Token{Type: TokenLParen, Value: "(", Pos: start, Length: 1}
	case ')':
		l.pos++
		return Token{Type: TokenRParen, Value: ")", Pos: start, Length: 1}
	case ',':
		l.pos++
		return Token{Type: TokenComma, Value: ",", Pos: start, Length: 1}
	case '.':
		l.pos++
		return Token{Type: TokenDot, Value: ".", Pos: start, Length: 1}
	}

	// Anything else is illegal
	l.pos++
	return Token{Type: TokenIllegal, Value: string(ch), Pos: start, Length: 1}
}

// skipWhitespace advances past spaces, tabs, newlines.
func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.pos++
	}
}

// readString reads a double-quoted string with escape handling.
// The token Value contains the unescaped content (without quotes).
// The token Length covers the full quoted literal in the source.
func (l *Lexer) readString(start int) Token {
	l.pos++ // skip opening '"'
	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			switch next {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				// Keep unrecognized escapes as-is
				sb.WriteByte('\\')
				sb.WriteByte(next)
			}
			l.pos += 2
			continue
		}
		if ch == '"' {
			l.pos++ // consume closing '"'
			return Token{
				Type:   TokenString,
				Value:  sb.String(),
				Pos:    start,
				Length: l.pos - start,
			}
		}
		sb.WriteByte(ch)
		l.pos++
	}
	// Unterminated string
	return Token{Type: TokenIllegal, Value: sb.String(), Pos: start, Length: l.pos - start}
}

// readNumber reads an integer or float, optionally followed by a unit suffix (kb, mb, gb).
func (l *Lexer) readNumber(start int) Token {
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	// Optional decimal part
	if l.pos < len(l.input) && l.input[l.pos] == '.' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
		l.pos++ // consume '.'
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}
	// Optional unit suffix: kb, mb, gb (case-insensitive, exactly 2 letters)
	if l.pos+1 < len(l.input) {
		suffix := strings.ToLower(l.input[l.pos : l.pos+2])
		if suffix == "kb" || suffix == "mb" || suffix == "gb" {
			l.pos += 2
		}
	}
	val := l.input[start:l.pos]
	return Token{Type: TokenNumber, Value: val, Pos: start, Length: l.pos - start}
}

// readRelDate reads a relative date token like -7d, -30d, -3m, -1y, -2w.
func (l *Lexer) readRelDate(start int) Token {
	l.pos++ // consume '-'
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	// consume the unit letter: d, w, m, y
	if l.pos < len(l.input) {
		unit := l.input[l.pos]
		unitLower := unicode.ToLower(rune(unit))
		if unitLower == 'd' || unitLower == 'w' || unitLower == 'm' || unitLower == 'y' {
			l.pos++
			val := l.input[start:l.pos]
			return Token{Type: TokenRelDate, Value: val, Pos: start, Length: l.pos - start}
		}
	}
	// Not a valid rel date — emit what we have as illegal
	val := l.input[start:l.pos]
	return Token{Type: TokenIllegal, Value: val, Pos: start, Length: l.pos - start}
}

// readParam reads a parameter placeholder: '$' followed by an identifier
// ([a-zA-Z_][a-zA-Z0-9_]*). The token Value holds the name without the leading
// '$'; Length covers the full "$name" span. A lone '$' or '$' followed by a
// non-identifier character is a lexer error (TokenIllegal).
func (l *Lexer) readParam(start int) Token {
	l.pos++ // consume '$'
	nameStart := l.pos
	if l.pos >= len(l.input) || !(isLetter(l.input[l.pos]) || l.input[l.pos] == '_') {
		// '$' not followed by a valid identifier start.
		return Token{Type: TokenIllegal, Value: l.input[start:l.pos], Pos: start, Length: l.pos - start}
	}
	for l.pos < len(l.input) && isWordChar(l.input[l.pos]) {
		l.pos++
	}
	return Token{Type: TokenParam, Value: l.input[nameStart:l.pos], Pos: start, Length: l.pos - start}
}

// readWord reads an identifier, keyword, or function call.
func (l *Lexer) readWord(start int) Token {
	for l.pos < len(l.input) && isWordChar(l.input[l.pos]) {
		l.pos++
	}
	word := l.input[start:l.pos]
	upper := strings.ToUpper(word)

	// Check for ORDER BY (two-word keyword): word is "ORDER" followed by whitespace then "BY"
	if upper == "ORDER" {
		savedPos := l.pos
		// skip whitespace
		tmp := l.pos
		for tmp < len(l.input) && unicode.IsSpace(rune(l.input[tmp])) {
			tmp++
		}
		// check "BY"
		if tmp+2 <= len(l.input) && strings.ToUpper(l.input[tmp:tmp+2]) == "BY" {
			// make sure "BY" is not followed by a word character
			endBy := tmp + 2
			if endBy >= len(l.input) || !isWordChar(l.input[endBy]) {
				l.pos = endBy
				return Token{
					Type:   TokenOrderBy,
					Value:  "ORDER BY",
					Pos:    start,
					Length: l.pos - start,
				}
			}
		}
		// Not ORDER BY — restore and fall through
		l.pos = savedPos
	}

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
				return Token{Type: TokenGroupBy, Value: "GROUP BY", Pos: start, Length: l.pos - start}
			}
		}
		l.pos = savedPos
	}

	// Check for SIMILAR TO (two-word keyword): word is "SIMILAR" followed by
	// whitespace then "TO". "SIMILAR" alone stays a plain identifier so fields
	// or meta keys named similar keep working.
	if upper == "SIMILAR" {
		savedPos := l.pos
		tmp := l.pos
		for tmp < len(l.input) && unicode.IsSpace(rune(l.input[tmp])) {
			tmp++
		}
		if tmp+2 <= len(l.input) && strings.ToUpper(l.input[tmp:tmp+2]) == "TO" {
			endTo := tmp + 2
			if endTo >= len(l.input) || !isWordChar(l.input[endTo]) {
				l.pos = endTo
				return Token{Type: TokenSimilarTo, Value: "SIMILAR TO", Pos: start, Length: l.pos - start}
			}
		}
		l.pos = savedPos
	}

	// Aggregate function keyword: word followed by "(" — emit keyword, don't consume "("
	if l.pos < len(l.input) && l.input[l.pos] == '(' {
		if aggType, ok := aggregateKeywords[upper]; ok {
			return Token{Type: aggType, Value: word, Pos: start, Length: l.pos - start}
		}
	}

	// Check if it's a function call: word followed by "()"
	if l.pos+1 < len(l.input) && l.input[l.pos] == '(' && l.input[l.pos+1] == ')' {
		funcName := upper
		if isFunctionName(funcName) {
			l.pos += 2 // consume "()"
			val := l.input[start:l.pos]
			return Token{Type: TokenFunc, Value: val, Pos: start, Length: l.pos - start}
		}
	}

	// Keyword lookup
	if tt, ok := keywordMap[upper]; ok {
		return Token{Type: tt, Value: word, Pos: start, Length: l.pos - start}
	}

	// Plain identifier
	return Token{Type: TokenIdentifier, Value: word, Pos: start, Length: l.pos - start}
}

// keywordMap maps uppercase keyword strings to token types.
var keywordMap = map[string]TokenType{
	"AND":    TokenAnd,
	"OR":     TokenOr,
	"NOT":    TokenNot,
	"IN":     TokenIn,
	"IS":     TokenIs,
	"EMPTY":  TokenEmpty,
	"NULL":   TokenNull,
	"ASC":    TokenAsc,
	"DESC":   TokenDesc,
	"LIMIT":  TokenLimit,
	"OFFSET": TokenOffset,
	"HAVING": TokenHaving,
	"TEXT":   TokenText,
	"TYPE":   TokenKwType,
	"SCOPE":  TokenScope,
}

// aggregateKeywords maps uppercase aggregate function names to token types.
var aggregateKeywords = map[string]TokenType{
	"COUNT": TokenCount,
	"SUM":   TokenSum,
	"AVG":   TokenAvg,
	"MIN":   TokenMin,
	"MAX":   TokenMax,
}

// knownFunctions is the set of recognized built-in function names (uppercase).
var knownFunctions = map[string]bool{
	"NOW":            true,
	"START_OF_DAY":   true,
	"START_OF_WEEK":  true,
	"START_OF_MONTH": true,
	"START_OF_YEAR":  true,
}

func isFunctionName(upper string) bool {
	return knownFunctions[upper]
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// isWordChar returns true for characters that can appear inside an identifier.
// Allows letters, digits, and underscores.
func isWordChar(ch byte) bool {
	return isLetter(ch) || isDigit(ch) || ch == '_'
}
