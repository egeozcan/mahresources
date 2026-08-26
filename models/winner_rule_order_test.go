package models

import (
	"testing"
	"time"
)

// The comparator has to be a total order: it sorts the list that decides which
// files are destroyed, and a non-transitive one makes that depend on the order
// the sort happens to compare in.
func TestWinnerRuleIsATotalOrder(t *testing.T) {
	rule := []string{WinnerCriterionPixelsAsc, WinnerCriterionNameAsc}
	mk := func(id uint, w, h uint, name string) WinnerCandidate {
		return WinnerCandidate{Resource: &Resource{ID: id, Width: w, Height: h, Name: name}}
	}
	all := []WinnerCandidate{
		mk(1, 10, 10, "a"),
		mk(2, 0, 0, "b"),
		mk(3, 5, 10, "c"),
		mk(4, 0, 0, "a"),
		mk(5, 10, 10, "c"),
	}
	for _, x := range all {
		for _, y := range all {
			for _, z := range all {
				xy := CompareByWinnerRule(rule, x, y)
				yz := CompareByWinnerRule(rule, y, z)
				xz := CompareByWinnerRule(rule, x, z)
				if xy < 0 && yz < 0 && xz >= 0 {
					t.Fatalf("not transitive: %d<%d, %d<%d, but %d !< %d",
						x.Resource.ID, y.Resource.ID, y.Resource.ID, z.Resource.ID, x.Resource.ID, z.Resource.ID)
				}
			}
		}
	}
	for _, x := range all {
		for _, y := range all {
			if x.Resource.ID == y.Resource.ID {
				continue
			}
			if CompareByWinnerRule(rule, x, y) != -CompareByWinnerRule(rule, y, x) {
				t.Fatalf("not antisymmetric for %d vs %d", x.Resource.ID, y.Resource.ID)
			}
		}
	}
}

// The rule a missing value follows: it loses its own criterion rather than tying,
// which is what keeps a video with no resolution out of the lowest-resolution
// contest without costing the comparator its transitivity.
func TestAMissingValueLosesItsCriterion(t *testing.T) {
	withPixels := WinnerCandidate{Resource: &Resource{ID: 1, Width: 100, Height: 100}}
	without := WinnerCandidate{Resource: &Resource{ID: 2}}
	for _, criterion := range []string{WinnerCriterionPixelsAsc, WinnerCriterionPixelsDesc} {
		if got := compareCriterion(criterion, withPixels, without); got != -1 {
			t.Errorf("%s: a Resource with a resolution should beat one without, got %d", criterion, got)
		}
	}
	bothWithout := WinnerCandidate{Resource: &Resource{ID: 3}}
	if got := compareCriterion(WinnerCriterionPixelsAsc, without, bothWithout); got != 0 {
		t.Errorf("neither carrying a resolution is a tie, got %d", got)
	}

	// The descending criteria reverse the comparison, not the missing-value rule.
	// Passing the presence flags swapped to match the reversal hands the criterion
	// to whichever candidate *lacks* the value.
	named := WinnerCandidate{Resource: &Resource{ID: 4, Name: "named"}}
	unnamed := WinnerCandidate{Resource: &Resource{ID: 5}}
	for _, criterion := range []string{WinnerCriterionNameAsc, WinnerCriterionNameDesc} {
		if got := compareCriterion(criterion, named, unnamed); got != -1 {
			t.Errorf("%s: a named Resource should beat an unnamed one, got %d", criterion, got)
		}
		if got := compareCriterion(criterion, unnamed, named); got != 1 {
			t.Errorf("%s: and the other way round, got %d", criterion, got)
		}
	}

	stamped := WinnerCandidate{Resource: &Resource{ID: 6, CreatedAt: time.Unix(1000, 0)}}
	unstamped := WinnerCandidate{Resource: &Resource{ID: 7}}
	for _, criterion := range []string{WinnerCriterionCreatedAsc, WinnerCriterionCreatedDesc} {
		if got := compareCriterion(criterion, stamped, unstamped); got != -1 {
			t.Errorf("%s: a stamped Resource should beat an unstamped one, got %d", criterion, got)
		}
	}
}

// An association count is always known, and zero is an answer. Treating it as a
// missing attribute makes "fewest tags, notes and groups" choose the copy with
// more of them — deleting the one the reviewer asked to keep.
func TestZeroAssociationsIsACountNotAMissingValue(t *testing.T) {
	none := WinnerCandidate{Resource: &Resource{ID: 1}, Associations: 0}
	one := WinnerCandidate{Resource: &Resource{ID: 2}, Associations: 1}

	if got := compareCriterion(WinnerCriterionAssociationsAsc, none, one); got != -1 {
		t.Errorf("fewest associations: the one with none should win, got %d", got)
	}
	if got := compareCriterion(WinnerCriterionAssociationsDesc, one, none); got != -1 {
		t.Errorf("most associations: the one with some should win, got %d", got)
	}
}

// A lead too short to print as a whole second must not print as none.
//
// One bulk upload writes many rows inside a second, so an Identical Cluster —
// where every earlier criterion ties by construction — routinely falls through to
// creation order with a sub-second gap. Rendering that as "by 0 seconds earlier"
// says the opposite of what happened: it reads as no margin at all, on the one
// tier where the margin is the only thing distinguishing the two copies.
// ratioMargin already answers this shape of question with "a hair's breadth".
func TestASubSecondLeadIsNotReportedAsZero(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	mk := func(id uint, created time.Time) WinnerCandidate {
		return WinnerCandidate{Resource: &Resource{ID: id, CreatedAt: created}}
	}
	rule := []string{WinnerCriterionCreatedAsc}

	criterion, margin, undecided := DecidingCriterion(rule, mk(1, base), mk(2, base.Add(300*time.Millisecond)))
	if undecided || criterion != WinnerCriterionCreatedAsc {
		t.Fatalf("expected the creation criterion to decide, got %q undecided=%v", criterion, undecided)
	}
	if margin == "" {
		t.Fatal("a real lead reported no margin at all")
	}
	if margin == "0 seconds earlier" {
		t.Fatalf("a %v lead was reported as %q", 300*time.Millisecond, margin)
	}

	// A lead that does round to whole seconds still reads as seconds.
	_, seconds, _ := DecidingCriterion(rule, mk(1, base), mk(2, base.Add(5*time.Second)))
	if seconds != "5 seconds earlier" {
		t.Fatalf("expected \"5 seconds earlier\", got %q", seconds)
	}

	// And a count of one is singular. Every bucket can produce one.
	for _, c := range []struct {
		gap  time.Duration
		want string
	}{
		{90 * time.Minute, "1 hour earlier"},
		{72 * time.Hour, "3 days earlier"},
		{25 * time.Hour, "25 hours earlier"},
		{61 * time.Second, "1 minute earlier"},
	} {
		if _, got, _ := DecidingCriterion(rule, mk(1, base), mk(2, base.Add(c.gap))); got != c.want {
			t.Errorf("a %v lead read as %q, want %q", c.gap, got, c.want)
		}
	}
}
