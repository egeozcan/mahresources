package shortcodes

import (
	"sort"
	"strings"
)

// tabAttrs are the attributes a [tab] opener reads. [tab] is a slot tag, not a
// token, so it has no BuiltinDocs entry for the generic attribute check.
var tabAttrs = map[string]bool{"label": true, "lazy": true}

const (
	tabsOutsideWarning    = "content outside [tab]…[/tab] inside [tabs] is not rendered"
	tabUnconsumedWarning  = "[tab] is only read directly inside [tabs], not inside a nested block such as [conditional]; here it is shown as text"
	tabStrayCloserProblem = "stray [/tab] closes no tab; [tab]…[/tab] is read only directly inside a [tabs] block"
)

// lintTabs reports what RenderTabsShortcode does with each [tabs] block, read
// with the same splitter (splitTabsM): the error markers it renders, and the
// labels, attributes and text it falls back on or drops. It then reports every
// [tab …] opener and [/tab] closer no [tabs] block consumes, which render as
// literal text.
//
// That last scan skips three places a bracket is not template text: a
// stylesheet (cssMode, or a <style> element), and a shortcode's own tag, where
// "[tab]" is part of an attribute value. It also skips the body of a block
// whose problem is already reported, so one malformed block is one issue.
func lintTabs(w *schemaWork, lm *renderMeter, input string, tokens []token, tabOpeners [][2]int, cssMode bool, add func(start, end int, severity, msg string)) {
	if !strings.Contains(input, "[tab") && !strings.Contains(input, tabCloseTag) {
		return
	}
	reloadSpans := namedBlockSpans(tokens, "reload")
	consumed := map[int]bool{}
	var reported [][2]int

	var stack []token
	for _, tk := range tokens {
		if tk.name != "tabs" {
			continue
		}
		if !tk.closing {
			stack = append(stack, tk)
			continue
		}
		if len(stack) == 0 {
			continue
		}
		open := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		base, inner := open.end, input[open.end:tk.start]

		if insideSpans(open.start, reloadSpans) {
			add(open.start, open.end, SeverityError, tabsReloadFaceProblem)
		}
		// The split copies and re-parses the body, once per nested [tabs].
		if !w.charge(1 + len(inner)/8) {
			return
		}
		split := splitTabsM(lm, inner)
		if lm.Err() != nil {
			return
		}
		for _, t := range split.Tabs {
			consumed[base+t.OpenStart] = true
			consumed[base+t.CloseStart] = true
		}
		if split.Problem != "" {
			add(base+split.ProblemAt[0], base+split.ProblemAt[1], SeverityError, split.Problem)
			reported = append(reported, [2]int{base, tk.start})
			continue
		}
		if len(split.Tabs) == 0 {
			add(open.start, open.end, SeverityError, tabsNoTabsProblem)
			reported = append(reported, [2]int{base, tk.start})
			continue
		}
		for _, o := range split.Outside {
			add(base+o[0], base+o[1], SeverityWarning, tabsOutsideWarning)
		}
		for i, t := range split.Tabs {
			start, end := base+t.OpenStart, base+t.OpenEnd
			if strings.TrimSpace(t.Attrs["label"]) == "" {
				add(start, end, SeverityWarning, `[tab] has no label; it is shown as "`+tabLabel(t, i)+`"`)
			}
			if v, set := t.Attrs["lazy"]; set && v != "true" && v != "false" {
				add(start, end, SeverityWarning, `lazy on [tab] must be "true" or "false"`)
			}
			names := make([]string, 0, len(t.Attrs))
			for name := range t.Attrs {
				if !tabAttrs[name] {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				add(start, end, SeverityWarning, `unknown attribute "`+name+`" on [tab]`)
			}
		}
	}

	if cssMode {
		return
	}
	skip := append(styleElementSpans(input), reported...)
	// Opener tags, sorted and disjoint, for the attribute-value check.
	var tags [][2]int
	for _, tk := range tokens {
		if !tk.closing {
			tags = append(tags, [2]int{tk.start, tk.end})
		}
	}
	literal := func(i int) bool {
		return !consumed[i] && !withinSpans(i, skip) && !insideSortedSpans(i, tags)
	}

	for _, loc := range tabOpeners {
		if !w.step() {
			return
		}
		if literal(loc[0]) {
			add(loc[0], loc[1], SeverityWarning, tabUnconsumedWarning)
		}
	}
	for from := 0; ; {
		i := strings.Index(input[from:], tabCloseTag)
		if i < 0 {
			break
		}
		i += from
		from = i + len(tabCloseTag)
		if !w.step() {
			return
		}
		if literal(i) && !insideSortedSpans(i, tabOpeners) {
			add(i, from, SeverityError, tabStrayCloserProblem)
		}
	}
}
