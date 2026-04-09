// Package shortcodes provides parsing and rendering of shortcode markup embedded in text.
// Shortcodes use a bracket syntax, e.g. [meta path="cooking.time"] or
// [plugin:my-plugin:rating value="5"], and are expanded into HTML by their respective handlers.
package shortcodes

import (
	"regexp"
	"strings"
)

// Shortcode represents a parsed shortcode occurrence in text.
type Shortcode struct {
	Name  string            // e.g., "meta" or "plugin:my-plugin:rating"
	Attrs map[string]string // e.g., {"path": "cooking.time", "editable": "true"}
	Raw   string            // original matched text including brackets
	Start int               // byte offset in input
	End   int               // byte offset end (exclusive)
}

// shortcodePattern matches [name ...attrs] where name is "meta", "property", "mrql",
// or "plugin:word:word". Plugin name segments allow lowercase letters, digits, hyphens,
// and underscores to match the plugin system's naming conventions.
var shortcodePattern = regexp.MustCompile(
	`\[(meta|property|mrql|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\s*([^\]]*)\]`,
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

// parseAttrs extracts key=value pairs from an attribute string.
// Attribute values may be double-quoted, single-quoted, or unquoted.
// When a key appears more than once, the last value wins.
func parseAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	if s == "" {
		return attrs
	}

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
