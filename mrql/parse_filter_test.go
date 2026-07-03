package mrql

import (
	"errors"
	"strings"
	"testing"
)

// parseFilterAndValidate is a helper that runs ParseFilter then Validate, the
// same pipeline application_context.applyMRQLFilter uses.
func parseFilterAndValidate(t *testing.T, entity EntityType, input string) (*Query, error) {
	t.Helper()
	q, err := ParseFilter(entity, input)
	if err != nil {
		return nil, err
	}
	return q, Validate(q)
}

func TestParseFilter_ValidExpressions(t *testing.T) {
	cases := []struct {
		name   string
		entity EntityType
		input  string
	}{
		{"resource tags and date", EntityResource, `tags = "vacation" AND created > -30d`},
		{"resource empty notes and size", EntityResource, `notes IS EMPTY AND fileSize > 10mb`},
		{"resource similar to", EntityResource, `SIMILAR TO resource(1234) AND tags != "reviewed"`},
		{"group descendants", EntityGroup, `descendants.category = "Archive"`},
		{"note simple", EntityNote, `name ~ "todo"`},
		{"resource text search", EntityResource, `TEXT ~ "sunset"`},
		{"resource parenthesised or", EntityResource, `(tags = "a" OR tags = "b") AND fileSize > 1kb`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseFilterAndValidate(t, tc.entity, tc.input)
			if err != nil {
				t.Fatalf("expected valid, got error: %v", err)
			}
			if q.EntityType != tc.entity {
				t.Fatalf("expected entity %v, got %v", tc.entity, q.EntityType)
			}
			if q.Where == nil {
				t.Fatalf("expected a WHERE expression")
			}
		})
	}
}

func TestParseFilter_RejectsClausesAndConstructs(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string // substring the error message must contain
	}{
		{"order by", `tags = "a" ORDER BY name`, "not allowed in a filter expression"},
		{"limit", `tags = "a" LIMIT 10`, "not allowed in a filter expression"},
		{"offset", `tags = "a" LIMIT 5 OFFSET 3`, "not allowed in a filter expression"},
		{"group by", `tags = "a" GROUP BY category`, "not allowed in a filter expression"},
		{"scope", `tags = "a" SCOPE 5`, "not allowed in a filter expression"},
		{"type field eq", `type = "note"`, "type field is implied"},
		{"type field neq", `type != "note"`, "type field is implied"},
		{"param placeholder", `tags = $tag`, "parameter placeholder $tag"},
		{"empty", ``, "empty filter expression"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFilterAndValidate(t, EntityResource, tc.input)
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestParseFilter_ErrorPositionsMatchInput verifies the positioned error points
// at the offending token in the original (un-prefixed) input, so the bar can
// underline it 1:1.
func TestParseFilter_ErrorPositionsMatchInput(t *testing.T) {
	input := `tags = "a" ORDER BY name`
	_, err := ParseFilter(EntityResource, input)
	if err == nil {
		t.Fatalf("expected error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	// "ORDER BY" begins at byte offset 11.
	if pe.Pos != strings.Index(input, "ORDER BY") {
		t.Fatalf("expected pos %d, got %d", strings.Index(input, "ORDER BY"), pe.Pos)
	}
}

// TestParseFilter_TypeFieldPositionMatchesInput checks the type-field rejection
// position points at the `type` token.
func TestParseFilter_TypeFieldPositionMatchesInput(t *testing.T) {
	input := `name ~ "x" AND type = "note"`
	_, err := ParseFilter(EntityResource, input)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ve *ValidationError
	// type-field rejection happens after parse succeeds, so it's a ValidationError.
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}
	if ve.Pos != strings.Index(input, "type") {
		t.Fatalf("expected pos %d, got %d", strings.Index(input, "type"), ve.Pos)
	}
}

// TestParseFilter_SimilarToRejectedOnNonResource verifies that SIMILAR TO in a
// note/group filter is rejected by Validate (entity type is set by ParseFilter).
func TestParseFilter_SimilarToRejectedOnNonResource(t *testing.T) {
	_, err := parseFilterAndValidate(t, EntityNote, `SIMILAR TO resource(5)`)
	if err == nil {
		t.Fatalf("expected SIMILAR TO to be rejected on notes")
	}
	if !strings.Contains(err.Error(), "SIMILAR TO requires type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestParseFilter_NoLimitInTranslatedSubquery verifies the parsed filter query
// carries no LIMIT (Limit == -1), so translation emits no LIMIT and it can serve
// as an all-rows membership predicate (translator.go only emits when Limit >= 0).
func TestParseFilter_NoLimitInTranslatedSubquery(t *testing.T) {
	q, err := parseFilterAndValidate(t, EntityResource, `tags = "vacation"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Limit != -1 {
		t.Fatalf("expected Limit -1 (no limit), got %d", q.Limit)
	}
}
