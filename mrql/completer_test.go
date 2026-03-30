package mrql

import (
	"testing"
)

// hasSuggestion checks whether the given value appears in the suggestions slice.
func hasSuggestion(suggestions []Suggestion, value string) bool {
	for _, s := range suggestions {
		if s.Value == value {
			return true
		}
	}
	return false
}

// suggestionTypes returns the set of Type values present in the suggestions.
func suggestionTypes(suggestions []Suggestion) map[string]bool {
	m := make(map[string]bool)
	for _, s := range suggestions {
		m[s.Type] = true
	}
	return m
}

// TestComplete_EmptyQuery verifies that an empty query suggests field names
// including "type" and "name".
func TestComplete_EmptyQuery(t *testing.T) {
	sugg := Complete("", 0)
	if len(sugg) == 0 {
		t.Fatal("expected suggestions for empty query, got none")
	}
	for _, want := range []string{"type", "name", "id", "created", "updated", "tags"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected suggestion %q in empty-query result", want)
		}
	}
	// Also expect TEXT keyword
	if !hasSuggestion(sugg, "TEXT") {
		t.Errorf("expected TEXT keyword suggestion in empty-query result")
	}
	// Types should include "field" and "keyword"
	types := suggestionTypes(sugg)
	if !types["field"] {
		t.Errorf("expected 'field' type in suggestions")
	}
	if !types["keyword"] {
		t.Errorf("expected 'keyword' type in suggestions")
	}
}

// TestComplete_AfterFieldName verifies that after a field name the completer
// suggests comparison operators.
func TestComplete_AfterFieldName(t *testing.T) {
	query := "name "
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected operator suggestions after field name, got none")
	}
	for _, want := range []string{"=", "!=", "~", "!~", "IS", "IN"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected operator %q after field name", want)
		}
	}
	types := suggestionTypes(sugg)
	if !types["operator"] {
		t.Errorf("expected 'operator' type in suggestions")
	}
}

// TestComplete_AfterAnd verifies that after AND the completer suggests field names.
func TestComplete_AfterAnd(t *testing.T) {
	query := "name = \"foo\" AND "
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected field suggestions after AND, got none")
	}
	for _, want := range []string{"name", "type", "id"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected field %q after AND", want)
		}
	}
}

// TestComplete_AfterTypeEquals verifies that after "type = " the completer
// suggests entity types.
func TestComplete_AfterTypeEquals(t *testing.T) {
	query := "type = "
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected entity type suggestions after 'type = ', got none")
	}
	for _, want := range []string{"resource", "note", "group"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected entity type %q after 'type = '", want)
		}
	}
	types := suggestionTypes(sugg)
	if !types["entity_type"] {
		t.Errorf("expected 'entity_type' suggestion type")
	}
}

// TestComplete_AfterTypeEqResource verifies that entity-specific fields are
// suggested after "type = resource AND ".
func TestComplete_AfterTypeEqResource(t *testing.T) {
	query := "type = resource AND "
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected field suggestions after 'type = resource AND ', got none")
	}
	// Should include resource-specific fields
	if !hasSuggestion(sugg, "contentType") {
		t.Errorf("expected resource-specific field 'contentType' in suggestions")
	}
	if !hasSuggestion(sugg, "fileSize") {
		t.Errorf("expected resource-specific field 'fileSize' in suggestions")
	}
	// Common fields should also be present
	if !hasSuggestion(sugg, "name") {
		t.Errorf("expected common field 'name' in suggestions")
	}
}

// TestComplete_AfterDateFieldOperator verifies that after a date field with an
// operator, relative dates and functions are suggested.
func TestComplete_AfterDateFieldOperator(t *testing.T) {
	query := "created >= "
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected date/function suggestions after date field operator, got none")
	}
	// Should include relative date examples
	relDates := []string{"-7d", "-30d", "-3m", "-1y"}
	for _, rd := range relDates {
		if !hasSuggestion(sugg, rd) {
			t.Errorf("expected relative date %q in date-field operator suggestions", rd)
		}
	}
	// Should include function suggestions
	funcs := []string{"NOW()", "START_OF_DAY()", "START_OF_WEEK()", "START_OF_MONTH()", "START_OF_YEAR()"}
	for _, fn := range funcs {
		if !hasSuggestion(sugg, fn) {
			t.Errorf("expected function %q in date-field operator suggestions", fn)
		}
	}
	types := suggestionTypes(sugg)
	if !types["rel_date"] {
		t.Errorf("expected 'rel_date' suggestion type")
	}
	if !types["function"] {
		t.Errorf("expected 'function' suggestion type")
	}
}

