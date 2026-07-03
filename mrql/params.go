package mrql

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// AggregatedColumns returns the ordered result-map column names for an
// aggregated GROUP BY query: the (deduplicated) group-by field names followed
// by the aggregate aliases, matching the SELECT aliases used by
// BuildAggregatedGroupBy. Returns nil when the query is not an aggregated
// GROUP BY. Call after Validate (which deduplicates GroupBy.Fields).
func AggregatedColumns(q *Query) []string {
	if q == nil || q.GroupBy == nil || len(q.GroupBy.Aggregates) == 0 {
		return nil
	}
	cols := make([]string, 0, len(q.GroupBy.Fields)+len(q.GroupBy.Aggregates))
	for _, f := range q.GroupBy.Fields {
		cols = append(cols, f.Name())
	}
	for _, agg := range q.GroupBy.Aggregates {
		if agg.Field == nil {
			cols = append(cols, "count")
		} else {
			cols = append(cols, strings.ToLower(agg.Name)+"_"+agg.Field.Name())
		}
	}
	return cols
}

// BucketKeyColumns returns the ordered group-by key column names for a bucketed
// GROUP BY query (the field names, matching MRQLBucket.Key entries). Returns
// nil when the query is not a bucketed GROUP BY.
func BucketKeyColumns(q *Query) []string {
	if q == nil || q.GroupBy == nil || len(q.GroupBy.Aggregates) > 0 {
		return nil
	}
	cols := make([]string, 0, len(q.GroupBy.Fields))
	for _, f := range q.GroupBy.Fields {
		cols = append(cols, f.Name())
	}
	return cols
}

// ListParams returns the distinct parameter placeholder names ($name) used in
// the query, in first-appearance order. Names are returned without the leading
// '$'. Placeholders are only valid in value positions (comparison RHS, IN list
// items, HAVING comparison RHS), so those are the only positions walked.
func ListParams(q *Query) []string {
	if q == nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	walkValueNodes(q, func(np *Node) {
		if pr, ok := (*np).(*ParamRef); ok {
			if !seen[pr.Name] {
				seen[pr.Name] = true
				names = append(names, pr.Name)
			}
		}
	})
	return names
}

// BindParams substitutes every ParamRef in the query with a concrete literal
// node derived from the supplied params map, mutating the AST in place. It
// performs strict checking: every placeholder must have a supplied value
// (missing → error) and every supplied key must correspond to a placeholder
// (unknown/extra → error). Param names are case-sensitive.
//
// Substitution happens at the AST value level (never string interpolation), so
// bound values translate to GORM bind placeholders exactly like typed literals.
func BindParams(q *Query, params map[string]any) error {
	if q == nil {
		return nil
	}
	required := ListParams(q)
	reqSet := make(map[string]bool, len(required))
	for _, n := range required {
		reqSet[n] = true
	}

	// Missing placeholders (in first-appearance order).
	var missing []string
	for _, n := range required {
		if _, ok := params[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s", missingParamMessage(missing))
	}

	// Unknown/extra supplied params (sorted for a stable message).
	var extra []string
	for k := range params {
		if !reqSet[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return fmt.Errorf("%s", unknownParamMessage(extra))
	}

	var bindErr error
	walkValueNodes(q, func(np *Node) {
		if bindErr != nil {
			return
		}
		if pr, ok := (*np).(*ParamRef); ok {
			node, err := coerceParamValue(params[pr.Name], pr.Token)
			if err != nil {
				bindErr = err
				return
			}
			*np = node
		}
	})
	return bindErr
}

func missingParamMessage(names []string) string {
	if len(names) == 1 {
		return "missing parameter $" + names[0]
	}
	return "missing parameters: " + strings.Join(dollarize(names), ", ")
}

func unknownParamMessage(names []string) string {
	if len(names) == 1 {
		return "unknown parameter $" + names[0]
	}
	return "unknown parameters: " + strings.Join(dollarize(names), ", ")
}

func dollarize(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "$" + n
	}
	return out
}

// coerceParamValue converts a supplied param value into an AST literal node,
// mirroring how the MRQL lexer would interpret the value if typed at that
// position. tok carries the placeholder's source position for later errors.
func coerceParamValue(raw any, tok Token) (Node, error) {
	switch v := raw.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			// Non-numeric json.Number should not occur, but fall back to string.
			return &StringLiteral{Token: tok, Value: v.String()}, nil
		}
		return numberLiteralFromFloat(f, tok), nil
	case float64:
		return numberLiteralFromFloat(v, tok), nil
	case float32:
		return numberLiteralFromFloat(float64(v), tok), nil
	case int:
		return numberLiteralFromFloat(float64(v), tok), nil
	case int8:
		return numberLiteralFromFloat(float64(v), tok), nil
	case int16:
		return numberLiteralFromFloat(float64(v), tok), nil
	case int32:
		return numberLiteralFromFloat(float64(v), tok), nil
	case int64:
		return numberLiteralFromFloat(float64(v), tok), nil
	case uint, uint8, uint16, uint32, uint64:
		return numberLiteralFromFloat(float64(reflectToUint64(v)), tok), nil
	case string:
		return coerceStringParam(v, tok), nil
	case bool:
		return &StringLiteral{Token: tok, Value: strconv.FormatBool(v)}, nil
	case nil:
		return &StringLiteral{Token: tok, Value: ""}, nil
	default:
		return &StringLiteral{Token: tok, Value: fmt.Sprint(v)}, nil
	}
}

