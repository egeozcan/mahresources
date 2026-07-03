package mrql

import "testing"

// TestComplete_SuggestsHavingAfterAggregate verifies HAVING appears after an
// aggregate function in GROUP BY context.
func TestComplete_SuggestsHavingAfterAggregate(t *testing.T) {
	q := `type = "resource" GROUP BY hash COUNT() `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "HAVING") {
		t.Errorf("expected HAVING in suggestions after aggregate, got: %v", suggestions)
	}
}

// TestComplete_SuggestsAggregatesAfterHaving verifies aggregate functions are
// suggested after the HAVING keyword.
func TestComplete_SuggestsAggregatesAfterHaving(t *testing.T) {
	q := `type = "resource" GROUP BY hash COUNT() HAVING `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "COUNT()") {
		t.Errorf("expected COUNT() in suggestions after HAVING, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "SUM(field)") {
		t.Errorf("expected SUM(field) in suggestions after HAVING, got: %v", suggestions)
	}
}

// TestComplete_SuggestsCountAfterRelationDot verifies "count" is suggested
// after a countable relation followed by a dot.
func TestComplete_SuggestsCountAfterRelationDot(t *testing.T) {
	q := `type = "resource" AND tags.`
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "count") {
		t.Errorf("expected count after tags., got: %v", suggestions)
	}

	q = `type = "group" AND children.`
	suggestions = Complete(q, len(q))
	if !hasSuggestion(suggestions, "count") {
		t.Errorf("expected count after children., got: %v", suggestions)
	}
}

// TestComplete_SuggestsCountFieldsInFieldPosition verifies <relation>.count
// entries appear where relation fields are suggested.
func TestComplete_SuggestsCountFieldsInFieldPosition(t *testing.T) {
	q := `type = "resource" AND `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "tags.count") {
		t.Errorf("expected tags.count in field suggestions, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "notes.count") {
		t.Errorf("expected notes.count in field suggestions, got: %v", suggestions)
	}

	q = `type = "group" AND `
	suggestions = Complete(q, len(q))
	if !hasSuggestion(suggestions, "children.count") {
		t.Errorf("expected children.count in group field suggestions, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "resources.count") {
		t.Errorf("expected resources.count in group field suggestions, got: %v", suggestions)
	}
}

// TestComplete_SuggestsDateBucketsInGroupBy verifies created.month and
// siblings appear in GROUP BY field position.
func TestComplete_SuggestsDateBucketsInGroupBy(t *testing.T) {
	q := `type = "note" GROUP BY `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "created.month") {
		t.Errorf("expected created.month in GROUP BY suggestions, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "updated.week") {
		t.Errorf("expected updated.week in GROUP BY suggestions, got: %v", suggestions)
	}
}

// TestComplete_SuggestsBucketSuffixAfterDateDotInGroupBy verifies suffixes are
// suggested after "created." inside GROUP BY.
func TestComplete_SuggestsBucketSuffixAfterDateDotInGroupBy(t *testing.T) {
	q := `type = "note" GROUP BY created.`
	suggestions := Complete(q, len(q))
	for _, want := range []string{"day", "week", "month", "year"} {
		if !hasSuggestion(suggestions, want) {
			t.Errorf("expected %s after created. in GROUP BY, got: %v", want, suggestions)
		}
	}
}

// TestComplete_AggregatedOrderKeysIncludeBucket verifies ORDER BY suggestions
// include the bucket key in aggregated mode.
func TestComplete_AggregatedOrderKeysIncludeBucket(t *testing.T) {
	q := `type = "note" GROUP BY created.month COUNT() ORDER BY `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "created.month") {
		t.Errorf("expected created.month as aggregated ORDER BY key, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "count") {
		t.Errorf("expected count as aggregated ORDER BY key, got: %v", suggestions)
	}
}

// TestComplete_AggregatedOrderKeysExcludeHavingAggregates verifies that
// aggregates appearing only in HAVING are not offered as ORDER BY keys
// (validation rejects them — only the GROUP BY aggregate list is orderable).
func TestComplete_AggregatedOrderKeysExcludeHavingAggregates(t *testing.T) {
	q := `type = "resource" GROUP BY hash COUNT() HAVING SUM(fileSize) > 100 ORDER BY `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "count") {
		t.Errorf("expected count as ORDER BY key, got: %v", suggestions)
	}
	if !hasSuggestion(suggestions, "hash") {
		t.Errorf("expected hash as ORDER BY key, got: %v", suggestions)
	}
	if hasSuggestion(suggestions, "sum_fileSize") {
		t.Errorf("sum_fileSize is HAVING-only and must not be suggested, got: %v", suggestions)
	}
}

// TestComplete_NewRelationFieldsSuggested verifies notes/resources relation
// fields appear via the field catalog.
func TestComplete_NewRelationFieldsSuggested(t *testing.T) {
	q := `type = "resource" AND `
	suggestions := Complete(q, len(q))
	if !hasSuggestion(suggestions, "notes") {
		t.Errorf("expected notes field on resources, got: %v", suggestions)
	}

	q = `type = "group" AND `
	suggestions = Complete(q, len(q))
	if !hasSuggestion(suggestions, "resources") {
		t.Errorf("expected resources field on groups, got: %v", suggestions)
	}
}
