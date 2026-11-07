package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"reflect"
	"strconv"
	"strings"
)

// resolveConditionalValue resolves the tested value for one condition, reading
// its source attributes with the given numbered suffix ("" for the base
// condition, "2"/"3"/… for additional multi-value conditions). Priority mirrors
// the single-condition behavior: mrql, then field, then meta path.
func resolveConditionalValue(reqCtx context.Context, attrs map[string]string, suffix string, ctx MetaShortcodeContext, executor QueryExecutor) (any, error) {
	attr := func(base string) string { return attrs[base+suffix] }

	if query := attr("mrql"); query != "" && executor != nil {
		scope := resolveScopeKeyword(attr("scope"), ctx)
		limit := parseIntAttr(attr("limit"), defaultMRQLShortcodeLimit)
		buckets := parseIntAttr(attr("buckets"), defaultMRQLShortcodeBuckets)
		params := collectShortcodeParams(attrs)
		result, err := executor(reqCtx, query, QueryOptions{
			Params:       params,
			Limit:        limit,
			Buckets:      buckets,
			ScopeGroupID: scope,
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, nil
		}
		// Aggregated results need an explicit column to test; other modes fold to
		// a count. extractScalarFromResult is the shared value-extraction helper
		// (also used by inline [mrql value=]), so the two can never drift apart.
		if result.Mode == "aggregated" {
			agg := attr("aggregate")
			if agg == "" {
				return nil, fmt.Errorf("mrql aggregated results require aggregate=\"column_name\" attribute")
			}
			return extractScalarFromResult(result, agg), nil
		}
		return extractScalarFromResult(result, "count"), nil
	}
	if fieldName := attr("field"); fieldName != "" && ctx.Entity != nil {
		v := reflect.ValueOf(ctx.Entity)
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil, nil
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil, nil
		}
		field := v.FieldByName(fieldName)
		if !field.IsValid() || !field.CanInterface() {
			return nil, nil
		}
		return formatFieldValue(field), nil
	}
	return ctx.rawValueAtPath(attr("path")), nil
}

// hasConditionSource reports whether a numbered-suffix condition names any value
// source (mrql/field/path). Used to stop the multi-value suffix loop.
func hasConditionSource(attrs map[string]string, suffix string) bool {
	return attrs["mrql"+suffix] != "" || attrs["field"+suffix] != "" || attrs["path"+suffix] != ""
}

// condFold folds booleans incrementally with the combine mode, so no slice of
// every result is kept: AND by default, OR when any. Nothing folded is false.
type condFold struct {
	any  bool
	seen bool
	val  bool
}

func (f *condFold) add(r bool) {
	if !f.seen {
		f.seen, f.val = true, r
		return
	}
	if f.any {
		f.val = f.val || r
	} else {
		f.val = f.val && r
	}
}

func (f *condFold) result() bool { return f.seen && f.val }

// evaluateConditionSet resolves and evaluates every condition on the attribute
// set — the base condition plus any numbered-suffix conditions (path2, eq2, …) —
// folding operators within a condition and the conditions themselves with the
// same combine mode: all (AND, default) or any (OR, combine="any"). Returns an
// error only when a source (e.g. an MRQL query) fails to resolve. Every source
// is still resolved, as before; the context is consulted before each condition,
// operator and result fold, and a stopped render ends the loop.
func evaluateConditionSet(reqCtx context.Context, attrs map[string]string, ctx MetaShortcodeContext, executor QueryExecutor) (bool, error) {
	m := meterFrom(reqCtx)
	fold := condFold{any: attrs["combine"] == "any"}
	for i := 1; ; i++ {
		suffix := ""
		if i >= 2 {
			suffix = strconv.Itoa(i)
			if !hasConditionSource(attrs, suffix) {
				break
			}
		}
		if !m.alive() {
			return false, m.Err()
		}
		value, err := resolveConditionalValue(reqCtx, attrs, suffix, ctx, executor)
		if err != nil {
			return false, err
		}
		fold.add(evaluateConditionM(m, value, attrs, suffix, fold.any))
	}
	if err := m.Err(); err != nil {
		return false, err
	}
	return fold.result(), nil
}

