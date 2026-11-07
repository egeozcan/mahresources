package shortcodes

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	mrqlHeaderTag = "header"
	mrqlFooterTag = "footer"
)

// slotTags are the tags each block shortcode's handler reads out of its own
// body. None is registered with the global parser, so each carries meaning only
// inside its parent: [mrql] pulls out [header], [footer] and a top-level
// [else] (parseMRQLSlots); [conditional] splits on [elseif] and [else]
// (SplitBranches); [each] and [using] split on [else] (SplitElse).
var slotTags = map[string][]string{
	"mrql":        {mrqlHeaderTag, mrqlFooterTag, "else"},
	"conditional": {"else", "elseif"},
	"each":        {"else"},
	"using":       {"else"},
}

// SlotTags returns the tags a block shortcode's handler reads out of its body,
// or nil for a block with none. Outside that block the same tags are plain text.
func SlotTags(parent string) []string {
	return append([]string(nil), slotTags[parent]...)
}

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
func parseMRQLSlots(inner string) mrqlSlots { return parseMRQLSlotsM(nil, inner) }

func parseMRQLSlotsM(m *renderMeter, inner string) mrqlSlots {
	var s mrqlSlots
	s.Header, inner, s.HasHeader = extractFirstSlotM(m, inner, mrqlHeaderTag)
	s.Footer, inner, s.HasFooter = extractFirstSlotM(m, inner, mrqlFooterTag)

	if countTopLevelElseM(m, inner) > 0 {
		s.Item, s.Else = splitElseM(m, inner)
		s.HasElse = true
	} else {
		s.Item = inner
	}
	return s
}

// MRQLParentBoundSlots returns the parts of an [mrql] block's inner content
// that render against the including entity rather than each result: the
// [header], [footer] and [else] branch, when present.
func MRQLParentBoundSlots(inner string) []string {
	s := parseMRQLSlots(inner)
	var slots []string
	if s.HasHeader {
		slots = append(slots, s.Header)
	}
	if s.HasFooter {
		slots = append(slots, s.Footer)
	}
	if s.HasElse {
		slots = append(slots, s.Else)
	}
	return slots
}

// extractFirstSlot removes the first top-level [tag]…[/tag] pair from content,
// returning the slot's inner content, the content with that pair excised, and
// whether a pair was found. Occurrences nested inside another block shortcode
// are skipped, mirroring SplitElse's nested-block handling.
func extractFirstSlot(content, tag string) (slot string, rest string, found bool) {
	return extractFirstSlotM(nil, content, tag)
}

func extractFirstSlotM(m *renderMeter, content, tag string) (slot string, rest string, found bool) {
	openIdx, innerStart, closeIdx, closeEnd := firstSlotSpanM(m, content, tag)
	if openIdx < 0 {
		return "", content, false
	}
	return content[innerStart:closeIdx], content[:openIdx] + content[closeEnd:], true
}

// firstSlotSpan locates the pair extractFirstSlot removes: the offsets of its
// opening tag, the start of its content, its closing tag, and the end of that.
// All are -1 when there is no complete top-level pair.
func firstSlotSpan(content, tag string) (openIdx, innerStart, closeIdx, closeEnd int) {
	return firstSlotSpanM(nil, content, tag)
}

