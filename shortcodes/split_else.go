package shortcodes

import (
	"regexp"
	"strings"
)

const elseTag = "[else]"

// elseifDividerPattern matches an [elseif ...] divider and captures its
// attribute string. [elseif] is a divider inside conditional inner content,
// never a block opener, so it is deliberately absent from shortcodePattern.
var elseifDividerPattern = regexp.MustCompile(`^\[elseif\s*([^\]]*)\]`)

// Branch is one arm of a conditional: its guard attributes and the content it
// renders when selected. Branch 0 (the "if" arm) carries nil Attrs and is
// guarded by the opening [conditional] tag's own attributes. An [elseif] arm
// carries its parsed guard Attrs. The [else] arm sets IsElse and matches
// unconditionally.
type Branch struct {
	Attrs   map[string]string
	Content string
	IsElse  bool
}

// blockInnerSpans returns the [start,end) inner-content byte ranges of every
// top-level block shortcode in content. Dividers ([else]/[elseif]) that fall
// inside one of these ranges belong to a nested block and must be skipped.
func blockInnerSpans(content string) [][2]int {
	blocks := ParseWithBlocks(content)
	var spans [][2]int
	for _, b := range blocks {
		if !b.IsBlock {
			continue
		}
		openEnd := strings.Index(content[b.Start:], "]")
		if openEnd < 0 {
			continue
		}
		innerStart := b.Start + openEnd + 1
		innerEnd := b.End - len("[/"+b.Name+"]")
		spans = append(spans, [2]int{innerStart, innerEnd})
	}
	return spans
}

// insideSpans reports whether byte offset i falls within any of the given spans.
func insideSpans(i int, spans [][2]int) bool {
	for _, s := range spans {
		if i >= s[0] && i < s[1] {
			return true
		}
	}
	return false
}

// SplitElse splits content on the first top-level [else] tag.
// [else] tags nested inside block shortcodes are ignored.
// Returns (ifBranch, elseBranch). If no top-level [else] is found, elseBranch is empty.
func SplitElse(content string) (string, string) {
	spans := blockInnerSpans(content)
	for i := 0; i < len(content); i++ {
		if content[i] == '[' && strings.HasPrefix(content[i:], elseTag) && !insideSpans(i, spans) {
			return content[:i], content[i+len(elseTag):]
		}
	}
	return content, ""
}

// SplitBranches splits conditional inner content into ordered branches on
// top-level [elseif …] and [else] dividers, skipping dividers nested inside
// block shortcodes (mirroring SplitElse). The first branch is always the "if"
// arm (nil Attrs); RenderConditionalShortcode guards it with the opening tag's
// attributes. First matching branch renders.
func SplitBranches(content string) []Branch {
	spans := blockInnerSpans(content)

	var branches []Branch
	current := Branch{}
	segStart := 0

	for i := 0; i < len(content); {
		if content[i] != '[' || insideSpans(i, spans) {
			i++
			continue
		}
		if strings.HasPrefix(content[i:], elseTag) {
			current.Content = content[segStart:i]
			branches = append(branches, current)
			segStart = i + len(elseTag)
			current = Branch{IsElse: true}
			i = segStart
			continue
		}
		if m := elseifDividerPattern.FindStringSubmatchIndex(content[i:]); m != nil {
			attrStr := content[i+m[2] : i+m[3]]
			current.Content = content[segStart:i]
			branches = append(branches, current)
			segStart = i + m[1]
			current = Branch{Attrs: parseAttrs(strings.TrimSpace(attrStr))}
			i = segStart
			continue
		}
		i++
	}

	current.Content = content[segStart:]
	branches = append(branches, current)
	return branches
}
