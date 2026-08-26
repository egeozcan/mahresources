package models

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// The Winner Rule is an ordered list of criteria from this fixed vocabulary.
// Each criterion breaks the ties the one before it left. A criterion with no
// discriminating power over a Cluster falls through to the next rather than
// picking arbitrarily, so a rule mentioning resolution still behaves sensibly for
// content types that have none.
//
// There is deliberately no filesystem modification time here, because there is
// none anywhere in this system. WinnerCriterionUpdatedAsc is when the row was
// last edited, which is a different fact and is labelled as one.
const (
	WinnerCriterionPixelsDesc       = "pixels_desc"
	WinnerCriterionPixelsAsc        = "pixels_asc"
	WinnerCriterionSizeDesc         = "size_desc"
	WinnerCriterionSizeAsc          = "size_asc"
	WinnerCriterionCreatedAsc       = "created_asc"
	WinnerCriterionCreatedDesc      = "created_desc"
	WinnerCriterionUpdatedAsc       = "updated_asc"
	WinnerCriterionUpdatedDesc      = "updated_desc"
	WinnerCriterionNameAsc          = "name_asc"
	WinnerCriterionNameDesc         = "name_desc"
	WinnerCriterionContentTypeAsc   = "content_type_asc"
	WinnerCriterionContentTypeDesc  = "content_type_desc"
	WinnerCriterionHasDescription   = "has_description"
	WinnerCriterionAssociationsDesc = "associations_desc"
	WinnerCriterionAssociationsAsc  = "associations_asc"
)

// UndecidedCriterion is what a Cluster reports when every criterion in the rule
// tied and the Winner fell to lowest id. It is named rather than hidden so the
// reviewer knows the choice was a tiebreaker of last resort and can look closer.
const UndecidedCriterion = "lowest_id"

// DefaultWinnerRule is pixel count, then file size, then oldest.
//
// Resolution first because it is the one criterion where more is unambiguously
// better; file size to break its ties, since two images of one resolution differ
// by compression; and creation time last, because the earliest copy is usually
// the one other things already point at. The curation criteria are available and
// off by default — they answer "which copy did someone work on", which is worth
// warning about (see ReductionCluster.Lossy) but is a poor first cut.
func DefaultWinnerRule() []string {
	return []string{WinnerCriterionPixelsDesc, WinnerCriterionSizeDesc, WinnerCriterionCreatedAsc}
}

// winnerCriteria is the vocabulary, with the label the page shows for each.
var winnerCriteria = map[string]string{
	WinnerCriterionPixelsDesc:       "highest resolution",
	WinnerCriterionPixelsAsc:        "lowest resolution",
	WinnerCriterionSizeDesc:         "largest file",
	WinnerCriterionSizeAsc:          "smallest file",
	WinnerCriterionCreatedAsc:       "created first",
	WinnerCriterionCreatedDesc:      "created last",
	WinnerCriterionUpdatedAsc:       "edited least recently",
	WinnerCriterionUpdatedDesc:      "edited most recently",
	WinnerCriterionNameAsc:          "name first alphabetically",
	WinnerCriterionNameDesc:         "name last alphabetically",
	WinnerCriterionContentTypeAsc:   "content type first alphabetically",
	WinnerCriterionContentTypeDesc:  "content type last alphabetically",
	WinnerCriterionHasDescription:   "has a description",
	WinnerCriterionAssociationsDesc: "most tags, notes and groups",
	WinnerCriterionAssociationsAsc:  "fewest tags, notes and groups",
	UndecidedCriterion:              "no criterion could decide",
}

// WinnerCriterionLabel returns the human label for a criterion token, or the
// token itself for one this build does not know.
func WinnerCriterionLabel(criterion string) string {
	if label, ok := winnerCriteria[criterion]; ok {
		return label
	}
	return criterion
}

// IsWinnerCriterion reports whether a token is in the vocabulary. Anything else
// is refused at the edge rather than silently ignored during clustering, where a
// dropped criterion would change which Resource is deleted.
func IsWinnerCriterion(criterion string) bool {
	_, ok := winnerCriteria[criterion]
	return ok && criterion != UndecidedCriterion
}

// NormalizeWinnerRule drops unknown and duplicate criteria and falls back to the
// default for an empty result.
func NormalizeWinnerRule(rule []string) []string {
	seen := make(map[string]bool, len(rule))
	out := make([]string, 0, len(rule))
	for _, c := range rule {
		if !IsWinnerCriterion(c) || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return DefaultWinnerRule()
	}
	return out
}

// WinnerCandidate is a Resource together with the derived facts the Winner Rule
// needs and the row does not carry. Association counts are one query for the
// whole Cluster rather than a preload per member.
type WinnerCandidate struct {
	Resource     *Resource
	Associations int
}

