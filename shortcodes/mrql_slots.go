package shortcodes

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

// mrqlSlots is the decomposition of an [mrql] block body into its optional
// header/footer/empty slots and the per-item template. All four are literal,
// handler-local tags ([header]/[footer]/[else]) — none are registered with the
// global parser, so they only carry meaning inside an [mrql] block (mirroring
// how [else] works inside [conditional]).
type mrqlSlots struct {
	Header    string
	HasHeader bool
	Footer    string
	HasFooter bool
	// Item is the per-item template: the block body with the header/footer spans
	// and the [else] branch removed.
	Item string
	// Else is the empty-state branch (content after a top-level [else]).
	Else    string
	HasElse bool
}

// parseMRQLSlots decomposes an [mrql] block's inner content. Extraction order
// matches the plan: pull the first top-level [header] and [footer] spans out
// first, then split the remainder on a top-level [else]. What is left is the
// per-item template. Occurrences nested inside another block shortcode are
// skipped so a [header] inside a nested [each]/[conditional]/[mrql] is left
// untouched.
func parseMRQLSlots(inner string) mrqlSlots {
	var s mrqlSlots
	s.Header, inner, s.HasHeader = extractFirstSlot(inner, "header")
	s.Footer, inner, s.HasFooter = extractFirstSlot(inner, "footer")

	if countTopLevelElse(inner) > 0 {
		s.Item, s.Else = SplitElse(inner)
		s.HasElse = true
	} else {
		s.Item = inner
	}
	return s
}

// extractFirstSlot removes the first top-level [tag]…[/tag] pair from content,
// returning the slot's inner content, the content with that pair excised, and
// whether a pair was found. Occurrences nested inside another block shortcode
// are skipped, mirroring SplitElse's nested-block handling.
func extractFirstSlot(content, tag string) (slot string, rest string, found bool) {
	openTag := "[" + tag + "]"
	closeTag := "[/" + tag + "]"
	spans := blockInnerSpans(content)

	openIdx := -1
	for i := 0; i+len(openTag) <= len(content); i++ {
		if content[i] == '[' && strings.HasPrefix(content[i:], openTag) && !insideSpans(i, spans) {
			openIdx = i
			break
		}
	}
	if openIdx < 0 {
		return "", content, false
	}

	innerStart := openIdx + len(openTag)
	closeIdx := -1
	for i := innerStart; i+len(closeTag) <= len(content); i++ {
		if content[i] == '[' && strings.HasPrefix(content[i:], closeTag) && !insideSpans(i, spans) {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return "", content, false
	}

	slot = content[innerStart:closeIdx]
	rest = content[:openIdx] + content[closeIdx+len(closeTag):]
	return slot, rest, true
}

// mentionsTotal reports whether any slot references the {total} placeholder.
// Its presence — and only its presence — sets WantTotal on the query, so the
// extra COUNT query never runs for templates that don't ask for a total.
func (s mrqlSlots) mentionsTotal() bool {
	return strings.Contains(s.Header, "{total}") ||
		strings.Contains(s.Footer, "{total}") ||
		strings.Contains(s.Else, "{total}")
}

// resultIsEmpty reports whether a query result carries no rows, per its mode.
func resultIsEmpty(result *QueryResult) bool {
	switch result.Mode {
	case "aggregated":
		return len(result.Rows) == 0
	case "bucketed":
		return len(result.Groups) == 0
	default:
		return len(result.Items) == 0
	}
}

// resultCount returns the number of rendered rows: items (flat), buckets
// (bucketed), or aggregate rows (aggregated). This is the {count} placeholder
// value and, when no true total was computed, the {total} fallback.
func resultCount(result *QueryResult) int {
	switch result.Mode {
	case "aggregated":
		return len(result.Rows)
	case "bucketed":
		return len(result.Groups)
	default:
		return len(result.Items)
	}
}

// viewAllURL builds the "/mrql" link that reproduces the shortcode's result set.
// The executor pre-resolves both fields: an unscoped saved query links by ID
// (/mrql?saved=<id>, preserving the saved-query identity), while an inline query
// — or a scoped saved query, which cannot use the globally-opening ?saved= link
// — links by its effective text with any applied scope already spliced in.
// Returns "" when neither a saved ID nor an effective query is available.
func viewAllURL(result *QueryResult) string {
	if result.SavedID != 0 {
		return "/mrql?saved=" + strconv.FormatUint(uint64(result.SavedID), 10)
	}
	q := result.EffectiveQuery
	if strings.TrimSpace(q) == "" {
		return ""
	}
	return "/mrql?q=" + url.QueryEscape(q)
}

// substitutePlaceholders replaces {count}, {total}, and {link-all} in slot
// content before it is shortcode-processed. {count} is the rendered row count,
// {total} the true total (falling back to {count} when none was computed), and
// {link-all} the bare view-all URL (escaped for HTML attribute use).
func substitutePlaceholders(text string, result *QueryResult) string {
	if !strings.ContainsRune(text, '{') {
		return text
	}
	count := resultCount(result)
	total := strconv.Itoa(count)
	if result.Total != nil {
		total = strconv.FormatInt(*result.Total, 10)
	}
	repl := strings.NewReplacer(
		"{count}", strconv.Itoa(count),
		"{total}", total,
		"{link-all}", html.EscapeString(viewAllURL(result)),
	)
	return repl.Replace(text)
}

// defaultViewAllLinkHTML renders the built-in "View all →" link appended after
// results when link-all="true". Returns "" when no URL can be built.
func defaultViewAllLinkHTML(result *QueryResult) string {
	u := viewAllURL(result)
	if u == "" {
		return ""
	}
	return fmt.Sprintf(
		`<div class="mrql-view-all mt-2 text-right"><a href="%s" class="text-sm text-amber-700 hover:text-amber-900 underline">View all →</a></div>`,
		html.EscapeString(u),
	)
}
