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
