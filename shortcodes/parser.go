// Package shortcodes provides parsing and rendering of shortcode markup embedded in text.
// Shortcodes use a bracket syntax, e.g. [meta path="cooking.time"] or
// [plugin:my-plugin:rating value="5"], and are expanded into HTML by their respective handlers.
package shortcodes

import (
	"html"
	"regexp"
	"sort"
	"strings"
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

// Scan slot openers alongside real openers so bracket text in a quoted [tab]
// label cannot start a token. The slot alternative has no capture groups.
var shortcodeOrTabOpenPattern = regexp.MustCompile(shortcodePattern.String() + "|" + tabOpenPattern.String())

// attrPattern matches key="value", key='value', or key=value pairs.
var attrPattern = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9_-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`,
)

// Parse returns built-in and plugin shortcode openers, including nested ones.
// Slot tags such as [tab] are left for their enclosing block to read.
func Parse(input string) []Shortcode {
	matches := shortcodeOrTabOpenPattern.FindAllStringSubmatchIndex(input, -1)
	var result []Shortcode
	for _, m := range matches {
		if m[2] < 0 {
			continue // [tab] is a slot opener, not a shortcode token.
		}
		if result == nil {
			result = make([]Shortcode, 0, len(matches))
		}
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
		attrStr := ""
		if m[4] >= 0 && m[5] >= 0 {
			attrStr = input[m[4]:m[5]]
		}

		attrs := parseAttrs(strings.TrimSpace(attrStr))

		result = append(result, Shortcode{
			Name:  name,
			Attrs: attrs,
			Raw:   input[fullStart:fullEnd],
			Start: fullStart,
			End:   fullEnd,
		})
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
	var tokens []token

	tokCap := -1
	attrCap := -1
	if m.limited() {
		tokCap, attrCap = budgetedTokenCap+1, budgetedAttrCap
	}

	opens := shortcodeOrTabOpenPattern.FindAllStringSubmatchIndex(input, tokCap)
	closes := closingTagPattern.FindAllStringSubmatchIndex(input, tokCap)
	if tokCap > 0 && len(opens)+len(closes) > budgetedTokenCap {
		m.fail("more than %d shortcode tokens", budgetedTokenCap)
		return nil
	}
	if len(opens)+len(closes) > 0 {
		tokens = make([]token, 0, len(opens)+len(closes))
	}

	cssMode := m != nil && m.cssMode
	var styleSpans [][2]int
	if !cssMode && len(opens) > 0 && strings.Contains(input, "<") {
		styleSpans = styleElementSpans(input)
	}
	var tabOpeners [][2]int
	for _, mm := range opens {
		if !m.tick() {
			return nil
		}
		fullStart, fullEnd := mm[0], mm[1]
		if mm[2] < 0 {
			tabOpeners = append(tabOpeners, [2]int{fullStart, fullEnd})
			continue
		}
		name := input[mm[2]:mm[3]]
		attrStr := ""
		if mm[4] >= 0 && mm[5] >= 0 {
			attrStr = input[mm[4]:mm[5]]
		}
		attrStr = strings.TrimSpace(attrStr)
		inStyle := insideSortedSpans(fullStart, styleSpans)
		if (cssMode || inStyle) && attrStr == "" && fullStart > 0 &&
			cssSelectorChar(input[fullStart-1]) && !strings.HasPrefix(name, "plugin:") {
			continue
		}
		attrs, over := parseAttrsReserve(attrStr, attrCap, m.reserveAttrs)
		if over {
			m.fail("more than %d attributes on one shortcode, or past the aggregate allowance", attrCap)
			return nil
		}
		tokens = append(tokens, token{
			name:    name,
			attrs:   attrs,
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
		if insideSortedSpans(fullStart, tabOpeners) {
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
		tokens[i].matched = true
	}

	return tokens
}

// ParseWithBlocks scans input for shortcode patterns and returns all top-level
// matches, including block shortcodes ([name]...[/name]). Nested shortcodes
// inside a block's InnerContent are left as raw text — they are not returned.
func ParseWithBlocks(input string) []Shortcode { return parseWithBlocksM(nil, input) }

func parseWithBlocksM(m *renderMeter, input string) []Shortcode {
	tokens := matchTokensM(m, input)
	if len(tokens) == 0 {
		return nil
	}

	// Build top-level results.
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
			// Find matching closer by depth tracking
			depth := 0
			closeIdx := -1
			for j := i + 1; j < len(tokens); j++ {
				if !m.tick() {
					return nil
				}
				if !tokens[j].closing && tokens[j].name == tokens[i].name {
					depth++
				} else if tokens[j].closing && tokens[j].name == tokens[i].name {
					if depth == 0 {
						closeIdx = j
						break
					}
					depth--
				}
			}

			if closeIdx < 0 {
				result = append(result, Shortcode{
					Name:    tokens[i].name,
					Attrs:   tokens[i].attrs,
					Raw:     tokens[i].raw,
					Start:   tokens[i].start,
					End:     tokens[i].end,
					inStyle: tokens[i].inStyle,
				})
				continue
			}

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

// parseAttrs extracts key=value pairs from an attribute string.
// Attribute values may be double-quoted, single-quoted, or unquoted.
// When a key appears more than once, the last value wins.
func parseAttrs(s string) map[string]string {
	attrs, _ := parseAttrsLimit(s, -1)
	return attrs
}

// parseAttrsLimit is parseAttrs that stops at limit attributes (limit < 0 means
// no limit) and reports over=true when more were present, so a budgeted render
// does not materialize every match of an attribute-heavy token.
func parseAttrsLimit(s string, limit int) (attrs map[string]string, over bool) {
	return parseAttrsReserve(s, limit, nil)
}

// parseAttrsReserve is parseAttrsLimit that asks reserve(n) for the n entries
// about to be inserted, before the map is filled; false is "over".
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
