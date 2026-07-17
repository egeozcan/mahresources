package mrql

const (
	// MaxQueryBytes bounds parser/completer memory independently of token count.
	MaxQueryBytes = 32 * 1024
	// MaxQueryTokens bounds AST size and recursive validation/translation walks.
	MaxQueryTokens = 2048
	// MaxExpressionDepth bounds user-authored NOT/parenthesis recursion.
	MaxExpressionDepth = 64
	// MaxINListValues keeps generated bind lists portable and bounded.
	MaxINListValues = 500
)

func querySizeError() *ParseError {
	return &ParseError{Message: "query exceeds maximum size of 32768 bytes", Pos: MaxQueryBytes, Length: 0}
}

func tokenLimitError(tok Token) *ParseError {
	return &ParseError{Message: "query exceeds maximum token count of 2048", Pos: tok.Pos, Length: tok.Length}
}

func depthLimitError(tok Token) *ParseError {
	return &ParseError{Message: "expression nesting exceeds maximum depth of 64", Pos: tok.Pos, Length: tok.Length}
}

func inListLimitError(tok Token) *ParseError {
	return &ParseError{Message: "IN list exceeds maximum of 500 values", Pos: tok.Pos, Length: tok.Length}
}
