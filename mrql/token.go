package mrql

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Literals
	TokenString     TokenType = iota // "quoted string"
	TokenNumber                      // 42, 10mb, 3.14
	TokenIdentifier                  // field names: name, contentType, etc.

	// Keywords
	TokenAnd     // AND
	TokenOr      // OR
	TokenNot     // NOT
	TokenIn      // IN
	TokenIs      // IS
	TokenEmpty   // EMPTY
	TokenNull    // NULL
	TokenOrderBy // ORDER BY (two words, merged by lexer)
	TokenAsc     // ASC
	TokenDesc    // DESC
	TokenLimit   // LIMIT
	TokenOffset  // OFFSET
	TokenGroupBy // GROUP BY (two words, merged by lexer)
	TokenCount   // COUNT (followed by '(')
	TokenSum     // SUM (followed by '(')
	TokenAvg     // AVG (followed by '(')
	TokenMin     // MIN (followed by '(')
	TokenMax     // MAX (followed by '(')
	TokenText    // TEXT (for TEXT ~)
	TokenKwType  // TYPE (also usable as field name via context)

	// Operators
	TokenEq      // =
	TokenNeq     // !=
	TokenGt      // >
	TokenGte     // >=
	TokenLt      // <
	TokenLte     // <=
	TokenLike    // ~
	TokenNotLike // !~

	// Delimiters
	TokenLParen // (
	TokenRParen // )
	TokenComma  // ,
	TokenDot    // .

	// Special
	TokenRelDate // -7d, -30d, -3m, -1y
	TokenFunc    // NOW(), START_OF_DAY(), etc.

	TokenEOF
	TokenIllegal
)

// Token represents a single lexical token with its position in the source.
type Token struct {
	Type   TokenType
	Value  string
	Pos    int // byte offset in the source string
	Length int // length in bytes
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%v, %q, pos=%d)", t.Type, t.Value, t.Pos)
}
