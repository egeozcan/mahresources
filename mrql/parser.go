package mrql

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseError is returned for all parse errors. It includes the error message,
// the byte position in the source where the error occurred, and the length of
// the offending token (may be 0 if no token is available).
type ParseError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at position %d: %s", e.Pos, e.Message)
}

// parser is the internal recursive-descent parser state.
type parser struct {
	lexer *Lexer
}

// Parse parses the given input string as an MRQL query and returns the AST.
// Returns *ParseError for parse errors.
func Parse(input string) (*Query, error) {
	p := &parser{lexer: NewLexer(input)}
	return p.parseQuery()
}

// parseQuery = [expression] [orderBy] [limit] [offset]
func (p *parser) parseQuery() (*Query, error) {
	q := &Query{
		Limit:  -1,
		Offset: -1,
	}

	// Parse optional WHERE expression — but only if the next token looks like
	// the start of an expression. ORDER BY, LIMIT, OFFSET signal no WHERE clause.
	tok := p.lexer.Peek()
	if tok.Type != TokenEOF && tok.Type != TokenOrderBy && tok.Type != TokenLimit && tok.Type != TokenOffset {
		var err error
		q.Where, err = p.parseExpression()
		if err != nil {
			return nil, err
		}
	}

	// Optional ORDER BY
	if p.lexer.Peek().Type == TokenOrderBy {
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		q.OrderBy = orderBy
	}

	// Optional LIMIT
	if p.lexer.Peek().Type == TokenLimit {
		p.lexer.Next() // consume LIMIT
		numTok := p.lexer.Next()
		if numTok.Type != TokenNumber {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected number after LIMIT, got %q", numTok.Value),
				Pos:     numTok.Pos,
				Length:  numTok.Length,
			}
		}
		n, err := parseIntFromToken(numTok)
		if err != nil {
			return nil, &ParseError{Message: err.Error(), Pos: numTok.Pos, Length: numTok.Length}
		}
		q.Limit = n
	}

	// Optional OFFSET
	if p.lexer.Peek().Type == TokenOffset {
		p.lexer.Next() // consume OFFSET
		numTok := p.lexer.Next()
		if numTok.Type != TokenNumber {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected number after OFFSET, got %q", numTok.Value),
				Pos:     numTok.Pos,
				Length:  numTok.Length,
			}
		}
		n, err := parseIntFromToken(numTok)
		if err != nil {
			return nil, &ParseError{Message: err.Error(), Pos: numTok.Pos, Length: numTok.Length}
		}
		q.Offset = n
	}

	// Should be at EOF now
	final := p.lexer.Peek()
	if final.Type != TokenEOF {
		return nil, &ParseError{
			Message: fmt.Sprintf("unexpected token %q at end of query", final.Value),
			Pos:     final.Pos,
			Length:  final.Length,
		}
	}

	return q, nil
}

// parseExpression = orExpr
func (p *parser) parseExpression() (Node, error) {
	return p.parseOrExpr()
}

// orExpr = andExpr ("OR" andExpr)*
func (p *parser) parseOrExpr() (Node, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for p.lexer.Peek().Type == TokenOr {
		opTok := p.lexer.Next() // consume OR
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: opTok, Right: right}
	}

	return left, nil
}

// andExpr = notExpr ("AND" notExpr)*
func (p *parser) parseAndExpr() (Node, error) {
	left, err := p.parseNotExpr()
	if err != nil {
		return nil, err
	}

	for p.lexer.Peek().Type == TokenAnd {
		opTok := p.lexer.Next() // consume AND
		right, err := p.parseNotExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: opTok, Right: right}
	}

	return left, nil
}

// notExpr = "NOT" notExpr | primary
func (p *parser) parseNotExpr() (Node, error) {
	if p.lexer.Peek().Type == TokenNot {
		notTok := p.lexer.Next() // consume NOT
		expr, err := p.parseNotExpr()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Token: notTok, Expr: expr}, nil
	}
	return p.parsePrimary()
}

// primary = "(" expression ")" | textSearch | fieldExpr
func (p *parser) parsePrimary() (Node, error) {
	tok := p.lexer.Peek()

	switch tok.Type {
	case TokenLParen:
		p.lexer.Next() // consume '('
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected ')' to close '(', got %q", rp.Value),
				Pos:     rp.Pos,
				Length:  rp.Length,
			}
		}
		return expr, nil

	case TokenText:
		return p.parseTextSearch()

	case TokenIdentifier, TokenKwType:
		return p.parseFieldExpr()

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("unexpected token %q, expected field name or '('", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}
}

