package shortcodes

import "strings"

const elseTag = "[else]"

// SplitElse splits content on the first top-level [else] tag.
// [else] tags nested inside block shortcodes are ignored.
// Uses ParseWithBlocks to identify block regions, then finds the first
// [else] that is not inside any block's span.
// Returns (ifBranch, elseBranch). If no top-level [else] is found, elseBranch is empty.
func SplitElse(content string) (string, string) {
	// Find all block spans so we know which byte ranges to skip.
	blocks := ParseWithBlocks(content)

	i := 0
	for i < len(content) {
		if content[i] == '[' && strings.HasPrefix(content[i:], elseTag) {
			// Check this [else] is not inside any block's inner span.
			insideBlock := false
			for _, b := range blocks {
				if !b.IsBlock {
					continue
				}
				// Find where InnerContent starts by locating the first ']' after b.Start.
				openEnd := strings.Index(content[b.Start:], "]")
				if openEnd < 0 {
					continue
				}
				innerStart := b.Start + openEnd + 1
				// InnerContent ends just before the closing tag.
				closingTag := "[/" + b.Name + "]"
				innerEnd := b.End - len(closingTag)
				if i >= innerStart && i < innerEnd {
					insideBlock = true
					break
				}
			}
			if !insideBlock {
				return content[:i], content[i+len(elseTag):]
			}
		}
		i++
	}

	return content, ""
}