func (c WinnerCandidate) pixels() uint64 {
	return uint64(c.Resource.Width) * uint64(c.Resource.Height)
}

// compareCriterion returns -1 when a is the better Winner, 1 when b is, and 0
// when this criterion cannot tell them apart.
//
// "Cannot tell them apart" means *both* sides lack the attribute — never one of
// them. That distinction is the difference between a total order and a comparator
// whose answer depends on the order it is asked in, and this comparator sorts the
// list that decides which files are destroyed.
//
// An earlier version returned 0 whenever *either* side was missing a value, to
// stop a video with no resolution winning "lowest resolution". It bought that at
// the cost of transitivity: under pixels_asc then name_asc, A=(100,"a"),
// B=(none,"b") and C=(50,"c") give A<B (by name), B<C (by name) and C<A (by
// pixels), so the Winner depended on which order sort happened to compare them
// in. A missing value now simply loses its criterion, which keeps the video out of
// the lowest-resolution contest — the thing that rule was for — and leaves a
// genuine tie, both sides missing, to fall through as designed.
func compareCriterion(criterion string, a, b WinnerCandidate) int {
	switch criterion {
	case WinnerCriterionPixelsDesc:
		return preferPresent(a.pixels() > 0, b.pixels() > 0, func() int { return compareUint64(b.pixels(), a.pixels()) })
	case WinnerCriterionPixelsAsc:
		return preferPresent(a.pixels() > 0, b.pixels() > 0, func() int { return compareUint64(a.pixels(), b.pixels()) })
	case WinnerCriterionSizeDesc:
		return preferPresent(a.Resource.FileSize > 0, b.Resource.FileSize > 0, func() int {
			return compareInt64(b.Resource.FileSize, a.Resource.FileSize)
		})
	case WinnerCriterionSizeAsc:
		return preferPresent(a.Resource.FileSize > 0, b.Resource.FileSize > 0, func() int {
			return compareInt64(a.Resource.FileSize, b.Resource.FileSize)
		})
	case WinnerCriterionCreatedAsc:
		return preferPresent(!a.Resource.CreatedAt.IsZero(), !b.Resource.CreatedAt.IsZero(), func() int {
			return compareTime(a.Resource.CreatedAt, b.Resource.CreatedAt)
		})
	case WinnerCriterionCreatedDesc:
		return preferPresent(!a.Resource.CreatedAt.IsZero(), !b.Resource.CreatedAt.IsZero(), func() int {
			return compareTime(b.Resource.CreatedAt, a.Resource.CreatedAt)
		})
	case WinnerCriterionUpdatedAsc:
		return preferPresent(!a.Resource.UpdatedAt.IsZero(), !b.Resource.UpdatedAt.IsZero(), func() int {
			return compareTime(a.Resource.UpdatedAt, b.Resource.UpdatedAt)
		})
	case WinnerCriterionUpdatedDesc:
		return preferPresent(!a.Resource.UpdatedAt.IsZero(), !b.Resource.UpdatedAt.IsZero(), func() int {
			return compareTime(b.Resource.UpdatedAt, a.Resource.UpdatedAt)
		})
	case WinnerCriterionNameAsc:
		return preferPresent(a.Resource.Name != "", b.Resource.Name != "", func() int {
			return compareString(a.Resource.Name, b.Resource.Name)
		})
	case WinnerCriterionNameDesc:
		return preferPresent(a.Resource.Name != "", b.Resource.Name != "", func() int {
			return compareString(b.Resource.Name, a.Resource.Name)
		})
	case WinnerCriterionContentTypeAsc:
		return preferPresent(a.Resource.ContentType != "", b.Resource.ContentType != "", func() int {
			return compareString(a.Resource.ContentType, b.Resource.ContentType)
		})
	case WinnerCriterionContentTypeDesc:
		return preferPresent(a.Resource.ContentType != "", b.Resource.ContentType != "", func() int {
			return compareString(b.Resource.ContentType, a.Resource.ContentType)
		})
	case WinnerCriterionHasDescription:
		return compareBool(strings.TrimSpace(a.Resource.Description) != "", strings.TrimSpace(b.Resource.Description) != "")
	case WinnerCriterionAssociationsDesc:
		return preferPresent(a.Associations > 0, b.Associations > 0, func() int {
			return compareInt(b.Associations, a.Associations)
		})
	case WinnerCriterionAssociationsAsc:
		return preferPresent(a.Associations > 0, b.Associations > 0, func() int {
			return compareInt(a.Associations, b.Associations)
		})
	}
	return 0
}

