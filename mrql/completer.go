package mrql

import "strings"

// Suggestion is a single autocompletion candidate returned by Complete.
type Suggestion struct {
	Value string `json:"value"`
	Type  string `json:"type"`  // "field", "operator", "keyword", "entity_type", "value", "function", "rel_date"
	Label string `json:"label,omitempty"` // human-readable label
}

// operators is the full set of comparison operators.
var operators = []Suggestion{
	{Value: "=", Type: "operator"},
	{Value: "!=", Type: "operator"},
	{Value: ">", Type: "operator"},
	{Value: ">=", Type: "operator"},
	{Value: "<", Type: "operator"},
	{Value: "<=", Type: "operator"},
	{Value: "~", Type: "operator", Label: "contains"},
	{Value: "!~", Type: "operator", Label: "not contains"},
	{Value: "IN", Type: "operator"},
	{Value: "NOT IN", Type: "operator"},
	{Value: "IS", Type: "operator"},
}

// postValueKeywords are suggested after a complete field=value expression or ")".
var postValueKeywords = []Suggestion{
	{Value: "AND", Type: "keyword"},
	{Value: "OR", Type: "keyword"},
	{Value: "GROUP BY", Type: "keyword"},
	{Value: "ORDER BY", Type: "keyword"},
	{Value: "LIMIT", Type: "keyword"},
}

// aggregateSuggestions are suggested after GROUP BY field(s).
// SUM/AVG/MIN/MAX show with "(field)" to indicate they require an argument.
var aggregateSuggestions = []Suggestion{
	{Value: "COUNT()", Type: "function", Label: "count rows"},
	{Value: "SUM(field)", Type: "function", Label: "sum of numeric field"},
	{Value: "AVG(field)", Type: "function", Label: "average of numeric field"},
	{Value: "MIN(field)", Type: "function", Label: "minimum value"},
	{Value: "MAX(field)", Type: "function", Label: "maximum value"},
}

// postAggregateKeywords are suggested after aggregate functions in GROUP BY context.
var postAggregateKeywords = []Suggestion{
	{Value: "COUNT()", Type: "function", Label: "count rows"},
	{Value: "SUM(field)", Type: "function", Label: "sum of numeric field"},
	{Value: "AVG(field)", Type: "function", Label: "average of numeric field"},
	{Value: "MIN(field)", Type: "function", Label: "minimum value"},
	{Value: "MAX(field)", Type: "function", Label: "maximum value"},
	{Value: "ORDER BY", Type: "keyword"},
	{Value: "LIMIT", Type: "keyword"},
}

// entityTypeSuggestions are suggested after "type = ".
var entityTypeSuggestions = []Suggestion{
	{Value: "resource", Type: "entity_type"},
	{Value: "note", Type: "entity_type"},
	{Value: "group", Type: "entity_type"},
}

// relDateSuggestions are example relative dates suggested after a date field.
var relDateSuggestions = []Suggestion{
	{Value: "-7d", Type: "rel_date", Label: "7 days ago"},
	{Value: "-30d", Type: "rel_date", Label: "30 days ago"},
	{Value: "-3m", Type: "rel_date", Label: "3 months ago"},
	{Value: "-1y", Type: "rel_date", Label: "1 year ago"},
}

// funcSuggestions are date functions suggested after a date field.
var funcSuggestions = []Suggestion{
	{Value: "NOW()", Type: "function", Label: "current timestamp"},
	{Value: "START_OF_DAY()", Type: "function", Label: "start of today"},
	{Value: "START_OF_WEEK()", Type: "function", Label: "start of this week"},
	{Value: "START_OF_MONTH()", Type: "function", Label: "start of this month"},
	{Value: "START_OF_YEAR()", Type: "function", Label: "start of this year"},
}

// metaSubFieldSuggestions are generic sub-field hints after "meta.".
var metaSubFieldSuggestions = []Suggestion{
	{Value: "meta.<key>", Type: "field", Label: "any meta key"},
}