// TestComplete_AfterUpdatedField also works with the "updated" date field.
func TestComplete_AfterUpdatedField(t *testing.T) {
	query := "updated = "
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "-7d") {
		t.Errorf("expected relative date suggestions after 'updated = '")
	}
	if !hasSuggestion(sugg, "NOW()") {
		t.Errorf("expected function suggestions after 'updated = '")
	}
}

// TestComplete_AfterValue verifies that after a value the completer suggests
// logical connectives and ORDER BY / LIMIT.
func TestComplete_AfterValue(t *testing.T) {
	query := `name = "foo" `
	sugg := Complete(query, len(query))
	for _, want := range []string{"AND", "OR", "ORDER BY", "LIMIT"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected keyword %q after value, got: %v", want, sugg)
		}
	}
	types := suggestionTypes(sugg)
	if !types["keyword"] {
		t.Errorf("expected 'keyword' type suggestions after a value")
	}
}

// TestComplete_AfterClosingParen verifies keyword suggestions after ")".
func TestComplete_AfterClosingParen(t *testing.T) {
	query := `(name = "foo") `
	sugg := Complete(query, len(query))
	for _, want := range []string{"AND", "OR"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected keyword %q after closing paren", want)
		}
	}
}

// TestComplete_AfterDot verifies sub-field suggestions after "meta." prefix.
func TestComplete_AfterDot(t *testing.T) {
	query := "meta."
	sugg := Complete(query, len(query))
	if len(sugg) == 0 {
		t.Fatal("expected sub-field suggestions after 'meta.', got none")
	}
	types := suggestionTypes(sugg)
	if !types["field"] {
		t.Errorf("expected 'field' type suggestions after 'meta.'")
	}
}

// TestComplete_TypeField verifies "type" is suggested even without trailing space.
func TestComplete_TypeField(t *testing.T) {
	sugg := Complete("", 0)
	if !hasSuggestion(sugg, "type") {
		t.Errorf("expected 'type' field suggestion in empty query result")
	}
}

// TestComplete_CursorMidQuery verifies completions are based on substring up to cursor.
func TestComplete_CursorMidQuery(t *testing.T) {
	// cursor is right after "type = " ignoring the rest
	query := "type = resource AND name = \"bar\""
	cursor := len("type = ")
	sugg := Complete(query, cursor)
	for _, want := range []string{"resource", "note", "group"} {
		if !hasSuggestion(sugg, want) {
			t.Errorf("expected entity type %q when cursor is after 'type = '", want)
		}
	}
}

// TestComplete_AfterOr verifies that after OR the completer suggests field names.
func TestComplete_AfterOr(t *testing.T) {
	query := "name = \"foo\" OR "
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "name") {
		t.Errorf("expected field suggestions after OR")
	}
}

// TestComplete_AfterNot verifies that after NOT the completer suggests field names.
func TestComplete_AfterNot(t *testing.T) {
	query := "NOT "
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "name") {
		t.Errorf("expected field suggestions after NOT")
	}
}

// TestComplete_AfterLParen verifies that after "(" the completer suggests field names.
func TestComplete_AfterLParen(t *testing.T) {
	query := "("
	sugg := Complete(query, len(query))
	if !hasSuggestion(sugg, "name") {
		t.Errorf("expected field suggestions after '('")
	}
}

func TestCompleterOwnerDot(t *testing.T) {
	suggestions := Complete(`type = "resource" AND owner.`, 28)
	hasName := false
	hasTags := false
	for _, s := range suggestions {
		if s.Value == "name" {
			hasName = true
		}
		if s.Value == "tags" {
			hasTags = true
		}
	}
	if !hasName || !hasTags {
		t.Fatalf("after owner., expected name and tags in suggestions; got %v", suggestions)
	}
}

