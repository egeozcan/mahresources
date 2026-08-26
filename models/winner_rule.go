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
	WinnerCriterionPixelsDesc      = "pixels_desc"
	WinnerCriterionPixelsAsc       = "pixels_asc"
	WinnerCriterionSizeDesc        = "size_desc"
	WinnerCriterionSizeAsc         = "size_asc"
	WinnerCriterionCreatedAsc      = "created_asc"
	WinnerCriterionCreatedDesc     = "created_desc"
	WinnerCriterionUpdatedAsc      = "updated_asc"
	WinnerCriterionUpdatedDesc     = "updated_desc"
	WinnerCriterionNameAsc         = "name_asc"
	WinnerCriterionNameDesc        = "name_desc"
	WinnerCriterionContentTypeAsc  = "content_type_asc"
	WinnerCriterionContentTypeDesc = "content_type_desc"
	WinnerCriterionHasDescription  = "has_description"
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
// when this criterion cannot tell them apart — which includes the case where
// neither carries the attribute at all.
func compareCriterion(criterion string, a, b WinnerCandidate) int {
	switch criterion {
	case WinnerCriterionPixelsDesc:
		return compareUint64(b.pixels(), a.pixels())
	case WinnerCriterionPixelsAsc:
		// A zero pixel count is "this content type has no resolution", not "this
		// one is the smallest". Ascending order would otherwise hand every
		// mixed Cluster to the video file.
		if a.pixels() == 0 || b.pixels() == 0 {
			return 0
		}
		return compareUint64(a.pixels(), b.pixels())
	case WinnerCriterionSizeDesc:
		return compareInt64(b.Resource.FileSize, a.Resource.FileSize)
	case WinnerCriterionSizeAsc:
		if a.Resource.FileSize == 0 || b.Resource.FileSize == 0 {
			return 0
		}
		return compareInt64(a.Resource.FileSize, b.Resource.FileSize)
	case WinnerCriterionCreatedAsc:
		return compareTime(a.Resource.CreatedAt, b.Resource.CreatedAt)
	case WinnerCriterionCreatedDesc:
		return compareTime(b.Resource.CreatedAt, a.Resource.CreatedAt)
	case WinnerCriterionUpdatedAsc:
		return compareTime(a.Resource.UpdatedAt, b.Resource.UpdatedAt)
	case WinnerCriterionUpdatedDesc:
		return compareTime(b.Resource.UpdatedAt, a.Resource.UpdatedAt)
	case WinnerCriterionNameAsc:
		return compareString(a.Resource.Name, b.Resource.Name)
	case WinnerCriterionNameDesc:
		return compareString(b.Resource.Name, a.Resource.Name)
	case WinnerCriterionContentTypeAsc:
		return compareString(a.Resource.ContentType, b.Resource.ContentType)
	case WinnerCriterionContentTypeDesc:
		return compareString(b.Resource.ContentType, a.Resource.ContentType)
	case WinnerCriterionHasDescription:
		return compareBool(strings.TrimSpace(a.Resource.Description) != "", strings.TrimSpace(b.Resource.Description) != "")
	case WinnerCriterionAssociationsDesc:
		return compareInt(b.Associations, a.Associations)
	case WinnerCriterionAssociationsAsc:
		if a.Associations == 0 || b.Associations == 0 {
			return 0
		}
		return compareInt(a.Associations, b.Associations)
	}
	return 0
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

func compareTime(a, b time.Time) int {
	// Both zero means neither is stamped, which is not a reason to prefer one.
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
