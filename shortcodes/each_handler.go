package shortcodes

import (
	"context"
	"crypto/rand"
	"html"
	"strconv"
	"strings"
)

// defaultEachLimit caps [each] iterations when no limit= is given. Templates
// render inline on entity pages, so the default is generous but bounded.
const defaultEachLimit = 100

// itemSentinelPrefix opens the inert placeholder that stands in for one [item]
// occurrence while the branch is processed. NUL carries no meaning in a template
// and is stripped from every rendered item value, so the bytes cannot come from
// the data; the per-process random component means that even a NUL smuggled into
// the output by some other handler cannot forge a placeholder.
var itemSentinelPrefix = "\x00mahitem" + rand.Text() + ":"

func itemSentinel(i int) string { return itemSentinelPrefix + strconv.Itoa(i) + "\x00" }

// RenderEachShortcode expands an [each] block by iterating an array value at the
// meta path. The item-branch is processed once per element with the parent
// entity context, so [conditional]/[mrql]/[meta] keep working inside the loop,
// and the element's own values are spliced into that *output*. A non-array or
// empty value renders the top-level [else] branch (nothing when there is no
// [else]).
//
// Splicing after processing rather than before is the whole point of the two
// steps. Substituting first made every array element template source: escaping
// leaves "[" and "]" alone, and parseAttrs runs html.UnescapeString over the
// attribute string, so an element could carry a working [property]/[mrql]/[meta]
// shortcode that then ran with the page's own reach.
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

	marked, items := markItems(itemBranch)

	var b strings.Builder
	for i, elem := range arr {
		if i >= limit {
			break
		}
		values := make([]string, len(items))
		for j, item := range items {
			values[j] = stripNUL(renderItemValue(item, elem, i+1))
		}
		// Once per element, as before: an [mrql] or a plugin shortcode inside
		// the branch is invoked for every element, and the page query budget
		// counts it that way.
		rendered := processWithDepth(reqCtx, marked, ctx, renderer, executor, depth+1)
		b.WriteString(spliceItems(rendered, values))
	}
	return b.String()
}

// markItems replaces every top-level [item …] token in branch with its inert
// sentinel, returning the marked branch and the tokens in sentinel order.
// [item] tokens inside a nested [each] block are not top-level (they live in
// that block's inner content), so they are left untouched to bind to the nearest
// enclosing [each].
func markItems(branch string) (string, []Shortcode) {
	// A branch that already contains NUL could otherwise carry sentinel bytes
	// of its own. Replacing it byte-for-byte keeps every parsed offset valid.
	if strings.IndexByte(branch, 0) >= 0 {
		branch = strings.ReplaceAll(branch, "\x00", " ")
	}

	scs := ParseWithBlocks(branch)
	if len(scs) == 0 {
		return branch, nil
	}

	var items []Shortcode
	var b strings.Builder
	lastEnd := 0
	for _, sc := range scs {
		if sc.Name != "item" {
			continue
		}
		b.WriteString(branch[lastEnd:sc.Start])
		b.WriteString(itemSentinel(len(items)))
		items = append(items, sc)
		lastEnd = sc.End
	}
	b.WriteString(branch[lastEnd:])
	return b.String(), items
}

// spliceItems writes the current element's values over the sentinels in the
// processed branch. One left-to-right pass, and a spliced value is never
// rescanned: a repeated strings.Replace would itself be injectable, since the
// value written for one sentinel could contain another's bytes. A sentinel that
// names no value is dropped rather than printed — NUL is not page content.
func spliceItems(rendered string, values []string) string {
	if len(values) == 0 {
		return rendered
	}

	var b strings.Builder
	b.Grow(len(rendered))
	for {
		at := strings.Index(rendered, itemSentinelPrefix)
		if at < 0 {
			b.WriteString(rendered)
			return b.String()
		}
		b.WriteString(rendered[:at])

		rest := rendered[at+len(itemSentinelPrefix):]
		end := strings.IndexByte(rest, 0)
		if end < 0 {
			// An opener with no terminator is not a sentinel this pass wrote;
			// drop the marker bytes and keep what follows verbatim.
			b.WriteString(rest)
			return b.String()
		}
		if idx, err := strconv.Atoi(rest[:end]); err == nil && idx >= 0 && idx < len(values) {
			b.WriteString(values[idx])
		}
		rendered = rest[end+1:]
	}
}

// stripNUL removes NUL bytes from a rendered item value. A JSON NUL escape
// unmarshals to a real NUL, so a Meta value can carry the byte the sentinels are
// delimited with — and a NUL has no business on a rendered page either way.
func stripNUL(s string) string {
	if strings.IndexByte(s, 0) < 0 {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// renderItemValue renders one [item] occurrence for the current element.
// [item index="true"] renders the 1-based position; otherwise the element (or a
// dot-path into it) is formatted with the same format=/layout=/default= helpers
// as [property]. Output is HTML-escaped unless raw="true", which means exactly
// that and nothing more: the value is text either way, never template source —
// the same contract raw= carries on [property] and [meta inline="true"].
func renderItemValue(sc Shortcode, elem any, index int) string {
	if sc.Attrs["index"] == "true" {
		return strconv.Itoa(index)
	}

	// formatJSONScalar, not formatPropertyValue: an [item] value came out of a
	// Meta blob, where a timestamp is a string and every number is a float64.
	value := navigateJSONValue(elem, sc.Attrs["path"])
	text := formatJSONScalar(value, sc.Attrs["format"], sc.Attrs["layout"])

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
