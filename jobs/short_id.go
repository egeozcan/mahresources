package jobs

import (
	"strings"
	"unicode"
)

// ShortID is the part of a Job's id that tells it from other Jobs of the same
// title in a page title or a file name: its last eight letters and digits,
// lowercased. A UUIDv7 begins with its acceptance time, which Jobs accepted
// together share; its tail is random. src/components/jobCenter.js shortJobId is
// the same rule.
func ShortID(id string) string {
	var kept []rune
	for _, r := range strings.ToLower(id) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			kept = append(kept, r)
		}
	}
	if len(kept) > 8 {
		kept = kept[len(kept)-8:]
	}
	return string(kept)
}
