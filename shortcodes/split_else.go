package shortcodes

import (
	"regexp"
	"sort"
	"strings"
)

const elseTag = "[else]"

// elseifDividerPattern matches an [elseif ...] divider and captures its
// attribute string. The name must end at whitespace or the closing bracket, so
// [elseiffoo] is not a divider. [elseif] is a divider inside conditional inner content,
// never a block opener, so it is deliberately absent from shortcodePattern.
var elseifDividerPattern = regexp.MustCompile(`^\[elseif(?:\s+([^\]]*))?\]`)

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
func blockInnerSpans(content string) [][2]int { return blockInnerSpansM(nil, content) }

// blockInnerSpansM is blockInnerSpans under a meter: the body is re-parsed
// through the same token and attribute admission as the render itself.
func blockInnerSpansM(m *renderMeter, content string) [][2]int {
	blocks := parseWithBlocksM(m, content)
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

// insideSortedSpans is insideSpans for spans that are sorted and disjoint, as
// blockInnerSpans returns them (top-level blocks, in order): a binary search
// instead of a scan, so a divider search over a long body is not spans x bytes.
func insideSortedSpans(i int, spans [][2]int) bool {
	k := sort.Search(len(spans), func(k int) bool { return spans[k][1] > i })
	return k < len(spans) && i >= spans[k][0]
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
func SplitElse(content string) (string, string) { return splitElseM(nil, content) }

func splitElseM(m *renderMeter, content string) (string, string) {
	spans := blockInnerSpansM(m, content)
	for i := 0; i < len(content); i++ {
		if i&0x3ff == 0 && !m.alive() {
			return content, ""
		}
		if content[i] == '[' && strings.HasPrefix(content[i:], elseTag) && !insideSortedSpans(i, spans) {
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
func SplitBranches(content string) []Branch { return splitBranchesM(nil, content) }

// splitBranchesM is SplitBranches under a meter. [elseif] is a divider, not a
// token, so its attribute maps are outside the ordinary token collection: each
// is parsed with the aggregate attribute reservation, and a stop ends the split.
func splitBranchesM(m *renderMeter, content string) []Branch {
	spans := blockInnerSpansM(m, content)

	var branches []Branch
	current := Branch{}
	segStart := 0

	for i := 0; i < len(content); {
		if !m.tick() {
			return branches
		}
		if content[i] != '[' || insideSortedSpans(i, spans) {
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
		if loc := elseifDividerPattern.FindStringSubmatchIndex(content[i:]); loc != nil {
			attrStr := ""
			if loc[2] >= 0 {
				attrStr = content[i+loc[2] : i+loc[3]]
			}
			current.Content = content[segStart:i]
			branches = append(branches, current)
			segStart = i + loc[1]
			attrCap := -1
			if m.limited() {
				attrCap = budgetedAttrCap
			}
			attrs, over := parseAttrsReserve(strings.TrimSpace(attrStr), attrCap, m.reserveAttrs)
			if over {
				m.fail("more than %d attributes on one [elseif], or past the aggregate allowance", attrCap)
				return branches
			}
			current = Branch{Attrs: attrs}
			i = segStart
			continue
		}
		i++
	}

	current.Content = content[segStart:]
	branches = append(branches, current)
	return branches
}

// branchDividerSpans returns the [start,end) offsets of the top-level [else]
// and [elseif …] dividers SplitBranches splits on.
func branchDividerSpans(content string) [][2]int { return branchDividerSpansM(nil, content) }

func branchDividerSpansM(m *renderMeter, content string) [][2]int {
	spans := blockInnerSpansM(m, content)
	var out [][2]int
	for i := 0; i < len(content); {
		if content[i] != '[' || insideSortedSpans(i, spans) {
			i++
			continue
		}
		if strings.HasPrefix(content[i:], elseTag) {
			out = append(out, [2]int{i, i + len(elseTag)})
			i += len(elseTag)
			continue
		}
		if m := elseifDividerPattern.FindStringSubmatchIndex(content[i:]); m != nil {
			out = append(out, [2]int{i, i + m[1]})
			i += m[1]
			continue
		}
		i++
	}
	return out
}