// traversalSubFieldSuggestions returns field suggestions for parent. / children. / owner. context.
func traversalSubFieldSuggestions(entityType EntityType) []Suggestion {
	var suggestions []Suggestion
	// Common fields valid on groups (since parent/children/owner are always groups)
	for name := range commonIndex {
		if name == "tags" {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field", Label: "group tag"})
		} else {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
		}
	}
	// Group-specific fields (category)
	for name, fd := range groupIndex {
		if fd.Type != FieldRelation {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
		}
	}
	// Allow chaining further traversals
	suggestions = append(suggestions, Suggestion{Value: "parent", Type: "field", Label: "parent group traversal"})
	suggestions = append(suggestions, Suggestion{Value: "children", Type: "field", Label: "child groups traversal"})
	return suggestions
}

// dateFieldNames is the set of field names that hold date/time values.
var dateFieldNames = map[string]bool{
	"created": true,
	"updated": true,
}

// Complete returns autocompletion suggestions for the given query string at the
// specified cursor position.  Only the substring query[:cursor] is analysed so
// that suggestions are relative to where the user is currently typing.
func Complete(query string, cursor int) []Suggestion {
	// Clamp cursor to valid range.
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(query) {
		cursor = len(query)
	}

	prefix := query[:cursor]

	// Tokenise the prefix.
	tokens := tokeniseAll(prefix)

	// Determine the entity type from the prefix so we can narrow field lists.
	entityType := detectEntityType(tokens)

	// Determine context from the last meaningful token(s).
	return suggestionsForContext(tokens, entityType, cursor)
}

