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
}

// shortcodePattern matches [name ...attrs] where name is "meta", "property", "mrql",
// "conditional", or "plugin:word:word". Plugin name segments allow lowercase letters,
// digits, hyphens, and underscores to match the plugin system's naming conventions.
var shortcodePattern = regexp.MustCompile(
	`\[(meta|property|mrql|conditional|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\s*([^\]]*)\]`,
)

// attrPattern matches key="value", key='value', or key=value pairs.
var attrPattern = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9_-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`,
)

// Parse scans input for shortcode patterns and returns all matches.
// Only the built-in "meta" shortcode and plugin shortcodes with lowercase kebab-case
// names (format "plugin:plugin-name:shortcode-name") are recognized; all other
// bracket expressions are left unchanged.
func Parse(input string) []Shortcode {
	matches := shortcodePattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make([]Shortcode, 0, len(matches))
	for _, m := range matches {
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
	`\[/(meta|property|mrql|conditional|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\]`,
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
}

// ParseWithBlocks scans input for shortcode patterns and returns all top-level
// matches, including block shortcodes ([name]...[/name]). Nested shortcodes
// inside a block's InnerContent are left as raw text — they are not returned.
func ParseWithBlocks(input string) []Shortcode {
	var tokens []token

	for _, m := range shortcodePattern.FindAllStringSubmatchIndex(input, -1) {
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
		attrStr := ""
		if m[4] >= 0 && m[5] >= 0 {
			attrStr = input[m[4]:m[5]]
		}
		tokens = append(tokens, token{
			name:  name,
			attrs: parseAttrs(strings.TrimSpace(attrStr)),
			raw:   input[fullStart:fullEnd],
			start: fullStart,
			end:   fullEnd,
		})
	}

	for _, m := range closingTagPattern.FindAllStringSubmatchIndex(input, -1) {
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
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

	// Phase 2: match pairs inside-out.
	for i := range tokens {
		if !tokens[i].closing {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if tokens[j].matched || tokens[j].closing {
				continue
			}
			if tokens[j].name == tokens[i].name {
				tokens[j].matched = true
				tokens[i].matched = true
				break
			}
		}
	}

	// Phase 3: build top-level results.
	var result []Shortcode
	skipUntil := -1

	for i := range tokens {
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
					Name:  tokens[i].name,
					Attrs: tokens[i].attrs,
					Raw:   tokens[i].raw,
					Start: tokens[i].start,
					End:   tokens[i].end,
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
			})
			skipUntil = tokens[closeIdx].end
		} else {
			result = append(result, Shortcode{
				Name:  tokens[i].name,
				Attrs: tokens[i].attrs,
				Raw:   tokens[i].raw,
				Start: tokens[i].start,
				End:   tokens[i].end,
			})
		}
	}

	return result
}

// parseAttrs extracts key=value pairs from an attribute string.
// Attribute values may be double-quoted, single-quoted, or unquoted.
// When a key appears more than once, the last value wins.
func parseAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	if s == "" {
		return attrs
	}

	// Unescape HTML entities so shortcodes work after markdown processing.
	// Markdown converts " to &quot; which breaks attribute parsing.
	s = html.UnescapeString(s)

	matches := attrPattern.FindAllStringSubmatch(s, -1)
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

	return attrs
}