// textSearch = "TEXT" "~" STRING
func (p *parser) parseTextSearch() (Node, error) {
	textTok := p.lexer.Next() // consume TEXT

	// Expect ~
	tilTok := p.lexer.Next()
	if tilTok.Type != TokenLike {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected '~' after TEXT, got %q", tilTok.Value),
			Pos:     tilTok.Pos,
			Length:  tilTok.Length,
		}
	}

	// Expect string
	strTok := p.lexer.Next()
	if strTok.Type != TokenString {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected string literal after TEXT ~, got %q", strTok.Value),
			Pos:     strTok.Pos,
			Length:  strTok.Length,
		}
	}

	return &TextSearchExpr{
		TextToken: textTok,
		Value:     &StringLiteral{Token: strTok, Value: strTok.Value},
	}, nil
}

// fieldExpr = field (comparison | inExpr | isExpr)
// field     = IDENT ("." IDENT)?   // max 2 parts
func (p *parser) parseFieldExpr() (Node, error) {
	field, err := p.parseField()
	if err != nil {
		return nil, err
	}

	next := p.lexer.Peek()

	switch next.Type {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte, TokenLike, TokenNotLike:
		return p.parseComparison(field)

	case TokenIn:
		return p.parseInExpr(field, false)

	case TokenNot:
		// field NOT IN (...)
		notTok := p.lexer.Next() // consume NOT
		inTok := p.lexer.Peek()
		if inTok.Type != TokenIn {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected IN after field NOT, got %q", inTok.Value),
				Pos:     notTok.Pos,
				Length:  notTok.Length,
			}
		}
		return p.parseInExpr(field, true)

	case TokenIs:
		return p.parseIsExpr(field)

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected comparison operator, IN, or IS after field %q, got %q", field.Name(), next.Value),
			Pos:     next.Pos,
			Length:  next.Length,
		}
	}
}

// parseField reads a field name: IDENT or KWTYPE, optionally followed by "." IDENT
func (p *parser) parseField() (*FieldExpr, error) {
	tok := p.lexer.Next()
	if tok.Type != TokenIdentifier && tok.Type != TokenKwType {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected field name (identifier), got %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	parts := []Token{tok}

	// Check for "." IDENT
	if p.lexer.Peek().Type == TokenDot {
		p.lexer.Next() // consume '.'

		nextTok := p.lexer.Next()
		if nextTok.Type != TokenIdentifier && nextTok.Type != TokenKwType {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected identifier after '.', got %q", nextTok.Value),
				Pos:     nextTok.Pos,
				Length:  nextTok.Length,
			}
		}
		parts = append(parts, nextTok)

		// Reject three-level traversal: a.b.c
		if p.lexer.Peek().Type == TokenDot {
			dotTok := p.lexer.Peek()
			return nil, &ParseError{
				Message: fmt.Sprintf("field %q.%q: only one level of dotted access is allowed (max 2 parts), use format 'parent.field'", tok.Value, nextTok.Value),
				Pos:     dotTok.Pos,
				Length:  dotTok.Length,
			}
		}
	}

	return &FieldExpr{Parts: parts}, nil
}

// parseComparison = op value
func (p *parser) parseComparison(field *FieldExpr) (Node, error) {
	opTok := p.lexer.Next() // consume operator

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return &ComparisonExpr{
		Field:    field,
		Operator: opTok,
		Value:    val,
	}, nil
}

// parseInExpr = ["NOT"] "IN" "(" value ("," value)* ")"
func (p *parser) parseInExpr(field *FieldExpr, negated bool) (Node, error) {
	inTok := p.lexer.Next() // consume IN

	lp := p.lexer.Next()
	if lp.Type != TokenLParen {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected '(' after IN, got %q", lp.Value),
			Pos:     lp.Pos,
			Length:  lp.Length,
		}
	}

	var values []Node

	// Must have at least one value
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	values = append(values, val)

	for p.lexer.Peek().Type == TokenComma {
		p.lexer.Next() // consume ','
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, val)
	}

	rp := p.lexer.Next()
	if rp.Type != TokenRParen {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected ')' to close IN list, got %q", rp.Value),
			Pos:     rp.Pos,
			Length:  rp.Length,
		}
	}

	return &InExpr{
		Field:   field,
		Negated: negated,
		Values:  values,
		InToken: inTok,
	}, nil
}

