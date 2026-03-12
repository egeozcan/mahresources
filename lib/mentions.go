package lib

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// mentionPattern matches @[type:id:name] markers where type is letters,
// id is digits, and name is everything up to the closing bracket (allowing colons).
var mentionPattern = regexp.MustCompile(`@\[([a-zA-Z]+):(\d+):([^\]]+)\]`)

// Mention represents a parsed @-mention marker extracted from text.
type Mention struct {
	Type          string // Entity type, lowercased (e.g. "group", "note", "resource")
	ID            uint   // Entity ID
	Name          string // Display name
	OriginalMatch string // The full original marker text as found in the source
}

// ParseMentions extracts all unique @[type:id:name] markers from text.
// It deduplicates by type+id, keeping the first occurrence.
// Invalid IDs (zero or unparseable) are skipped.
func ParseMentions(text string) []Mention {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []Mention

	for _, match := range matches {
		typ := strings.ToLower(match[1])
		idStr := match[2]
		name := match[3]

		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			continue
		}

		key := fmt.Sprintf("%s:%d", typ, id)
		if seen[key] {
			continue
		}
		seen[key] = true

		result = append(result, Mention{
			Type:          typ,
			ID:            uint(id),
			Name:          name,
			OriginalMatch: match[0],
		})
	}

	return result
}

// htmlTagPattern matches HTML tags for stripping in line-only checks.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// IsMentionOnlyOnLine returns true if the given marker string is the only
// non-whitespace, non-HTML content on its line within the full text.
// This is used to determine whether a mention should render as a standalone
// embed or as an inline link.
// It handles both plain text and HTML-wrapped text (e.g. after markdown processing
// wraps standalone lines in <p> tags).
func IsMentionOnlyOnLine(fullText, marker string) bool {
	lines := strings.Split(fullText, "\n")
	for _, line := range lines {
		if strings.Contains(line, marker) {
			trimmed := strings.TrimSpace(line)
			if trimmed == marker {
				return true
			}
			// After markdown, the line may be wrapped in HTML tags like <p>...</p>
			stripped := strings.TrimSpace(htmlTagPattern.ReplaceAllString(trimmed, ""))
			if stripped == marker {
				return true
			}
		}
	}
	return false
}

// GroupMentionsByType groups mention IDs by their entity type.
// The returned map keys are type strings and values are slices of IDs
// in the order they appear in the input.
func GroupMentionsByType(mentions []Mention) map[string][]uint {
	if len(mentions) == 0 {
		return nil
	}

	result := make(map[string][]uint)
	for _, m := range mentions {
		result[m.Type] = append(result[m.Type], m.ID)
	}
	return result
}
