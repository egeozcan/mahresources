package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

func extractRawValueAtPath(metaRaw json.RawMessage, path string) any {
	if len(metaRaw) == 0 || path == "" {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil
	}
	parts := strings.Split(path, ".")
	var current any = meta
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[part]
		if !ok {
			return nil
		}
	}
	return current
}

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
		if !field.IsValid() {
			return nil, nil
		}
		return formatFieldValue(field), nil
	}
	return extractRawValueAtPath(ctx.Meta, attr("path")), nil
}

// hasConditionSource reports whether a numbered-suffix condition names any value
// source (mrql/field/path). Used to stop the multi-value suffix loop.
func hasConditionSource(attrs map[string]string, suffix string) bool {
	return attrs["mrql"+suffix] != "" || attrs["field"+suffix] != "" || attrs["path"+suffix] != ""
}

// evaluateConditionSet resolves and evaluates every condition on the attribute
// set — the base condition plus any numbered-suffix conditions (path2, eq2, …) —
// folding operators within a condition and the conditions themselves with the
// same combine mode: all (AND, default) or any (OR, combine="any"). Returns an
// error only when a source (e.g. an MRQL query) fails to resolve.
func evaluateConditionSet(reqCtx context.Context, attrs map[string]string, ctx MetaShortcodeContext, executor QueryExecutor) (bool, error) {
	combineAny := attrs["combine"] == "any"

	var results []bool
	for i := 1; ; i++ {
		suffix := ""
		if i >= 2 {
			suffix = strconv.Itoa(i)
			if !hasConditionSource(attrs, suffix) {
				break
			}
		}
		value, err := resolveConditionalValue(reqCtx, attrs, suffix, ctx, executor)
		if err != nil {
			return false, err
		}
		results = append(results, evaluateCondition(value, attrs, suffix, combineAny))
	}
	return foldBools(results, combineAny), nil
}

// evaluateCondition folds every operator present for one condition (identified
// by suffix) with the combine mode. A condition with no operators is false,
// preserving the "needs a comparison operator" contract.
func evaluateCondition(value any, attrs map[string]string, suffix string, combineAny bool) bool {
	var results []bool
	for _, op := range conditionalOperators {
		operand, ok := attrs[op+suffix]
		if !ok {
			continue
		}
		results = append(results, evaluateOperator(value, op, operand))
	}
	if len(results) == 0 {
		return false
	}
	return foldBools(results, combineAny)
}

// evaluateOperator applies a single operator to the tested value.
func evaluateOperator(value any, op, operand string) bool {
	switch op {
	case "eq":
		return fmt.Sprint(value) == operand
	case "neq":
		return fmt.Sprint(value) != operand
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
		target := fmt.Sprint(value)
		for _, item := range strings.Split(operand, ",") {
			if strings.TrimSpace(item) == target {
				return true
			}
		}
		return false
	case "contains":
		return strings.Contains(fmt.Sprint(value), operand)
	case "matches":
		re, err := regexp.Compile(operand)
		if err != nil {
			return false
		}
		return re.MatchString(fmt.Sprint(value))
	case "empty":
		return value == nil || fmt.Sprint(value) == ""
	case "not-empty":
		return value != nil && fmt.Sprint(value) != ""
	}
	return false
}

// foldBools reduces results to a single verdict: AND by default, OR when any.
// An empty slice folds to false (no condition asserted).
func foldBools(results []bool, any bool) bool {
	if len(results) == 0 {
		return false
	}
	if any {
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	}
	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
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
		return sc.Raw
	}

	branches := SplitBranches(sc.InnerContent)
	for i, br := range branches {
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
