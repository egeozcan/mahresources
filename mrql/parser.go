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
		Limit:       -1,
		Offset:      -1,
		BucketLimit: -1,
	}

	// Parse optional WHERE expression — but only if the next token looks like
	// the start of an expression. ORDER BY, LIMIT, OFFSET signal no WHERE clause.
	tok := p.lexer.Peek()
	if tok.Type != TokenEOF && tok.Type != TokenOrderBy && tok.Type != TokenLimit && tok.Type != TokenOffset && tok.Type != TokenGroupBy && tok.Type != TokenScope {
		var err error
		q.Where, err = p.parseExpression()
		if err != nil {
			return nil, err
		}
	}

	// Optional SCOPE
	if p.lexer.Peek().Type == TokenScope {
		scope, err := p.parseScope()
		if err != nil {
			return nil, err
		}
		q.Scope = scope
	}

	// Optional GROUP BY
	if p.lexer.Peek().Type == TokenGroupBy {
		groupBy, err := p.parseGroupBy()
		if err != nil {
			return nil, err
		}
		q.GroupBy = groupBy
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
		if isAggregateToken(final.Type) {
			return nil, &ParseError{
				Message: fmt.Sprintf("aggregate function %s requires a preceding GROUP BY clause", final.Value),
				Pos:     final.Pos,
				Length:  final.Length,
			}
		}
		if final.Type == TokenHaving {
			return nil, &ParseError{
				Message: "HAVING requires a preceding GROUP BY clause",
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

	return q, nil
}

// ParseFilter parses a bare boolean filter expression (the WHERE-clause grammar
// only) and returns a Query with the given entity type set. It powers the
// list-page filter bar, where sort and pagination belong to the page, the entity
// type is implied by the page, and queries must be self-contained.
//
// Compared to Parse it rejects, each with a position that matches the input 1:1:
//   - clause keywords (ORDER BY, LIMIT, OFFSET, GROUP BY, HAVING, SCOPE);
//   - the `type` pseudo-field (implied by the page);
//   - `$name` parameter placeholders (there are no param inputs on list pages).
//
// Everything else in the expression grammar is allowed, including
// SIMILAR TO resource(N) (whose resource-only requirement is enforced by
// Validate once EntityType is set).
func ParseFilter(entity EntityType, input string) (*Query, error) {
	p := &parser{lexer: NewLexer(input)}

	// An empty expression has nothing to filter on — surface a clear error
	// rather than a bare "unexpected EOF".
	if p.lexer.Peek().Type == TokenEOF {
		return nil, &ParseError{Message: "empty filter expression", Pos: 0, Length: 0}
	}

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Anything left after a complete expression is a clause keyword (ORDER BY,
	// LIMIT, OFFSET, GROUP BY, HAVING, SCOPE) or stray input — none allowed here.
	if tok := p.lexer.Peek(); tok.Type != TokenEOF {
		return nil, filterTrailingTokenError(tok)
	}

	// Reject `type` fields and $name placeholders anywhere in the expression.
	if err := rejectFilterConstructs(expr); err != nil {
		return nil, err
	}

	return &Query{
		Where:       expr,
		Limit:       -1,
		Offset:      -1,
		BucketLimit: -1,
		EntityType:  entity,
	}, nil
}

// filterTrailingTokenError maps a token left over after a filter expression to a
// positioned ParseError. Clause keywords get a targeted message; anything else
// falls back to the generic "unexpected token" phrasing.
func filterTrailingTokenError(tok Token) *ParseError {
	switch tok.Type {
	case TokenOrderBy, TokenLimit, TokenOffset, TokenGroupBy, TokenHaving, TokenScope:
		return &ParseError{
			Message: fmt.Sprintf("%s is not allowed in a filter expression; the list page controls sort and pagination", strings.ToUpper(tok.Value)),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	default:
		return &ParseError{
			Message: fmt.Sprintf("unexpected token %q after filter expression", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}
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

	case TokenSimilarTo:
		return p.parseSimilarTo()

	case TokenIdentifier, TokenKwType, TokenHaving:
		// TokenHaving: the word "having" stays usable as a field name here —
		// the HAVING clause is only recognized inside GROUP BY.
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

// similarTo = "SIMILAR TO" "resource" "(" NUMBER ")" [ "WITHIN" NUMBER ]
// WITHIN is deliberately not a lexer keyword — it is matched here as a plain
// identifier so fields or meta keys named "within" keep working.
func (p *parser) parseSimilarTo() (Node, error) {
	simTok := p.lexer.Next() // consume SIMILAR TO

	kindTok := p.lexer.Next()
	if kindTok.Type != TokenIdentifier || !strings.EqualFold(kindTok.Value, "resource") {
		return nil, &ParseError{
			Message: fmt.Sprintf("SIMILAR TO expects resource(<id>), got %q — only resources have perceptual hashes", kindTok.Value),
			Pos:     kindTok.Pos,
			Length:  kindTok.Length,
		}
	}

	lp := p.lexer.Next()
	if lp.Type != TokenLParen {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected '(' after SIMILAR TO resource, got %q", lp.Value),
			Pos:     lp.Pos,
			Length:  lp.Length,
		}
	}

	idTok := p.lexer.Next()
	targetID, err := similarToInt(idTok)
	if err != nil {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected an integer resource ID in SIMILAR TO resource(...), got %q", idTok.Value),
			Pos:     idTok.Pos,
			Length:  idTok.Length,
		}
	}

	rp := p.lexer.Next()
	if rp.Type != TokenRParen {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected ')' to close SIMILAR TO resource(, got %q", rp.Value),
			Pos:     rp.Pos,
			Length:  rp.Length,
		}
	}

	within := -1
	if next := p.lexer.Peek(); next.Type == TokenIdentifier && strings.EqualFold(next.Value, "within") {
		p.lexer.Next() // consume WITHIN
		distTok := p.lexer.Next()
		within, err = similarToInt(distTok)
		if err != nil {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected an integer distance after WITHIN, got %q", distTok.Value),
				Pos:     distTok.Pos,
				Length:  distTok.Length,
			}
		}
	}

	return &SimilarToExpr{Token: simTok, TargetID: int64(targetID), Within: within}, nil
}

// similarToInt parses a TokenNumber as a plain non-fractional integer.
func similarToInt(tok Token) (int, error) {
	if tok.Type != TokenNumber || strings.Contains(tok.Value, ".") {
		return 0, fmt.Errorf("not a plain integer")
	}
	return strconv.Atoi(tok.Value)
}

// fieldExpr = field (comparison | inExpr | isExpr)
// field     = IDENT ("." IDENT)?   // max 2 parts
func (p *parser) parseFieldExpr() (Node, error) {
	field, err := p.parseField()
	if err != nil {
		return nil, err
	}

	next := p.lexer.Peek()

	// BETWEEN is not a lexer keyword — it arrives as a bare identifier, matched
	// case-insensitively here (like WITHIN in package 3) so field/meta keys named
	// "between" keep working. `field BETWEEN lo AND hi` desugars to
	// `(field >= lo AND field <= hi)`.
	if next.Type == TokenIdentifier && strings.EqualFold(next.Value, "BETWEEN") {
		return p.parseBetween(field, nil)
	}

	switch next.Type {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte, TokenLike, TokenNotLike, TokenRegex, TokenNotRegex:
		return p.parseComparison(field)

	case TokenIn:
		return p.parseInExpr(field, false)

	case TokenNot:
		// field NOT IN (...) or field NOT BETWEEN lo AND hi
		notTok := p.lexer.Next() // consume NOT
		nextTok := p.lexer.Peek()
		if nextTok.Type == TokenIdentifier && strings.EqualFold(nextTok.Value, "BETWEEN") {
			return p.parseBetween(field, &notTok)
		}
		if nextTok.Type != TokenIn {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected IN or BETWEEN after field NOT, got %q", nextTok.Value),
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

// maxFieldParts is the maximum number of parts in a dotted field expression.
const maxFieldParts = 8

// isFieldNameToken reports whether tok can serve as a field name or dotted
// field segment. TokenHaving is included so the word "having" stays usable as
// a field/meta-key name (the HAVING clause is only recognized inside GROUP BY,
// where it never collides with field parsing).
func isFieldNameToken(tok Token) bool {
	return tok.Type == TokenIdentifier || tok.Type == TokenKwType || tok.Type == TokenHaving
}

// parseField reads a field name: IDENT (. IDENT)* with up to maxFieldParts parts.
func (p *parser) parseField() (*FieldExpr, error) {
	tok := p.lexer.Next()
	if !isFieldNameToken(tok) {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected field name (identifier), got %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	parts := []Token{tok}

	for p.lexer.Peek().Type == TokenDot {
		if len(parts) >= maxFieldParts {
			dotTok := p.lexer.Peek()
			return nil, &ParseError{
				Message: fmt.Sprintf("traversal chain too deep (max %d parts)", maxFieldParts),
				Pos:     dotTok.Pos,
				Length:  dotTok.Length,
			}
		}
		p.lexer.Next() // consume '.'

		nextTok := p.lexer.Next()
		if !isFieldNameToken(nextTok) {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected identifier after '.', got %q", nextTok.Value),
				Pos:     nextTok.Pos,
				Length:  nextTok.Length,
			}
		}
		parts = append(parts, nextTok)
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

// parseBetween parses `BETWEEN lo AND hi` and desugars it into existing nodes:
// `field >= lo AND field <= hi`, wrapped in NotExpr for `NOT BETWEEN`. No new AST
// node — validation, param binding, translation, EXPLAIN, and the filter bar all
// operate on the desugared tree unchanged. The synthesized operator/AND tokens
// carry the BETWEEN token's position so downstream errors still point at the
// query text. The two comparisons get separate FieldExpr nodes (sharing the
// read-only Parts slice) to avoid shared-node aliasing.
func (p *parser) parseBetween(field *FieldExpr, notTok *Token) (Node, error) {
	betweenTok := p.lexer.Next() // consume BETWEEN identifier

	lo, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	andTok := p.lexer.Next()
	if andTok.Type != TokenAnd {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected AND between BETWEEN bounds, got %q", andTok.Value),
			Pos:     andTok.Pos,
			Length:  andTok.Length,
		}
	}

	hi, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	gteTok := Token{Type: TokenGte, Value: ">=", Pos: betweenTok.Pos, Length: betweenTok.Length}
	lteTok := Token{Type: TokenLte, Value: "<=", Pos: betweenTok.Pos, Length: betweenTok.Length}
	andSynth := Token{Type: TokenAnd, Value: "AND", Pos: betweenTok.Pos, Length: betweenTok.Length}

	fieldHi := &FieldExpr{Parts: field.Parts}
	lower := &ComparisonExpr{Field: field, Operator: gteTok, Value: lo}
	upper := &ComparisonExpr{Field: fieldHi, Operator: lteTok, Value: hi}
	andExpr := &BinaryExpr{Left: lower, Operator: andSynth, Right: upper}

	if notTok != nil {
		return &NotExpr{Token: *notTok, Expr: andExpr}, nil
	}
	return andExpr, nil
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

	case TokenParam:
		p.lexer.Next()
		return &ParamRef{Token: tok, Name: tok.Value}, nil

	case TokenIdentifier, TokenHaving:
		// Bare identifier: treat as string value. TokenHaving keeps the word
		// "having" usable as a bare value (e.g. name = having).
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

// parseScope = "SCOPE" (NUMBER | STRING)
// SCOPE accepts a plain integer (group ID) or a string (group name).
// Unit suffixes (kb, mb, gb) are not allowed for scope IDs.
func (p *parser) parseScope() (*ScopeClause, error) {
	scopeTok := p.lexer.Next() // consume SCOPE

	valTok := p.lexer.Next()
	switch valTok.Type {
	case TokenNumber:
		// Reject unit suffixes — scope IDs must be plain integers.
		lower := strings.ToLower(valTok.Value)
		if strings.HasSuffix(lower, "kb") || strings.HasSuffix(lower, "mb") || strings.HasSuffix(lower, "gb") {
			return nil, &ParseError{
				Message: fmt.Sprintf("SCOPE requires a plain integer or string, got %q", valTok.Value),
				Pos:     valTok.Pos,
				Length:  valTok.Length,
			}
		}
		val, err := strconv.ParseFloat(valTok.Value, 64)
		if err != nil {
			return nil, &ParseError{
				Message: fmt.Sprintf("invalid number in SCOPE %q: %v", valTok.Value, err),
				Pos:     valTok.Pos,
				Length:  valTok.Length,
			}
		}
		rawBytes := int64(val)
		return &ScopeClause{
			Token: scopeTok,
			Value: &NumberLiteral{
				Token: valTok,
				Value: val,
				Unit:  "",
				Raw:   rawBytes,
			},
		}, nil

	case TokenString:
		return &ScopeClause{
			Token: scopeTok,
			Value: &StringLiteral{Token: valTok, Value: valTok.Value},
		}, nil

	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected number or string after SCOPE, got %q", valTok.Value),
			Pos:     valTok.Pos,
			Length:  valTok.Length,
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

		// RANDOM() is a context-sensitive sort key, not a lexer keyword: a
		// single-part field spelled RANDOM followed by "()" is the random-order
		// clause. It takes no direction.
		if len(field.Parts) == 1 && strings.EqualFold(field.Parts[0].Value, "RANDOM") && p.lexer.Peek().Type == TokenLParen {
			p.lexer.Next() // consume '('
			rp := p.lexer.Next()
			if rp.Type != TokenRParen {
				return nil, &ParseError{
					Message: fmt.Sprintf("expected ')' after RANDOM(, got %q", rp.Value),
					Pos:     rp.Pos,
					Length:  rp.Length,
				}
			}
			if dir := p.lexer.Peek(); dir.Type == TokenAsc || dir.Type == TokenDesc {
				return nil, &ParseError{
					Message: "RANDOM() does not take a direction",
					Pos:     dir.Pos,
					Length:  dir.Length,
				}
			}
			clauses = append(clauses, OrderByClause{Random: true})
			if p.lexer.Peek().Type != TokenComma {
				break
			}
			p.lexer.Next() // consume ','
			continue
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

	// Parse optional HAVING expression
	if p.lexer.Peek().Type == TokenHaving {
		p.lexer.Next() // consume HAVING
		having, err := p.parseHavingOr()
		if err != nil {
			return nil, err
		}
		clause.Having = having
	}

	return clause, nil
}

// parseHavingOr = havingAnd ("OR" havingAnd)*
func (p *parser) parseHavingOr() (Node, error) {
	left, err := p.parseHavingAnd()
	if err != nil {
		return nil, err
	}
	for p.lexer.Peek().Type == TokenOr {
		opTok := p.lexer.Next()
		right, err := p.parseHavingAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: opTok, Right: right}
	}
	return left, nil
}

// parseHavingAnd = havingUnary ("AND" havingUnary)*
func (p *parser) parseHavingAnd() (Node, error) {
	left, err := p.parseHavingUnary()
	if err != nil {
		return nil, err
	}
	for p.lexer.Peek().Type == TokenAnd {
		opTok := p.lexer.Next()
		right, err := p.parseHavingUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: opTok, Right: right}
	}
	return left, nil
}

// parseHavingUnary = ["NOT"] havingPrimary
func (p *parser) parseHavingUnary() (Node, error) {
	if p.lexer.Peek().Type == TokenNot {
		notTok := p.lexer.Next()
		expr, err := p.parseHavingUnary()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Token: notTok, Expr: expr}, nil
	}
	return p.parseHavingPrimary()
}

// parseHavingPrimary = "(" havingOr ")" | aggregateFunc op value
func (p *parser) parseHavingPrimary() (Node, error) {
	tok := p.lexer.Peek()

	if tok.Type == TokenLParen {
		p.lexer.Next() // consume '('
		expr, err := p.parseHavingOr()
		if err != nil {
			return nil, err
		}
		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected ')' to close '(' in HAVING, got %q", rp.Value),
				Pos:     rp.Pos,
				Length:  rp.Length,
			}
		}
		return expr, nil
	}

	if !isAggregateToken(tok.Type) {
		return nil, &ParseError{
			Message: "HAVING conditions must use aggregate functions; filter plain fields in the WHERE clause instead",
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	agg, err := p.parseAggregateFunc()
	if err != nil {
		return nil, err
	}

	opTok := p.lexer.Next()
	switch opTok.Type {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte:
		// supported
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected comparison operator (=, !=, >, >=, <, <=) after %s in HAVING, got %q", agg.Name, opTok.Value),
			Pos:     opTok.Pos,
			Length:  opTok.Length,
		}
	}

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return &HavingComparison{Agg: agg, Operator: opTok, Value: val}, nil
}

// parseAggregateFunc = ("COUNT" "(" ")" | ("SUM"|"AVG"|"MIN"|"MAX") "(" field ")")
func (p *parser) parseAggregateFunc() (AggregateFunc, error) {
	tok := p.lexer.Next() // consume aggregate keyword
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
		rp := p.lexer.Next()
		if rp.Type != TokenRParen {
			return AggregateFunc{}, &ParseError{
				Message: fmt.Sprintf("COUNT() takes no arguments; expected ')', got %q", rp.Value),
				Pos:     rp.Pos,
				Length:  rp.Length,
			}
		}
	} else {
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
	// Reject fractional values — LIMIT/OFFSET must be whole numbers
	if strings.Contains(raw, ".") {
		return 0, fmt.Errorf("LIMIT/OFFSET requires a whole number, got %q", tok.Value)
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %v", tok.Value, err)
	}
	return val, nil
}
