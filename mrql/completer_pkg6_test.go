package mrql

import "testing"

// TestComplete_OperatorsIncludeRegexAndBetween verifies the new operators are
// offered after a field name.
func TestComplete_OperatorsIncludeRegexAndBetween(t *testing.T) {
	query := "name "
	sugg := Complete(query, len(query))
	for _, want := range []string{"~*", "!~*", "BETWEEN"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected operator %q after field name", want)
		}
	}
}

// TestComplete_OrderByAlwaysRandom verifies RANDOM() is suggested in ORDER BY.
func TestComplete_OrderByAlwaysRandom(t *testing.T) {
	query := `type = "resource" ORDER BY `
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "RANDOM()") {
		t.Errorf("expected RANDOM() in ORDER BY suggestions")
	}
	// RANK is not suggested without a TEXT predicate.
	if hasSuggestion(sugg, "RANK") {
		t.Errorf("did not expect RANK without a TEXT predicate")
	}
}

// TestComplete_OrderByRankWithText verifies RANK is suggested when TEXT ~ present.
func TestComplete_OrderByRankWithText(t *testing.T) {
	query := `type = "note" AND TEXT ~ "x" ORDER BY `
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "RANK") {
		t.Errorf("expected RANK in ORDER BY suggestions when TEXT ~ present")
	}
	if !hasSuggestion(sugg, "RANDOM()") {
		t.Errorf("expected RANDOM() in ORDER BY suggestions")
	}
}