// tokeniseAll runs the lexer over input until EOF and returns all tokens
// (excluding the final EOF token).
func tokeniseAll(input string) []Token {
	l := NewLexer(input)
	var tokens []Token
	for {
		tok := l.Next()
		if tok.Type == TokenEOF {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

// detectEntityType scans tokens for a pattern like:
//
//	TYPE = <identifier>   (where <identifier> is resource/note/group)
//	or
//	type = <identifier>
//
// and returns the matching EntityType.
func detectEntityType(tokens []Token) EntityType {
	for i := 0; i+2 < len(tokens); i++ {
		t0 := tokens[i]
		t1 := tokens[i+1]
		t2 := tokens[i+2]
		if (t0.Type == TokenKwType || (t0.Type == TokenIdentifier && strings.ToLower(t0.Value) == "type")) &&
			t1.Type == TokenEq &&
			(t2.Type == TokenIdentifier || t2.Type == TokenString) {
			switch strings.ToLower(t2.Value) {
			case "resource":
				return EntityResource
			case "note":
				return EntityNote
			case "group":
				return EntityGroup
			}
		}
	}
	return EntityUnspecified
}

// fieldSuggestions returns field name suggestions for the given entity type.
// The "type" pseudo-field is always included at the top.
func fieldSuggestions(entityType EntityType) []Suggestion {
	// Start with the "type" pseudo-field which is not in commonFields.
	suggs := []Suggestion{
		{Value: "type", Type: "field", Label: "entity type filter"},
	}

	// Add common fields.
	for _, fd := range commonFields {
		suggs = append(suggs, Suggestion{Value: fd.Name, Type: "field"})
	}

	// Add entity-specific fields.
	var extra []FieldDef
	switch entityType {
	case EntityResource:
		extra = resourceFields
	case EntityNote:
		extra = noteFields
	case EntityGroup:
		extra = groupFields
	}
	seen := make(map[string]bool)
	for _, s := range suggs {
		seen[s.Value] = true
	}
	for _, fd := range extra {
		if !seen[fd.Name] {
			suggs = append(suggs, Suggestion{Value: fd.Name, Type: "field"})
			seen[fd.Name] = true
		}
	}

	// Always add TEXT keyword as a special entry.
	suggs = append(suggs, Suggestion{Value: "TEXT", Type: "keyword", Label: "full-text search"})

	return suggs
}

// groupByFieldSuggestions returns field suggestions valid for GROUP BY.
// Excludes the "type" pseudo-field and TEXT keyword which are not groupable.
func groupByFieldSuggestions(entityType EntityType) []Suggestion {
	var suggs []Suggestion

	// Common fields (all valid for GROUP BY)
	for _, fd := range commonFields {
		suggs = append(suggs, Suggestion{Value: fd.Name, Type: "field"})
	}

	// Entity-specific fields
	var extra []FieldDef
	switch entityType {
	case EntityResource:
		extra = resourceFields
	case EntityNote:
		extra = noteFields
	case EntityGroup:
		extra = groupFields
	}
	seen := make(map[string]bool)
	for _, s := range suggs {
		seen[s.Value] = true
	}
	for _, fd := range extra {
		if !seen[fd.Name] {
			suggs = append(suggs, Suggestion{Value: fd.Name, Type: "field"})
			seen[fd.Name] = true
		}
	}

	// meta.* hint
	suggs = append(suggs, Suggestion{Value: "meta.<key>", Type: "field", Label: "any meta key"})

	// Traversal hints
	if entityType == EntityResource || entityType == EntityNote {
		suggs = append(suggs, Suggestion{Value: "owner.name", Type: "field", Label: "owner group name"})
		suggs = append(suggs, Suggestion{Value: "owner.meta.<key>", Type: "field", Label: "owner meta field"})
	}
	if entityType == EntityGroup {
		suggs = append(suggs, Suggestion{Value: "parent.name", Type: "field", Label: "parent group name"})
	}

	return suggs
}

// isInGroupByClause returns true if the cursor is within the GROUP BY clause
// (between GROUP BY and ORDER BY/LIMIT/OFFSET/EOF). Returns false if we've
// moved past into ORDER BY or LIMIT territory.
func isInGroupByClause(tokens []Token) bool {
	foundGroupBy := false
	for _, t := range tokens {
		if t.Type == TokenGroupBy {
			foundGroupBy = true
		}
		// Once we see ORDER BY, LIMIT, or OFFSET after GROUP BY, we've left the clause
		if foundGroupBy && (t.Type == TokenOrderBy || t.Type == TokenLimit || t.Type == TokenOffset) {
			return false
		}
	}
	return foundGroupBy
}

// suggestionsForContext analyses the token stream and returns the appropriate
// suggestions for the cursor position.
func suggestionsForContext(tokens []Token, entityType EntityType, cursor int) []Suggestion {
	// Empty prefix — suggest fields.
	if len(tokens) == 0 {
		return fieldSuggestions(entityType)
	}

	last := tokens[len(tokens)-1]

	// Check if cursor is immediately after the last token (no trailing space).
	// If so, the user is still typing that token — suggest completions for it,
	// not what comes after it.
	cursorAtTokenEnd := (last.Pos + last.Length) == cursor

	// After GROUP BY — suggest groupable fields (no type pseudo-field, no TEXT).
	if last.Type == TokenGroupBy {
		return groupByFieldSuggestions(entityType)
	}

	// After AND / OR / NOT / "(" — suggest fields.
	switch last.Type {
	case TokenAnd, TokenOr, TokenNot, TokenLParen:
		return fieldSuggestions(entityType)
	}

	// After a dot — context depends on what's before the dot.
	if last.Type == TokenDot && len(tokens) >= 2 {
		prev := tokens[len(tokens)-2]
		switch prev.Value {
		case "parent", "children", "owner":
			return traversalSubFieldSuggestions(entityType)
		default:
			return metaSubFieldSuggestions
		}
	}

	// Check for GROUP BY clause context: between GROUP BY and ORDER BY/LIMIT.
	if isInGroupByClause(tokens) {
		// After a field name in GROUP BY clause (with space after it),
		// suggest aggregates + ORDER BY + LIMIT.
		if last.Type == TokenIdentifier || last.Type == TokenKwType {
			if !cursorAtTokenEnd {
				var suggs []Suggestion
				suggs = append(suggs, aggregateSuggestions...)
				suggs = append(suggs, Suggestion{Value: "ORDER BY", Type: "keyword"})
				suggs = append(suggs, Suggestion{Value: "LIMIT", Type: "keyword"})
				return suggs
			}
			// Still typing the field name — suggest groupable fields only.
			return groupByFieldSuggestions(entityType)
		}
		// After closing paren in GROUP BY clause — suggest more aggregates or ORDER BY/LIMIT.
		if last.Type == TokenRParen {
			return postAggregateKeywords
		}
	}

	// After an identifier or keyword: if cursor is right at the token end
	// (no space), the user is still typing the field name → suggest fields.
	// If there's a space after, the field name is complete → suggest operators.
	if last.Type == TokenIdentifier || last.Type == TokenKwType {
		if cursorAtTokenEnd {
			return fieldSuggestions(entityType)
		}
		return operators
	}

	// After an operator — decide what value to suggest.
	if isOperatorToken(last) {
		return valuesuggestions(tokens, entityType)
	}

	// After "NOT IN" — the last two tokens would be TokenNot, TokenIn.
	// But our lexer emits NOT and IN as separate tokens.
	if last.Type == TokenIn {
		// Could be part of "NOT IN" or standalone "IN" — in either case we want value suggestions.
		return valuesuggestions(tokens, entityType)
	}

	// After a string literal, number, rel-date, function, or closing paren
	// — suggest logical connectives / ORDER BY / LIMIT.
	switch last.Type {
	case TokenString, TokenNumber, TokenRelDate, TokenFunc, TokenRParen:
		return postValueKeywords
	}

	// After ORDER BY, ASC, DESC, LIMIT, OFFSET — no further field completions needed here.
	// Return empty for now (the API layer may add numeric hints).
	return nil
}

// isOperatorToken returns true when tok is a comparison/equality operator.
func isOperatorToken(tok Token) bool {
	switch tok.Type {
	case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte, TokenLike, TokenNotLike, TokenIs:
		return true
	}
	return false
}

// valuesuggestions returns value-level suggestions based on the field that
// precedes the operator.
func valuesuggestions(tokens []Token, entityType EntityType) []Suggestion {
	// Walk backwards to find the field name before the operator(s).
	fieldName := extractFieldBeforeOperator(tokens)

	// "type" field → suggest entity types.
	if strings.ToLower(fieldName) == "type" {
		return entityTypeSuggestions
	}

	// Date fields → suggest relative dates + functions.
	if dateFieldNames[strings.ToLower(fieldName)] {
		var suggs []Suggestion
		suggs = append(suggs, relDateSuggestions...)
		suggs = append(suggs, funcSuggestions...)
		return suggs
	}

	// Generic hint.
	return []Suggestion{
		{Value: `"value"`, Type: "value", Label: "enter a value"},
	}
}

// extractFieldBeforeOperator scans backwards through tokens to find the field
// name (simple or qualified) that appears before the most recent operator.
// Returns the field name string, or "" if not found.
func extractFieldBeforeOperator(tokens []Token) string {
	// Find the last operator index.
	opIdx := -1
	for i := len(tokens) - 1; i >= 0; i-- {
		if isOperatorToken(tokens[i]) || tokens[i].Type == TokenIn {
			opIdx = i
			break
		}
	}
	if opIdx <= 0 {
		return ""
	}

	// The field is the identifier(s) immediately before the operator.
	// It may be a simple name or a dotted name (meta.key).
	prev := tokens[opIdx-1]
	if prev.Type == TokenIdentifier || prev.Type == TokenKwType {
		// Simple field name.
		return prev.Value
	}
	// Could be second part of a dotted name (e.g. "meta.key" → last part is "key").
	if opIdx >= 3 && prev.Type == TokenIdentifier {
		dot := tokens[opIdx-2]
		base := tokens[opIdx-3]
		if dot.Type == TokenDot && (base.Type == TokenIdentifier || base.Type == TokenKwType) {
			return base.Value + "." + prev.Value
		}
	}
	return ""
}