// preferPresent resolves the missing-value cases and defers the rest. A candidate
// carrying the attribute beats one that does not; neither carrying it is a tie.
//
// aHas and bHas are always in the *caller's* orientation — a first, b second —
// even where the comparison it defers to is reversed for a descending criterion.
// Passing them swapped to match the reversal hands the criterion to whichever
// candidate lacks the value, which is the opposite of the rule.
func preferPresent(aHas, bHas bool, both func() int) int {
	switch {
	case aHas && !bHas:
		return -1
	case bHas && !aHas:
		return 1
	case !aHas && !bHas:
		return 0
	}
	return both()
}

func compareUint64(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareTime orders two stamped instants, earliest first. The missing cases are
// the caller's, through preferPresent, because only the caller knows which way
// round its criterion runs.
func compareTime(a, b time.Time) int {
	if a.IsZero() || b.IsZero() {
		return 0
	}
	switch {
	case a.Before(b):
		return -1
	case b.Before(a):
		return 1
	}
	return 0
}

// compareString orders two non-empty strings. The empty cases are the caller's,
// for the same reason compareTime's are.
func compareString(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareBool(a, b bool) int {
	switch {
	case a && !b:
		return -1
	case b && !a:
		return 1
	}
	return 0
}

// CompareByWinnerRule orders two candidates best-first under the whole rule,
// falling through each criterion that cannot discriminate and landing on lowest
// id when none can.
func CompareByWinnerRule(rule []string, a, b WinnerCandidate) int {
	for _, criterion := range rule {
		if cmp := compareCriterion(criterion, a, b); cmp != 0 {
			return cmp
		}
	}
	return compareUint(a.Resource.ID, b.Resource.ID)
}

func compareUint(a, b uint) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// DecidingCriterion reports which criterion actually chose the Winner over the
// runner-up, and the margin by which it did.
//
// The margin is the difference between reading "the rule picked this one" and
// reading "the rule picked this one by four times the resolution" — a Winner
// chosen by a hair's breadth deserves a closer look, and nothing else on the page
// says so. An empty criterion with undecided set means every criterion tied.
func DecidingCriterion(rule []string, winner, runnerUp WinnerCandidate) (criterion string, margin string, undecided bool) {
	for _, c := range rule {
		if compareCriterion(c, winner, runnerUp) < 0 {
			return c, marginFor(c, winner, runnerUp), false
		}
	}
	return UndecidedCriterion, "", true
}

func marginFor(criterion string, winner, runnerUp WinnerCandidate) string {
	switch criterion {
	case WinnerCriterionPixelsDesc, WinnerCriterionPixelsAsc:
		return ratioMargin(float64(winner.pixels()), float64(runnerUp.pixels()), "the pixels")
	case WinnerCriterionSizeDesc, WinnerCriterionSizeAsc:
		return ratioMargin(float64(winner.Resource.FileSize), float64(runnerUp.Resource.FileSize), "the size")
	case WinnerCriterionCreatedAsc:
		return durationMargin(runnerUp.Resource.CreatedAt.Sub(winner.Resource.CreatedAt), "earlier")
	case WinnerCriterionCreatedDesc:
		return durationMargin(winner.Resource.CreatedAt.Sub(runnerUp.Resource.CreatedAt), "later")
	case WinnerCriterionUpdatedAsc:
		return durationMargin(runnerUp.Resource.UpdatedAt.Sub(winner.Resource.UpdatedAt), "earlier")
	case WinnerCriterionUpdatedDesc:
		return durationMargin(winner.Resource.UpdatedAt.Sub(runnerUp.Resource.UpdatedAt), "later")
	case WinnerCriterionAssociationsDesc, WinnerCriterionAssociationsAsc:
		return fmt.Sprintf("%d against %d", winner.Associations, runnerUp.Associations)
	}
	return ""
}

func ratioMargin(winner, runnerUp float64, noun string) string {
	if runnerUp <= 0 || winner <= 0 {
		return ""
	}
	ratio := winner / runnerUp
	if ratio < 1 {
		ratio = 1 / ratio
	}
	if math.Abs(ratio-1) < 0.005 {
		return fmt.Sprintf("a hair's breadth in %s", noun)
	}
	return fmt.Sprintf("%.2gx %s", ratio, noun)
}

func durationMargin(d time.Duration, direction string) string {
	if d <= 0 {
		return ""
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds %s", int(d.Seconds()), direction)
	case d < time.Hour:
		return fmt.Sprintf("%d minutes %s", int(d.Minutes()), direction)
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours %s", int(d.Hours()), direction)
	default:
		return fmt.Sprintf("%d days %s", int(d.Hours()/24), direction)
	}
}