// parseIsExpr = "IS" ["NOT"] ("EMPTY" | "NULL")
func (p *parser) parseIsExpr(field *FieldExpr) (Node, error) {
	isTok := p.lexer.Next() // consume IS

	negated := false
	if p.lexer.Peek().Type == TokenNot {
		p.lexer.Next() // consume NOT
		negated = true
	}

	kindTok := p.lexer.Next()
	isNull := false
	switch kindTok.Type {
	case TokenEmpty:
		isNull = false
	case TokenNull:
		isNull = true
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected EMPTY or NULL after IS [NOT], got %q", kindTok.Value),
			Pos:     kindTok.Pos,
			Length:  kindTok.Length,
		}
	}

	return &IsExpr{
		Field:   field,
		Negated: negated,
		IsNull:  isNull,
		IsToken: isTok,
	}, nil
}

// parseValue = STRING | NUMBER | REL_DATE | FUNC | IDENT (bare identifier as string)
func (p *parser) parseValue() (Node, error) {
	tok := p.lexer.Peek()

	switch tok.Type {
	case TokenString:
		p.lexer.Next()
		return &StringLiteral{Token: tok, Value: tok.Value}, nil

	case TokenNumber:
		p.lexer.Next()
		return parseNumberLiteral(tok)

	case TokenRelDate:
		p.lexer.Next()
		return parseRelDateLiteral(tok)

	case TokenFunc:
		p.lexer.Next()
		return &FuncCall{Token: tok, Name: tok.Value}, nil

	case TokenIdentifier:
		// Bare identifier: treat as string value
		p.lexer.Next()
		return &StringLiteral{Token: tok, Value: tok.Value}, nil

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected value (string, number, date, function, or identifier), got %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}
}

// parseOrderBy = "ORDER BY" field ("ASC"|"DESC")? ("," field ("ASC"|"DESC")?)*
func (p *parser) parseOrderBy() ([]OrderByClause, error) {
	p.lexer.Next() // consume ORDER BY

	var clauses []OrderByClause

	for {
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}

		ascending := true // default is ASC
		switch p.lexer.Peek().Type {
		case TokenAsc:
			p.lexer.Next()
			ascending = true
		case TokenDesc:
			p.lexer.Next()
			ascending = false
		}

		clauses = append(clauses, OrderByClause{Field: field, Ascending: ascending})

		if p.lexer.Peek().Type != TokenComma {
			break
		}
		p.lexer.Next() // consume ','
	}

	return clauses, nil
}

// parseNumberLiteral parses a TokenNumber into a NumberLiteral node.
// It extracts the numeric value, optional unit, and computes the raw byte value.
func parseNumberLiteral(tok Token) (*NumberLiteral, error) {
	raw := tok.Value
	unit := ""

	// Check for unit suffix (case-insensitive)
	lower := strings.ToLower(raw)
	if strings.HasSuffix(lower, "kb") {
		unit = "kb"
		raw = raw[:len(raw)-2]
	} else if strings.HasSuffix(lower, "mb") {
		unit = "mb"
		raw = raw[:len(raw)-2]
	} else if strings.HasSuffix(lower, "gb") {
		unit = "gb"
		raw = raw[:len(raw)-2]
	}

	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid number %q: %v", tok.Value, err),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	var rawBytes int64
	switch unit {
	case "kb":
		rawBytes = int64(math.Round(val * 1024))
	case "mb":
		rawBytes = int64(math.Round(val * 1024 * 1024))
	case "gb":
		rawBytes = int64(math.Round(val * 1024 * 1024 * 1024))
	default:
		rawBytes = int64(math.Round(val))
	}

	return &NumberLiteral{
		Token: tok,
		Value: val,
		Unit:  unit,
		Raw:   rawBytes,
	}, nil
}

// parseRelDateLiteral parses a TokenRelDate like "-7d" into a RelDateLiteral.
func parseRelDateLiteral(tok Token) (*RelDateLiteral, error) {
	s := tok.Value // e.g. "-7d"
	if len(s) < 3 || s[0] != '-' {
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid relative date %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	// last char is the unit
	unitChar := strings.ToLower(string(s[len(s)-1]))
	numStr := s[1 : len(s)-1] // strip '-' and unit

	amount, err := strconv.Atoi(numStr)
	if err != nil {
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid relative date amount in %q: %v", tok.Value, err),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	return &RelDateLiteral{
		Token:  tok,
		Amount: amount,
		Unit:   unitChar,
	}, nil
}

// parseIntFromToken parses a TokenNumber as an integer (rejects floats and units).
func parseIntFromToken(tok Token) (int, error) {
	// For LIMIT/OFFSET we only want plain integers; strip units if lexer added them
	raw := tok.Value
	lower := strings.ToLower(raw)
	if strings.HasSuffix(lower, "kb") || strings.HasSuffix(lower, "mb") || strings.HasSuffix(lower, "gb") {
		return 0, fmt.Errorf("LIMIT/OFFSET requires a plain integer, got %q", tok.Value)
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %v", tok.Value, err)
	}
	return int(val), nil
}
