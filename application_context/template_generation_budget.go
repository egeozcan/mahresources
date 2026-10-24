package application_context

import "strings"

// fitSection keeps entries, in order, while the section build makes from
// them stays within limit bytes, measured as the prompt joins it: lines
// joined by newlines. build gets the kept entries and how many were left
// out, so its omission notice and any wording that depends on completeness
// are measured too. An entry that does not fit is left out and later ones
// are still tried. It returns the section, built even when nothing fits, and
// the indexes of the entries kept.
func fitSection(entries []string, limit int, build func(kept []string, omitted int) []string) ([]string, []int) {
	var kept []string
	var indexes []int
	for i, entry := range entries {
		candidate := append(kept[:len(kept):len(kept)], entry)
		if sectionBytes(build(candidate, len(entries)-len(candidate))) <= limit {
			kept = candidate
			indexes = append(indexes, i)
		}
	}
	return build(kept, len(entries)-len(kept)), indexes
}

// sectionBytes is the size of a section's text: its lines joined by newlines.
func sectionBytes(lines []string) int {
	return len(strings.Join(lines, "\n"))
}