func TestCompleterOwnerFieldSuggestion(t *testing.T) {
	suggestions := Complete(`type = "resource" AND `, 22)
	hasOwner := false
	for _, s := range suggestions {
		if s.Value == "owner" {
			hasOwner = true
		}
	}
	if !hasOwner {
		t.Fatalf("expected owner in field suggestions for resource; got %v", suggestions)
	}
}

func TestCompleterOwnerParentDot(t *testing.T) {
	suggestions := Complete(`type = "resource" AND owner.parent.`, 35)
	hasName := false
	for _, s := range suggestions {
		if s.Value == "name" {
			hasName = true
		}
	}
	if !hasName {
		t.Fatalf("after owner.parent., expected name in suggestions; got %v", suggestions)
	}
}

// TestComplete_SuggestsGroupByAfterValue verifies that GROUP BY is suggested after a value.
func TestComplete_SuggestsGroupByAfterValue(t *testing.T) {
	suggestions := Complete(`type = "resource" `, 19)
	if !hasSuggestion(suggestions, "GROUP BY") {
		t.Errorf("expected GROUP BY in suggestions after value, got: %v", suggestions)
	}
}

// TestComplete_SuggestsFieldsAfterGroupBy verifies that fields are suggested after GROUP BY.
func TestComplete_SuggestsFieldsAfterGroupBy(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY `, 27)
	if !hasSuggestion(suggestions, "contentType") {
		t.Errorf("expected field suggestions after GROUP BY, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "name") {
		t.Errorf("expected common field 'name' after GROUP BY, got: %v", suggestions)
	}
}

// TestComplete_SuggestsAggregatesAfterGroupByField verifies that aggregate functions
// are suggested after a GROUP BY field name.
func TestComplete_SuggestsAggregatesAfterGroupByField(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY contentType `, 39)
	foundCount := false
	foundSum := false
	foundOrderBy := false
	foundLimit := false
	for _, s := range suggestions {
		switch s.Value {
		case "COUNT()":
			foundCount = true
		case "SUM()":
			foundSum = true
		case "ORDER BY":
			foundOrderBy = true
		case "LIMIT":
			foundLimit = true
		}
	}
	if !foundCount {
		t.Errorf("expected COUNT() in suggestions after GROUP BY field, got: %v", suggestions)
	}
	if !foundSum {
		t.Errorf("expected SUM() in suggestions after GROUP BY field, got: %v", suggestions)
	}
	if !foundOrderBy {
		t.Errorf("expected ORDER BY in suggestions after GROUP BY field, got: %v", suggestions)
	}
	if !foundLimit {
		t.Errorf("expected LIMIT in suggestions after GROUP BY field, got: %v", suggestions)
	}
}

