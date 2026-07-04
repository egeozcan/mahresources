package mrql

import (
	"strconv"
	"strings"
)

// InsertScopeClause returns query with a `SCOPE <groupID>` clause spliced in at
// the grammatically correct position: after any WHERE expression and before the
// first of GROUP BY / ORDER BY / LIMIT / OFFSET / HAVING (the clause order the
// parser enforces). It is used to build a "view all" link that reproduces a
// shortcode's scoped result set.
//
// A naive append (query + " SCOPE <id>") is invalid for any query carrying one
// of those trailing clauses, since SCOPE may not follow them. This lexes the
// query so keywords inside string literals are never mistaken for clauses.
//
// If the query already carries an explicit SCOPE, or cannot be lexed cleanly, it
// is returned unchanged — the caller only scopes queries that lack one.
func InsertScopeClause(query string, groupID uint) string {
	clause := "SCOPE " + strconv.FormatUint(uint64(groupID), 10)
	lex := NewLexer(query)
	for {
		tok := lex.Next()
		switch tok.Type {
		case TokenScope:
			// Already scoped — do not add a second SCOPE clause.
			return query
		case TokenGroupBy, TokenOrderBy, TokenLimit, TokenOffset, TokenHaving:
			head := strings.TrimRight(query[:tok.Pos], " \t\r\n")
			return head + " " + clause + " " + query[tok.Pos:]
		case TokenEOF:
			return strings.TrimRight(query, " \t\r\n") + " " + clause
		case TokenIllegal:
			// Cannot trust token positions on unlexable input; degrade to the
			// unscoped query rather than emitting a broken clause.
			return query
		}
	}
}
