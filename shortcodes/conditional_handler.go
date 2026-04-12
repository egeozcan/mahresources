package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"reflect"
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

func resolveConditionalValue(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, executor QueryExecutor) (any, error) {
	if query := sc.Attrs["mrql"]; query != "" && executor != nil {
		scope := resolveScopeKeyword(sc.Attrs["scope"], ctx)
		limit := parseIntAttr(sc.Attrs["limit"], defaultMRQLShortcodeLimit)
		buckets := parseIntAttr(sc.Attrs["buckets"], defaultMRQLShortcodeBuckets)
		result, err := executor(reqCtx, query, "", limit, buckets, scope)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, nil
		}
		switch result.Mode {
		case "flat", "":
			return float64(len(result.Items)), nil
		case "aggregated":
			agg := sc.Attrs["aggregate"]
			if agg == "" {
				return nil, fmt.Errorf("mrql aggregated results require aggregate=\"column_name\" attribute")
			}
			if len(result.Rows) == 0 {
				return nil, nil
			}
			return result.Rows[0][agg], nil
		case "bucketed":
			return float64(len(result.Groups)), nil
		}
		return nil, nil
	}
	if fieldName := sc.Attrs["field"]; fieldName != "" && ctx.Entity != nil {
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
	return extractRawValueAtPath(ctx.Meta, sc.Attrs["path"]), nil
}

func evaluateCondition(value any, attrs map[string]string) bool {
	if _, ok := attrs["eq"]; ok {
		return fmt.Sprint(value) == attrs["eq"]
	}
	if _, ok := attrs["neq"]; ok {
		return fmt.Sprint(value) != attrs["neq"]
	}
	if gtStr, ok := attrs["gt"]; ok {
		lhs, lhsOk := toFloat(value)
		rhs, rhsOk := toFloat(gtStr)
		return lhsOk && rhsOk && lhs > rhs
	}
	if ltStr, ok := attrs["lt"]; ok {
		lhs, lhsOk := toFloat(value)
		rhs, rhsOk := toFloat(ltStr)
		return lhsOk && rhsOk && lhs < rhs
	}
	if substr, ok := attrs["contains"]; ok {
		return strings.Contains(fmt.Sprint(value), substr)
	}
	if _, ok := attrs["empty"]; ok {
		return value == nil || fmt.Sprint(value) == ""
	}
	if _, ok := attrs["not-empty"]; ok {
		return value != nil && fmt.Sprint(value) != ""
	}
	return false
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
	value, err := resolveConditionalValue(reqCtx, sc, ctx, executor)
	if err != nil {
		return fmt.Sprintf(
			`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
			html.EscapeString(err.Error()),
		)
	}
	conditionMet := evaluateCondition(value, sc.Attrs)
	ifBranch, elseBranch := SplitElse(sc.InnerContent)
	var selected string
	if conditionMet {
		selected = ifBranch
	} else {
		selected = elseBranch
	}
	if selected == "" {
		return ""
	}
	return processWithDepth(reqCtx, selected, ctx, renderer, executor, depth+1)
}
