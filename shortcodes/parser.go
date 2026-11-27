// Package shortcodes provides parsing and rendering of shortcode markup embedded in text.
// Shortcodes use a bracket syntax, e.g. [meta path="cooking.time"] or
// [plugin:my-plugin:rating value="5"], and are expanded into HTML by their respective handlers.
package shortcodes

import (
	"html"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Shortcode represents a parsed shortcode occurrence in text.
type Shortcode struct {
	Name         string            // e.g., "meta" or "plugin:my-plugin:rating"
	Attrs        map[string]string // e.g., {"path": "cooking.time", "editable": "true"}
	Raw          string            // original matched text including brackets
	Start        int               // byte offset in input
	End          int               // byte offset end (exclusive)
	InnerContent string            // text between opening and closing tags (block shortcodes only)
	IsBlock      bool              // true when this is a paired [name]...[/name] block
	inStyle      bool              // the opener is inside a stylesheet embedded in markup
}

// shortcodePattern matches [name ...attrs] where name is "meta", "property", "mrql",
// "conditional", or "plugin:word:word". Plugin name segments allow lowercase letters,
// digits, hyphens, and underscores to match the plugin system's naming conventions.
// Quoted attributes may contain brackets, e.g. a pills options JSON array.
// The name ends at whitespace or the closing bracket, so a longer word or a CSS
// value that merely starts with a name ("[similarity]", "max-w-[calc(100%)]")
// stays literal text.
var shortcodePattern = regexp.MustCompile(
	`\[(meta|datetime|pills|property|mrql|conditional|link|each|item|relations|partial|lazy|details|reload|autorefresh|using|similar|calc|calendar|adjacent|inspect|excerpt|action|tabs|pivot|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)(?:\s((?:"[^"]*"|'[^']*'|[^\]])*))?\]`,
)

// Discover possible slot openers alongside real openers. The slot alternative
// has no capture groups and is only opaque in a paired [tabs] body.
var shortcodeOrTabOpenPattern = regexp.MustCompile(shortcodePattern.String() + "|" + tabOpenPattern.String())

// attrPattern matches key="value", key='value', or key=value pairs.
var attrPattern = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9_-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`,
)

// Parse returns built-in and plugin shortcode openers, including nested ones.
// Slot tags such as [tab] are left for their enclosing block to read.
func Parse(input string) []Shortcode {
	var result []Shortcode
	for _, tk := range matchTokens(input) {
		if !tk.closing {
			result = append(result, Shortcode{Name: tk.name, Attrs: tk.attrs, Raw: tk.raw, Start: tk.start, End: tk.end})
		}
	}

	return result
}

// closingTagPattern matches [/name] closing tags.
var closingTagPattern = regexp.MustCompile(
	`\[/(meta|datetime|pills|property|mrql|conditional|link|each|item|relations|partial|lazy|details|reload|autorefresh|using|similar|calc|calendar|adjacent|inspect|excerpt|action|tabs|pivot|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\]`,
)

// token represents a parsed opening or closing tag.
type token struct {
	name    string
	attrs   map[string]string
	raw     string
	start   int
	end     int
	closing bool
	matched bool
	closeAt int // index of the matching closer, for a paired opener only
	inStyle bool
}

// matchTokens scans input for opening and closing shortcode tags, sorts them by
// position, and pairs them inside-out (setting token.matched on both sides of a
// pair). It is the shared front-end for ParseWithBlocks (which builds a shortcode
// tree from the result) and Lint (which reports unmatched/misused tokens).
func matchTokens(input string) []token { return matchTokensM(nil, input) }

// matchTokensM is matchTokens under a render meter (nil: unbounded). A budgeted
// meter caps the retained tokens and the attributes of one token, and every
// token loop consults the context.
func matchTokensM(m *renderMeter, input string) []token {
	tokens, _ := scanTokensM(m, input, false)
	return tokens
}

type tagScan struct {
	tabCandidates [][2]int // possible literal [tab] text for lint
	tabOpeners    [][2]int // actual slot opener spans, opaque to shortcode parsing
	styles        [][2]int
}

// A tab opener is local syntax. Discover scope with its label provisionally
// opaque (even a quoted [/tabs] is just text), then rescan any source where
// a candidate turned out to be literal. Splitters call this on an already
// paired [tabs] body, so top-level slots there also have an owning block.
func scanTokensM(m *renderMeter, input string, tabsBody bool) ([]token, tagScan) {
	var scan tagScan
	tokCap, attrCap := -1, -1
	if m.limited() {
		tokCap, attrCap = budgetedTokenCap+1, budgetedAttrCap
	}
	opens := shortcodeOrTabOpenPattern.FindAllStringSubmatchIndex(input, tokCap)
	closes := closingTagPattern.FindAllStringSubmatchIndex(input, tokCap)
	withinCap := func(n int) bool {
		if tokCap > 0 && n > budgetedTokenCap {
			m.fail("more than %d shortcode tokens", budgetedTokenCap)
			return false
		}
		return true
	}
	if !withinCap(len(opens) + len(closes)) {
		return nil, scan
	}
	for _, mm := range opens {
		if !m.tick() {
			return nil, scan
		}
		if mm[2] < 0 {
			scan.tabCandidates = append(scan.tabCandidates, [2]int{mm[0], mm[1]})
		}
	}

	cssMode := m != nil && m.cssMode
	if !cssMode {
		scan.styles = shortcodeStyleSpans(input, opens)
	}
	tokens := pairTokensM(m, input, opens, closes, scan.tabCandidates, scan.styles, cssMode)
	if len(scan.tabCandidates) > 0 {
		scan.tabOpeners = scopedTabOpeners(m, tokens, scan.tabCandidates, tabsBody)
		if m.Err() != nil {
			return nil, scan
		}
		if len(scan.tabOpeners) != len(scan.tabCandidates) {
			masked := []byte(input)
			for _, span := range scan.tabOpeners {
				blankTagSpan(masked, span[0], span[1])
			}
			source := string(masked)
			opens = shortcodePattern.FindAllStringSubmatchIndex(source, tokCap)
			if !withinCap(len(opens) + len(closes) + len(scan.tabOpeners)) {
				return nil, scan
			}
			if !cssMode {
				scan.styles = shortcodeStyleSpans(source, opens)
			}
			tokens = pairTokensM(m, input, opens, closes, scan.tabOpeners, scan.styles, cssMode)
		}
	}
	for i := range tokens {
		if !m.tick() {
			return nil, scan
		}
		tk := &tokens[i]
		if tk.closing {
			continue
		}
		attrStr := strings.TrimSpace(tk.raw[len(tk.name)+1 : len(tk.raw)-1])
		attrs, over := parseAttrsReserve(attrStr, attrCap, m.reserveAttrs)
		if over {
			m.fail("more than %d attributes on one shortcode, or past the aggregate allowance", attrCap)
			return nil, scan
		}
		tk.attrs = attrs
	}
	return tokens, scan
}

func scopedTabOpeners(m *renderMeter, tokens []token, candidates [][2]int, tabsBody bool) [][2]int {
	var slots [][2]int
	var blocks []int // opener indices, innermost last
	next := 0
	leaveBlocks := func(pos int) {
		for len(blocks) > 0 && tokens[tokens[blocks[len(blocks)-1]].closeAt].start <= pos {
			blocks = blocks[:len(blocks)-1]
		}
	}
	for _, span := range candidates {
		if !m.tick() {
			return nil
		}
		for ; next < len(tokens) && tokens[next].start < span[0]; next++ {
			if !m.tick() {
				return nil
			}
			tk := tokens[next]
			leaveBlocks(tk.start)
			// A block that crosses its parent's closing tag isn't paired when
			// that parent's body is later rendered on its own.
			if !tk.closing && tk.matched && (len(blocks) == 0 || tk.closeAt < tokens[blocks[len(blocks)-1]].closeAt) {
				blocks = append(blocks, next)
			}
		}
		leaveBlocks(span[0])
		if len(blocks) == 0 && tabsBody || len(blocks) > 0 && tokens[blocks[len(blocks)-1]].name == "tabs" {
			slots = append(slots, span)
		}
	}
	return slots
}

// Mask complete openers before reading HTML: <style> inside a quoted shortcode
// attribute is text, not the start of a stylesheet.
func shortcodeStyleSpans(input string, opens [][]int) [][2]int {
	if !styleStartPattern.MatchString(input) {
		return nil
	}
	masked := []byte(input)
	for _, mm := range opens {
		blankTagSpan(masked, mm[0], mm[1])
	}
	return styleElementSpans(string(masked))
}

func cssLiteralOpener(input string, start, nameEnd int) bool {
	return start > 0 && input[start+1] != '/' && cssSelectorChar(input[start-1]) &&
		!strings.HasPrefix(input[start+1:nameEnd], "plugin:") && strings.HasPrefix(strings.TrimLeftFunc(input[nameEnd:], unicode.IsSpace), "]")
}

func pairTokensM(m *renderMeter, input string, opens, closes [][]int, tabOpeners, styleSpans [][2]int, cssMode bool) []token {
	if len(opens)+len(closes) == 0 {
		return nil
	}
	tokens := make([]token, 0, len(opens)+len(closes))
	// A closer quoted inside an opener's attributes ([tabs label="[/tabs]"])
	// is attribute text, not a tag. Opener matches never overlap and arrive in
	// source order, so the spans stay sorted.
	openSpans := make([][2]int, 0, len(opens))
	for _, mm := range opens {
		if !m.tick() {
			return nil
		}
		if mm[2] < 0 {
			continue // A slot is never itself a global token.
		}
		fullStart, fullEnd := mm[0], mm[1]
		openSpans = append(openSpans, [2]int{fullStart, fullEnd})
		inStyle := insideSortedSpans(fullStart, styleSpans)
		if (cssMode || inStyle) && cssLiteralOpener(input, fullStart, mm[3]) {
			continue
		}
		tokens = append(tokens, token{
			name:    input[mm[2]:mm[3]],
			raw:     input[fullStart:fullEnd],
			start:   fullStart,
			end:     fullEnd,
			inStyle: inStyle,
		})
	}

	for _, mm := range closes {
		if !m.tick() {
			return nil
		}
		fullStart, fullEnd := mm[0], mm[1]
		if insideSortedSpans(fullStart, tabOpeners) || insideSortedSpans(fullStart, openSpans) {
			continue
		}
		name := input[mm[2]:mm[3]]
		tokens = append(tokens, token{
			name:    name,
			start:   fullStart,
			end:     fullEnd,
			raw:     input[fullStart:fullEnd],
			closing: true,
		})
	}

	if len(tokens) == 0 {
		return nil
	}

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].start != tokens[j].start {
			return tokens[i].start < tokens[j].start
		}
		if tokens[i].closing != tokens[j].closing {
			return !tokens[i].closing
		}
		return false
	})

	// Pair inside-out. A closer takes the nearest earlier unmatched opener of
	// its name, which is the top of that name's stack of unmatched openers; the
	// stack replaces a backward scan that was quadratic on nested same-name tags.
	stacks := map[string][]int{}
	for i := range tokens {
		if !m.tick() {
			return nil
		}
		if !tokens[i].closing {
			stacks[tokens[i].name] = append(stacks[tokens[i].name], i)
			continue
		}
		st := stacks[tokens[i].name]
		if len(st) == 0 {
			continue
		}
		j := st[len(st)-1]
		stacks[tokens[i].name] = st[:len(st)-1]
		tokens[j].matched = true
		tokens[j].closeAt = i
		tokens[i].matched = true
	}

	return tokens
}

// ParseWithBlocks scans input for shortcode patterns and returns all top-level
// matches, including block shortcodes ([name]...[/name]). Nested shortcodes
// inside a block's InnerContent are left as raw text — they are not returned.
func ParseWithBlocks(input string) []Shortcode { return parseWithBlocksM(nil, input) }

func parseWithBlocksM(m *renderMeter, input string) []Shortcode {
	return topLevelShortcodes(m, input, matchTokensM(m, input))
}

func topLevelShortcodes(m *renderMeter, input string, tokens []token) []Shortcode {
	var result []Shortcode
	skipUntil := -1

	for i := range tokens {
		if !m.tick() {
			return nil
		}
		if tokens[i].start < skipUntil {
			continue
		}

		if tokens[i].closing {
			continue
		}

		if tokens[i].matched {
			closeIdx := tokens[i].closeAt

			innerStart := tokens[i].end
			innerEnd := tokens[closeIdx].start
			inner := ""
			if innerEnd > innerStart {
				inner = input[innerStart:innerEnd]
			}

			result = append(result, Shortcode{
				Name:         tokens[i].name,
				Attrs:        tokens[i].attrs,
				Raw:          input[tokens[i].start:tokens[closeIdx].end],
				Start:        tokens[i].start,
				End:          tokens[closeIdx].end,
				InnerContent: inner,
				IsBlock:      true,
				inStyle:      tokens[i].inStyle,
			})
			skipUntil = tokens[closeIdx].end
		} else {
			result = append(result, Shortcode{
				Name:    tokens[i].name,
				Attrs:   tokens[i].attrs,
				Raw:     tokens[i].raw,
				Start:   tokens[i].start,
				End:     tokens[i].end,
				inStyle: tokens[i].inStyle,
			})
		}
	}

	return result
}

// parseAttrsReserve extracts key=value pairs from an attribute string. Values
// may be double-quoted, single-quoted, or unquoted; when a key repeats, the
// last value wins. It stops at limit attributes (limit < 0 means no limit) and
// reports over=true when more were present, so a budgeted render does not
// materialize every match of an attribute-heavy token. It asks reserve(n), when
// set, for the n entries about to be inserted, before the map is filled; false
// is "over".
func parseAttrsReserve(s string, limit int, reserve func(n int) bool) (attrs map[string]string, over bool) {
	attrs = make(map[string]string)
	if s == "" {
		return attrs, false
	}

	// Unescape HTML entities so shortcodes work after markdown processing.
	// Markdown converts " to &quot; which breaks attribute parsing.
	s = html.UnescapeString(s)

	n := -1
	if limit >= 0 {
		n = limit + 1
	}
	matches := attrPattern.FindAllStringSubmatch(s, n)
	if limit >= 0 && len(matches) > limit {
		return nil, true
	}
	if reserve != nil && !reserve(len(matches)) {
		return nil, true
	}
	for _, m := range matches {
		key := m[1]
		val := m[2]
		if val == "" {
			val = m[3]
		}
		if val == "" {
			val = m[4]
		}
		attrs[key] = val
	}

	return attrs, false
}
