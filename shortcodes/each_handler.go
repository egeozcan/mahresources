package shortcodes

import (
	"context"
	"html"
	"reflect"
	"strconv"
	"strings"
)

// defaultEachLimit caps [each] iterations when no limit= is given. Templates
// render inline on entity pages, so the default is generous but bounded.
const defaultEachLimit = 100

// RenderEachShortcode expands an [each] block by iterating an array value at the
// meta path. Each element renders the item-branch once, with [item …] tokens
// substituted for that element; the substituted branch is then processed with
// the parent entity context so [conditional]/[mrql]/[meta] keep working inside
// the loop. A non-array or empty value renders the top-level [else] branch
// (nothing when there is no [else]).
func RenderEachShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return ""
	}

	itemBranch, elseBranch := SplitElse(sc.InnerContent)

	renderElse := func() string {
		if strings.TrimSpace(elseBranch) == "" {
			return ""
		}
		return processWithDepth(reqCtx, elseBranch, ctx, renderer, executor, depth+1)
	}

	arr, ok := extractRawValueAtPath(ctx.Meta, sc.Attrs["path"]).([]any)
	if !ok || len(arr) == 0 {
		return renderElse()
	}

	limit := parseIntAttr(sc.Attrs["limit"], defaultEachLimit)

	var b strings.Builder
	for i, elem := range arr {
		if i >= limit {
			break
		}
		substituted := substituteItems(itemBranch, elem, i+1)
		b.WriteString(processWithDepth(reqCtx, substituted, ctx, renderer, executor, depth+1))
	}
	return b.String()
}

// substituteItems replaces every top-level [item …] token in branch with its
// rendered value for elem (index is the 1-based position). [item] tokens inside
// a nested [each] block are not top-level (they live in that block's inner
// content), so they are left untouched to bind to the nearest enclosing [each].
func substituteItems(branch string, elem any, index int) string {
	scs := ParseWithBlocks(branch)
	if len(scs) == 0 {
		return branch
	}
	var b strings.Builder
	lastEnd := 0
	for _, sc := range scs {
		if sc.Name != "item" {
			continue
		}
		b.WriteString(branch[lastEnd:sc.Start])
		b.WriteString(renderItemValue(sc, elem, index))
		lastEnd = sc.End
	}
	b.WriteString(branch[lastEnd:])
	return b.String()
}

// renderItemValue renders one [item] occurrence for the current element.
// [item index="true"] renders the 1-based position; otherwise the element (or a
// dot-path into it) is formatted with the same format=/layout=/default= helpers
// as [property]. Output is HTML-escaped unless raw="true".
func renderItemValue(sc Shortcode, elem any, index int) string {
	if sc.Attrs["index"] == "true" {
		return strconv.Itoa(index)
	}

	value := navigateJSONValue(elem, sc.Attrs["path"])
	text := formatItemValue(value, sc.Attrs["format"], sc.Attrs["layout"])

	if text == "" {
		if def := sc.Attrs["default"]; def != "" {
			text = def
		}
	}

	if sc.Attrs["raw"] == "true" {
		return text
	}
	return html.EscapeString(text)
}

// navigateJSONValue walks a dot-separated path into a decoded JSON value
// (map[string]any at each step). An empty path returns current unchanged; a
// missing segment or non-object step returns nil.
func navigateJSONValue(current any, path string) any {
	if path == "" {
		return current
	}
	for _, part := range strings.Split(path, ".") {
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

// formatItemValue formats a decoded JSON value using the property handler's
// format/layout helpers so [item] and [property] format uniformly.
func formatItemValue(v any, format, layout string) string {
	if v == nil {
		return ""
	}
	return formatPropertyValue(reflect.ValueOf(v), format, layout)
}