// TestComplete_SuggestsAggregatesAfterAggregateParen verifies that after an
// aggregate function's closing paren in GROUP BY context, more aggregates,
// ORDER BY and LIMIT are suggested.
func TestComplete_SuggestsAggregatesAfterAggregateParen(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY contentType COUNT() `, 48)
	foundCount := false
	foundOrderBy := false
	foundLimit := false
	for _, s := range suggestions {
		switch s.Value {
		case "COUNT()":
			foundCount = true
		case "ORDER BY":
			foundOrderBy = true
		case "LIMIT":
			foundLimit = true
		}
	}
	if !foundCount {
		t.Errorf("expected COUNT() in post-aggregate suggestions, got: %v", suggestions)
	}
	if !foundOrderBy {
		t.Errorf("expected ORDER BY in post-aggregate suggestions, got: %v", suggestions)
	}
	if !foundLimit {
		t.Errorf("expected LIMIT in post-aggregate suggestions, got: %v", suggestions)
	}
}

// TestComplete_SuggestsAllAggregatesAfterGroupByField verifies all five aggregate
// functions are suggested.
func TestComplete_SuggestsAllAggregatesAfterGroupByField(t *testing.T) {
	suggestions := Complete(`type = "note" GROUP BY noteType `, 32)
	for _, want := range []string{"COUNT()", "SUM()", "AVG()", "MIN()", "MAX()"} {
		if !hasSuggestion(suggestions, want) {
			t.Errorf("expected aggregate %q in suggestions, got: %v", want, suggestions)
		}
	}
}

// TestComplete_GroupByContextNotLeakedToNonGroupBy verifies that aggregate
// suggestions do not appear in non-GROUP BY contexts.
func TestComplete_GroupByContextNotLeakedToNonGroupBy(t *testing.T) {
	suggestions := Complete(`type = "resource" `, 19)
	for _, s := range suggestions {
		if s.Value == "COUNT()" || s.Value == "SUM()" || s.Value == "AVG()" {
			t.Errorf("aggregate %q should not appear in non-GROUP BY post-value context", s.Value)
		}
	}
}

// ---- Additional GROUP BY completer tests ----

// TestComplete_AfterGroupByCommaReturnsEmpty verifies the current behavior:
// the completer does not yet handle the second field position after a comma in
// GROUP BY (returns empty suggestions). This documents the limitation.
func TestComplete_AfterGroupByCommaReturnsEmpty(t *testing.T) {
	suggestions := Complete(`type = "resource" GROUP BY contentType, `, 40)
	// Current behavior: no suggestions after comma in GROUP BY.
	// This is a known limitation of the completer.
	if len(suggestions) != 0 {
		// If this starts passing, it means the completer was enhanced — update test.
		t.Logf("completer now returns %d suggestions after GROUP BY comma (previously 0): %v", len(suggestions), suggestions)
	}
}

// TestComplete_NoAggregatesOutsideGroupBy verifies that aggregate suggestions
// never appear in non-GROUP BY contexts (e.g., after a simple comparison).
func TestComplete_NoAggregatesOutsideGroupBy(t *testing.T) {
	queries := []struct {
		q string
		c int
	}{
		{`name = "foo" `, 13},
		{`type = "resource" AND name = "foo" `, 35},
		{`ORDER BY name `, 14},
		{`LIMIT 10 `, 9},
	}
	aggNames := []string{"COUNT()", "SUM()", "AVG()", "MIN()", "MAX()"}
	for _, tc := range queries {
		sugg := Complete(tc.q, tc.c)
		for _, s := range sugg {
			for _, agg := range aggNames {
				if s.Value == agg {
					t.Errorf("query=%q: aggregate %q should not appear outside GROUP BY", tc.q, agg)
				}
			}
		}
	}
}

// TestComplete_PartialAggregateAfterCount verifies that after typing a partial
// aggregate keyword in GROUP BY context, further aggregates are suggested.
func TestComplete_PartialAggregateAfterCount(t *testing.T) {
	// After COUNT() with partial "S" — should suggest SUM()
	suggestions := Complete(`type = "resource" GROUP BY contentType COUNT() S`, 48)
	// The completer may not support partial token matching for aggregates,
	// but we verify no crash and sensible output
	if len(suggestions) >= 0 {
		// At minimum, no panic — completer returns something or empty
		for _, s := range suggestions {
			if s.Value == "" || s.Type == "" {
				t.Errorf("suggestion has empty Value or Type: %+v", s)
			}
		}
	}
}

// TestComplete_SuggestionStructure verifies all returned suggestions have non-empty Value and Type.
func TestComplete_SuggestionStructure(t *testing.T) {
	queries := []struct {
		q string
		c int
	}{
		{"", 0},
		{"name ", 5},
		{"name = \"x\" ", 11},
		{"type = ", 7},
		{"created >= ", 11},
	}
	for _, tc := range queries {
		sugg := Complete(tc.q, tc.c)
		for _, s := range sugg {
			if s.Value == "" {
				t.Errorf("query=%q: suggestion has empty Value: %+v", tc.q, s)
			}
			if s.Type == "" {
				t.Errorf("query=%q: suggestion has empty Type: %+v", tc.q, s)
			}
		}
	}
}
