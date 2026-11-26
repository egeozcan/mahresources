package shortcodes

import (
	"context"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// tabOpenPattern matches a [tab …] opener inside a [tabs] body. The name ends
// at whitespace (a newline included) or the closing bracket, so [tabs], [tabx]
// and [tab-x] are not openers. Quoted attribute values may contain "]".
var tabOpenPattern = regexp.MustCompile(`\[tab(?:\s(?:"[^"]*"|'[^']*'|[^\]"'])*)?\]`)

const tabCloseTag = "[/tab]"

// Messages shared by the renderer's error marker and lint.
const (
	tabsNoTabsProblem     = `[tabs] needs at least one [tab label="…"]…[/tab]`
	tabsUnclosedProblem   = "[tab] is missing its closing [/tab]"
	tabsNestedTabProblem  = "[tab] cannot contain another [tab]; nest a [tabs] block instead"
	tabsReloadFaceProblem = "[tabs] cannot be used inside a [reload] button face"
)

// tabSlot is one top-level [tab …]…[/tab] pair of a [tabs] body. The offsets
// are within that body: the opener is [OpenStart,OpenEnd) and the closer
// [CloseStart,CloseEnd), so Body is body[OpenEnd:CloseStart].
type tabSlot struct {
	Attrs                                    map[string]string
	Body                                     string
	OpenStart, OpenEnd, CloseStart, CloseEnd int
}

// tabsSplit is a [tabs] body read the way the renderer, SlotTagSpans and lint
// all read it.
type tabsSplit struct {
	// Tabs are the complete top-level pairs, in source order. On a Problem they
	// are the pairs completed before it.
	Tabs []tabSlot
	// Outside are the runs of non-whitespace text that sit outside every
	// top-level pair (trimmed). The renderer drops them.
	Outside [][2]int
	// Problem is why the block cannot render, "" when it can; ProblemAt is the
	// [tab …] opener it is about.
	Problem   string
	ProblemAt [2]int
}

// splitTabsM reads the top-level [tab …]…[/tab] pairs of a [tabs] body.
//
// "Top-level" means outside the whole extent of every shortcode the parser
// finds in the body: a nested block's opener, body and closer, and an inline
// shortcode's tag. So a [tab] inside a nested [conditional] or a nested [tabs],
// or inside a quoted attribute value such as [mrql query='[tab]'], is not a
// tab of this block.
func splitTabsM(m *renderMeter, inner string) tabsSplit {
	var out tabsSplit
	var shields [][2]int
	for _, sc := range parseWithBlocksM(m, inner) {
		shields = append(shields, [2]int{sc.Start, sc.End})
	}
	if m.Err() != nil {
		return out
	}

	limit := -1
	attrCap := -1
	if m.limited() {
		limit, attrCap = budgetedTokenCap+1, budgetedAttrCap
	}
	var opens [][]int
	for _, loc := range tabOpenPattern.FindAllStringIndex(inner, limit) {
		if !insideSortedSpans(loc[0], shields) {
			opens = append(opens, loc)
		}
	}
	if limit > 0 && len(opens) > budgetedTokenCap {
		m.fail("more than %d [tab] openers", budgetedTokenCap)
		return out
	}

	// nextClose finds the first top-level [/tab] at or after from.
	nextClose := func(from int) int {
		for from < len(inner) {
			i := strings.Index(inner[from:], tabCloseTag)
			if i < 0 {
				return -1
			}
			i += from
			if !insideSortedSpans(i, shields) {
				return i
			}
			from = i + len(tabCloseTag)
		}
		return -1
	}
	noteOutside := func(start, end int) {
		seg := inner[start:end]
		trimmedStart := len(seg) - len(strings.TrimLeft(seg, " \t\r\n\f"))
		trimmed := strings.TrimSpace(seg)
		if trimmed != "" {
			out.Outside = append(out.Outside, [2]int{start + trimmedStart, start + trimmedStart + len(trimmed)})
		}
	}

	prevEnd := 0
	for k := 0; k < len(opens); k++ {
		if !m.tick() || !m.reserveDividers(1) {
			return out
		}
		openStart, openEnd := opens[k][0], opens[k][1]
		noteOutside(prevEnd, openStart)
		closeStart := nextClose(openEnd)
		if closeStart < 0 {
			out.Problem, out.ProblemAt = tabsUnclosedProblem, [2]int{openStart, openEnd}
			return out
		}
		if k+1 < len(opens) && opens[k+1][0] < closeStart {
			out.Problem, out.ProblemAt = tabsNestedTabProblem, [2]int{openStart, openEnd}
			return out
		}
		// The attribute text sits between "[tab" and the final "]".
		attrs, over := parseAttrsReserve(strings.TrimSpace(inner[openStart+len("[tab"):openEnd-1]), attrCap, m.reserveAttrs)
		if over {
			m.fail("more than %d attributes on one [tab], or past the aggregate allowance", attrCap)
			return out
		}
		closeEnd := closeStart + len(tabCloseTag)
		out.Tabs = append(out.Tabs, tabSlot{
			Attrs:      attrs,
			Body:       inner[openEnd:closeStart],
			OpenStart:  openStart,
			OpenEnd:    openEnd,
			CloseStart: closeStart,
			CloseEnd:   closeEnd,
		})
		prevEnd = closeEnd
	}
	noteOutside(prevEnd, len(inner))
	return out
}

// tabLabel is the visible text and accessible name of the tab at index i: its
// label, or "Tab N" (1-based) when that is blank.
func tabLabel(t tabSlot, i int) string {
	if label := strings.TrimSpace(t.Attrs["label"]); label != "" {
		return label
	}
	return "Tab " + strconv.Itoa(i+1)
}

// RenderTabsShortcode expands a [tabs]…[/tabs] block of [tab label="…"]…[/tab]
// pairs into the WAI-ARIA tabs pattern.
//
// The server emits a progressive-enhancement layout: a hidden tab list, and
// each panel as a named role="group" with a visible (aria-hidden) label, so a
// reader without JavaScript gets every section stacked and named. The
// <tabs-shortcode> element (src/webcomponents/tabsShortcode.js) then shows the
// tab list, wires the IDs and roles, and hides every panel but the selected
// one. The server emits no IDs, because one template renders many times on a
// page (cards, hover cards).
//
// An eager panel renders its body now. A lazy="true" panel is a [lazy] block,
// so on a display page it costs nothing until the panel is first shown.
func RenderTabsShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return shortcodeErrorMarker("tabs", "[tabs] requires a closing [/tabs] tag")
	}
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("tabs", tabsReloadFaceProblem)
	}

	m := meterFrom(reqCtx)
	split := splitTabsM(m, sc.InnerContent)
	if m.Err() != nil {
		return ""
	}
	if split.Problem != "" {
		return shortcodeErrorMarker("tabs", split.Problem)
	}
	if len(split.Tabs) == 0 {
		return shortcodeErrorMarker("tabs", tabsNoTabsProblem)
	}

	var b strings.Builder
	b.WriteString(`<tabs-shortcode class="tabs-shortcode"><div class="tabs-list" role="tablist"`)
	if label := strings.TrimSpace(sc.Attrs["label"]); label != "" {
		b.WriteString(` aria-label="` + html.EscapeString(label) + `"`)
	}
	b.WriteString(` hidden>`)
	for i, t := range split.Tabs {
		selected, tabindex := "false", "-1"
		if i == 0 {
			selected, tabindex = "true", "0"
		}
		b.WriteString(`<button type="button" role="tab" class="tabs-tab" aria-selected="` + selected + `" tabindex="` + tabindex + `">`)
		b.WriteString(html.EscapeString(tabLabel(t, i)))
		b.WriteString(`</button>`)
	}
	b.WriteString(`</div>`)

	for i, t := range split.Tabs {
		label := html.EscapeString(tabLabel(t, i))
		b.WriteString(`<div class="tabs-panel" role="group" aria-label="` + label + `"><p class="tabs-fallback-label" aria-hidden="true">` + label + `</p>`)
		if t.Attrs["lazy"] == "true" {
			b.WriteString(RenderLazyShortcode(reqCtx, Shortcode{Name: "lazy", IsBlock: true, InnerContent: t.Body}, ctx, renderer, executor, depth))
		} else {
			b.WriteString(processWithDepth(reqCtx, t.Body, ctx, renderer, executor, depth+1))
		}
		if m.Err() != nil {
			return ""
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</tabs-shortcode>`)
	return b.String()
}