func reflectToUint64(v any) uint64 {
	switch n := v.(type) {
	case uint:
		return uint64(n)
	case uint8:
		return uint64(n)
	case uint16:
		return uint64(n)
	case uint32:
		return uint64(n)
	case uint64:
		return n
	}
	return 0
}

// numberLiteralFromFloat builds a unit-less NumberLiteral from a float value.
func numberLiteralFromFloat(f float64, tok Token) *NumberLiteral {
	raw := int64(math.Round(f))
	if f == float64(int64(f)) {
		raw = int64(f)
	}
	return &NumberLiteral{Token: tok, Value: f, Unit: "", Raw: raw}
}

// coerceStringParam interprets a supplied string value. If the string lexes to
// exactly one value token followed by EOF — a number (optionally unit-suffixed),
// a relative date (-7d), a date function (NOW()), or a "quoted string" (which
// unwraps) — that literal is used. Anything else (spaces, operators, bare
// identifiers, multiple tokens) becomes a plain StringLiteral of the raw input,
// which keeps injection-shaped strings inert.
func coerceStringParam(s string, tok Token) Node {
	lex := NewLexer(s)
	first := lex.Next()
	second := lex.Next()
	if second.Type == TokenEOF {
		switch first.Type {
		case TokenString:
			// "quoted string" — unwrap to its literal content (force-string hatch).
			return &StringLiteral{Token: tok, Value: first.Value}
		case TokenNumber:
			if nl, err := parseNumberLiteral(first); err == nil {
				return nl
			}
		case TokenRelDate:
			if rd, err := parseRelDateLiteral(first); err == nil {
				return rd
			}
		case TokenFunc:
			return &FuncCall{Token: first, Name: first.Value}
		}
	}
	return &StringLiteral{Token: tok, Value: s}
}

// walkValueNodes visits a pointer to every value-position Node in the query
// (comparison RHS, IN list items, HAVING comparison RHS), so callers can read
// or replace ParamRef placeholders in place.
func walkValueNodes(q *Query, visit func(*Node)) {
	walkExprValues(q.Where, visit)
	if q.GroupBy != nil {
		walkHavingValues(q.GroupBy.Having, visit)
	}
}

func walkExprValues(node Node, visit func(*Node)) {
	switch n := node.(type) {
	case *BinaryExpr:
		walkExprValues(n.Left, visit)
		walkExprValues(n.Right, visit)
	case *NotExpr:
		walkExprValues(n.Expr, visit)
	case *ComparisonExpr:
		visit(&n.Value)
	case *InExpr:
		for i := range n.Values {
			visit(&n.Values[i])
		}
	}
	// IsExpr, TextSearchExpr, SimilarToExpr carry no value-position placeholders.
}

func walkHavingValues(node Node, visit func(*Node)) {
	switch n := node.(type) {
	case *BinaryExpr:
		walkHavingValues(n.Left, visit)
		walkHavingValues(n.Right, visit)
	case *NotExpr:
		walkHavingValues(n.Expr, visit)
	case *HavingComparison:
		visit(&n.Value)
	}
}
