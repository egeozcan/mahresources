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

// decimalDigits is len(strconv.Itoa(n)) without the allocation.
func decimalDigits(n int) int {
	d := 1
	for n >= 10 {
		n /= 10
		d++
	}
	return d
}

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

	itemBranch, elseBranch := splitElseM(meterFrom(reqCtx), sc.InnerContent)

	renderElse := func() string {
		if strings.TrimSpace(elseBranch) == "" {
			return ""
		}
		return processWithDepth(reqCtx, elseBranch, ctx, renderer, executor, depth+1)
	}

	arr, ok := ctx.rawValueAtPath(sc.Attrs["path"]).([]any)
	if !ok || len(arr) == 0 {
		return renderElse()
	}

	limit := parseIntAttr(sc.Attrs["limit"], defaultEachLimit)

	m := meterFrom(reqCtx)
	marked, items := markItems(m, itemBranch)

	var b strings.Builder
	for i, elem := range arr {
		if i >= limit {
			break
		}
		if !m.alive() {
			return ""
		}
		// Once per element, as before: an [mrql] or a plugin shortcode inside
		// the branch is invoked for every element, and the page query budget
		// counts it that way.
		rendered := processWithDepth(reqCtx, marked, ctx, renderer, executor, depth+1)
		if m.Err() != nil {
			return ""
		}
		// Values are formatted at their sentinel, one at a time, so a budget
		// stop precedes the next formatting instead of following a slice of all.
		spliced, ok := spliceLazy(m, rendered, len(items), func(idx int) string {
			return stripNUL(renderItemValueObserved(ctx, items[idx], elem, i+1))
		})
		if !ok {
			return ""
		}
		b.WriteString(spliced)
	}
	return b.String()
}

// markItems replaces every top-level [item …] token in branch with its inert
// sentinel, returning the marked branch and the tokens in sentinel order.
// [item] tokens inside a nested [each] block are not top-level (they live in
// that block's inner content), so they are left untouched to bind to the nearest
// enclosing [each].
//
// Top-level is a proxy for "belongs to this loop", and it is a conservative one:
// an [item] inside *any* block — a [conditional], say — is also left alone, and
// the processor's own dispatch then renders it empty. That predates the sentinel
// scheme (it was equally true when the branch was substituted before being
// processed) and is deliberately kept, because marking deeper is not a local
// change: [lazy], [details] and [reload] seal their raw body into a signed token
// that a *later* request renders, and a sentinel sealed into one would be
// emitted to the page by a render that has no splice pass to consume it — and,
// the prefix being per-process, could not be consumed even in principle.
// Widening this means classifying every block by whether its body is rendered in
// this pass, which belongs with those handlers rather than here.
func markItems(m *renderMeter, branch string) (string, []Shortcode) {
	// A branch that already contains NUL could otherwise carry sentinel bytes
	// of its own. Replacing it byte-for-byte keeps every parsed offset valid.
	if strings.IndexByte(branch, 0) >= 0 {
		branch = strings.ReplaceAll(branch, "\x00", " ")
	}

	scs := parseWithBlocksM(m, branch)
	if len(scs) == 0 {
		return branch, nil
	}

	// The marked source is longer than the branch (a sentinel is longer than
	// most [item] tokens), so its size is admitted before it is built, and the
	// item records are reserved with it.
	size, count := int64(len(branch)), 0
	for _, sc := range scs {
		if sc.Name == "item" {
			size += int64(len(itemSentinelPrefix)+decimalDigits(count)+1) - int64(sc.End-sc.Start)
			count++
		}
	}
	if m.Err() != nil || !m.admitParsed(size) || !m.chargeScratch(int64(count)*160) {
		return "", nil
	}

	var items []Shortcode
	var b strings.Builder
	lastEnd := 0
	for _, sc := range scs {
		if sc.Name != "item" {
			continue
		}
		if !m.tick() {
			return "", nil
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
	out, _ := spliceLazy(nil, rendered, len(values), func(i int) string { return values[i] })
	return out
}

// spliceLazy is spliceItems with the value produced on demand by value(idx) for
// idx < n. Before each value is produced the meter is consulted, and its bytes
// are reserved before they are appended, so a stop leaves at most one value
// built. ok is false when the meter stopped the render.
func spliceLazy(m *renderMeter, rendered string, n int, value func(idx int) string) (string, bool) {
	if n == 0 {
		return rendered, true
	}

	var b strings.Builder
	b.Grow(len(rendered))
	for {
		at := strings.Index(rendered, itemSentinelPrefix)
		if at < 0 {
			if !m.chargeOut(int64(len(rendered))) {
				return "", false
			}
			b.WriteString(rendered)
			return b.String(), true
		}
		if !m.tick() || !m.chargeOut(int64(at)) {
			return "", false
		}
		b.WriteString(rendered[:at])

		rest := rendered[at+len(itemSentinelPrefix):]
		end := strings.IndexByte(rest, 0)
		if end < 0 {
			// An opener with no terminator is not a sentinel this pass wrote;
			// drop the marker bytes and keep what follows verbatim.
			if !m.chargeOut(int64(len(rest))) {
				return "", false
			}
			b.WriteString(rest)
			return b.String(), true
		}
		if idx, err := strconv.Atoi(rest[:end]); err == nil && idx >= 0 && idx < n {
			v := value(idx)
			if !m.chargeOut(int64(len(v))) {
				return "", false
			}
			b.WriteString(v)
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
	return renderItemValueObserved(MetaShortcodeContext{}, sc, elem, index)
}

// renderItemValueObserved is renderItemValue for a render whose scalar-format
// hook (WithScalarFormatObserver) rides on ctx.
func renderItemValueObserved(ctx MetaShortcodeContext, sc Shortcode, elem any, index int) string {
	if sc.Attrs["index"] == "true" {
		return strconv.Itoa(index)
	}

	// An empty path is the identity here: [item] with no path= renders the
	// element itself. formatJSONScalar, not formatPropertyValue: an [item] value
	// came out of a Meta blob, where a timestamp is a string and every number is
	// a float64.
	value, _ := navigateJSONValue(elem, sc.Attrs["path"])
	text := formatJSONScalarObserved(ctx, value, sc.Attrs["format"], sc.Attrs["layout"])

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