// evaluateCondition folds every operator present for one condition (identified
// by suffix) with the combine mode. A condition with no operators is false,
// preserving the "needs a comparison operator" contract.
func evaluateCondition(value any, attrs map[string]string, suffix string, combineAny bool) bool {
	return evaluateConditionM(nil, value, attrs, suffix, combineAny)
}

func evaluateConditionM(m *renderMeter, value any, attrs map[string]string, suffix string, combineAny bool) bool {
	fold := condFold{any: combineAny}
	// The value's text is built once per condition, not once per operator.
	var text string
	var haveText bool
	str := func() string {
		if !haveText {
			text, haveText = fmt.Sprint(value), true
		}
		return text
	}
	for _, op := range conditionalOperators {
		operand, ok := attrs[op+suffix]
		if !ok {
			continue
		}
		if !m.alive() {
			return false
		}
		fold.add(evaluateOperatorM(m, value, str, op, operand))
	}
	return fold.result()
}

// evaluateOperator applies a single operator to the tested value.
func evaluateOperator(value any, op, operand string) bool {
	return evaluateOperatorM(nil, value, func() string { return fmt.Sprint(value) }, op, operand)
}

func evaluateOperatorM(m *renderMeter, value any, str func() string, op, operand string) bool {
	switch op {
	case "eq":
		return str() == operand
	case "neq":
		return str() != operand
	case "gt", "gte", "lt", "lte":
		lhs, lhsOk := toFloat(value)
		rhs, rhsOk := toFloat(operand)
		if !lhsOk || !rhsOk {
			return false
		}
		switch op {
		case "gt":
			return lhs > rhs
		case "gte":
			return lhs >= rhs
		case "lt":
			return lhs < rhs
		default:
			return lhs <= rhs
		}
	case "in":
		target := str()
		// Walk the operand in place rather than splitting it into a slice.
		for rest, more := operand, true; more; {
			var item string
			item, rest, more = strings.Cut(rest, ",")
			if strings.TrimSpace(item) == target {
				return true
			}
			if !m.tick() {
				return false
			}
		}
		return false
	case "contains":
		return strings.Contains(str(), operand)
	case "matches":
		re, ok := m.regexp(operand)
		if !ok || re == nil {
			return false
		}
		return re.MatchString(str())
	case "empty":
		return value == nil || str() == ""
	case "not-empty":
		return value != nil && str() != ""
	}
	return false
}

// foldBools reduces results to a single verdict: AND by default, OR when any.
// An empty slice folds to false (no condition asserted).
func foldBools(results []bool, any bool) bool {
	f := condFold{any: any}
	for _, r := range results {
		f.add(r)
	}
	return f.result()
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		f, err := strconv.ParseFloat(fmt.Sprint(v), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
}

func RenderConditionalShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		// A [conditional] with no closing tag can't gate anything — surface it as
		// an author-facing marker instead of leaking the raw opening tag.
		return shortcodeErrorMarker("conditional", "conditional requires a closing [/conditional]")
	}

	m := meterFrom(reqCtx)
	branches := SplitBranches(sc.InnerContent)
	for i, br := range branches {
		if !m.alive() {
			return ""
		}
		matched := true
		if !br.IsElse {
			// Branch 0 is guarded by the opening tag's attrs; [elseif] arms by
			// their own attrs.
			guard := sc.Attrs
			if i > 0 {
				guard = br.Attrs
			}
			ok, err := evaluateConditionSet(reqCtx, guard, ctx, executor)
			if err != nil {
				if errors.Is(err, ErrQueryUnavailable) {
					// Not "no match": no [elseif] or [else] arm may be taken,
					// so the whole conditional becomes the inert comment.
					return unavailableComment(err)
				}
				if m.Err() != nil {
					return ""
				}
				return fmt.Sprintf(
					`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
					html.EscapeString(err.Error()),
				)
			}
			matched = ok
		}
		if matched {
			if br.Content == "" {
				return ""
			}
			return processWithDepth(reqCtx, br.Content, ctx, renderer, executor, depth+1)
		}
	}
	return ""
}