func firstSlotSpanM(m *renderMeter, content, tag string) (openIdx, innerStart, closeIdx, closeEnd int) {
	openTag := "[" + tag + "]"
	closeTag := "[/" + tag + "]"
	spans := blockInnerSpansM(m, content)
	if m.Err() != nil {
		return -1, -1, -1, -1
	}

	openIdx = -1
	for i := 0; i+len(openTag) <= len(content); i++ {
		if i&0x3ff == 0 && !m.alive() {
			return -1, -1, -1, -1
		}
		if content[i] == '[' && strings.HasPrefix(content[i:], openTag) && !insideSortedSpans(i, spans) {
			openIdx = i
			break
		}
	}
	if openIdx < 0 {
		return -1, -1, -1, -1
	}

	innerStart = openIdx + len(openTag)
	closeIdx = -1
	for i := innerStart; i+len(closeTag) <= len(content); i++ {
		if i&0x3ff == 0 && !m.alive() {
			return -1, -1, -1, -1
		}
		if content[i] == '[' && strings.HasPrefix(content[i:], closeTag) && !insideSortedSpans(i, spans) {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return -1, -1, -1, -1
	}
	return openIdx, innerStart, closeIdx, closeIdx + len(closeTag)
}

// SlotTagSpans returns, in order, the [start,end) offsets, within inner, of the tags the
// parent block's handler consumes from its body: for [mrql] the first complete
// [header] and [footer] pair and the top-level [else]; for [conditional] every
// top-level [else] and [elseif …] divider; for [each] and [using] the first
// top-level [else]. Any other occurrence of the same tag stays literal text at runtime.
func SlotTagSpans(parent, inner string) [][2]int { return slotTagSpansM(nil, parent, inner) }

func slotTagSpansM(m *renderMeter, parent, inner string) [][2]int {
	switch parent {
	case "mrql":
		return mrqlSlotTagSpansM(m, inner)
	case "conditional":
		return branchDividerSpansM(m, inner)
	case "each", "using":
		if i := firstElseIndexM(m, inner); i >= 0 {
			return [][2]int{{i, i + len(elseTag)}}
		}
	}
	return nil
}

func firstElseIndexM(m *renderMeter, content string) int {
	spans := blockInnerSpansM(m, content)
	if m.Err() != nil {
		return -1
	}
	for i := 0; i < len(content); i++ {
		if i&0x3ff == 0 && !m.alive() {
			return -1
		}
		if content[i] == '[' && strings.HasPrefix(content[i:], elseTag) && !insideSortedSpans(i, spans) {
			return i
		}
	}
	return -1
}

// mrqlSlotTagSpans repeats parseMRQLSlots' extraction order (header, then
// footer from what is left, then [else] from what is left), keeping a map from
// each remainder back to offsets in inner.
func mrqlSlotTagSpansM(m *renderMeter, inner string) [][2]int {
	var out [][2]int
	// origin[i] is the offset in inner of byte i of the current remainder. Its
	// storage (8 bytes a byte) is reserved before it is allocated, as is each
	// copy of it and of the remainder below.
	if !m.chargeScratch(8 * int64(len(inner)+1)) {
		return nil
	}
	origin := make([]int, len(inner)+1)
	for i := range origin {
		origin[i] = i
	}
	rest := inner
	for _, tag := range []string{mrqlHeaderTag, mrqlFooterTag} {
		o, is, ci, ce := firstSlotSpanM(m, rest, tag)
		if o < 0 {
			continue
		}
		if m.Err() != nil || !m.chargeScratch(8*int64(len(origin))+int64(len(rest))) {
			return nil
		}
		out = append(out, [2]int{origin[o], origin[o] + (is - o)}, [2]int{origin[ci], origin[ci] + (ce - ci)})
		rest = rest[:o] + rest[ce:]
		origin = append(append([]int(nil), origin[:o]...), origin[ce:]...)
	}
	if countTopLevelElseM(m, rest) > 0 {
		if i := firstElseIndexM(m, rest); i >= 0 {
			out = append(out, [2]int{origin[i], origin[i] + len(elseTag)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
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
	out, _ := substitutePlaceholdersM(nil, text, result)
	return out
}

// substitutePlaceholdersM is substitutePlaceholders under a render meter. The
// expansion is admitted before it is built: occurrences are counted, the
// expanded size is computed with overflow checks, and that size must fit the
// parsed-source budget (the expanded slot is parsed next), so 36,000 {link-all}
// over a long query is refused instead of allocating about a gigabyte. ok is
// false when the meter stopped the render.
func substitutePlaceholdersM(m *renderMeter, text string, result *QueryResult) (string, bool) {
	if !strings.ContainsRune(text, '{') {
		return text, true
	}
	count := resultCount(result)
	total := strconv.Itoa(count)
	if result.Total != nil {
		total = strconv.FormatInt(*result.Total, 10)
	}
	values := [3][2]string{
		{"{count}", strconv.Itoa(count)},
		{"{total}", total},
		{"{link-all}", html.EscapeString(viewAllURL(result))},
	}

	size := int64(len(text))
	for _, v := range values {
		n := int64(strings.Count(text, v[0]))
		if n == 0 {
			continue
		}
		delta := int64(len(v[1])) - int64(len(v[0]))
		if delta > 0 && n > (math.MaxInt64-size)/delta {
			m.fail("placeholder expansion overflows")
			return "", false
		}
		size += n * delta
	}
	if !m.admitParsed(size) {
		return "", false
	}

	repl := strings.NewReplacer(
		values[0][0], values[0][1],
		values[1][0], values[1][1],
		values[2][0], values[2][1],
	)
	return repl.Replace(text), true
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
