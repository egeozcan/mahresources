package mrql

import (
	"errors"
	"strings"
	"testing"
)

func TestQueryByteLimit(t *testing.T) {
	prefix, suffix := `name = "`, `"`
	at := prefix + strings.Repeat("x", MaxQueryBytes-len(prefix)-len(suffix)) + suffix
	if _, err := Parse(at); err != nil {
		t.Fatalf("query at byte limit rejected: %v", err)
	}
	over := at + " "
	_, err := Parse(over)
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != "query exceeds maximum size of 32768 bytes" || pe.Pos != MaxQueryBytes {
		t.Fatalf("unexpected over-limit error: %#v", err)
	}
}

func TestExpressionDepthLimit(t *testing.T) {
	at := strings.Repeat("NOT ", MaxExpressionDepth) + `name = "x"`
	if _, err := Parse(at); err != nil {
		t.Fatalf("query at depth limit rejected: %v", err)
	}
	over := strings.Repeat("NOT ", MaxExpressionDepth+1) + `name = "x"`
	_, err := Parse(over)
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != "expression nesting exceeds maximum depth of 64" {
		t.Fatalf("unexpected depth error: %#v", err)
	}
}

func TestINListLimit(t *testing.T) {
	values := make([]string, MaxINListValues)
	for i := range values {
		values[i] = "1"
	}
	at := "id IN (" + strings.Join(values, ",") + ")"
	if _, err := ParseFilter(EntityResource, at); err != nil {
		t.Fatalf("IN list at limit rejected: %v", err)
	}
	over := "id IN (" + strings.Join(append(values, "1"), ",") + ")"
	_, err := ParseFilter(EntityResource, over)
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != "IN list exceeds maximum of 500 values" {
		t.Fatalf("unexpected IN limit error: %#v", err)
	}
}

func TestTokenAndCompletionLimits(t *testing.T) {
	parts := make([]string, 513)
	for i := range parts {
		parts[i] = "id = 1"
	}
	_, err := Parse(strings.Join(parts, " AND "))
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != "query exceeds maximum token count of 2048" {
		t.Fatalf("unexpected token limit error: %#v", err)
	}

	overLimitQuery := strings.Repeat("x", MaxQueryBytes+1)
	for _, cursor := range []int{1, len(overLimitQuery)} {
		suggestions := Complete(overLimitQuery, cursor)
		if suggestions == nil || len(suggestions) != 0 {
			t.Fatalf("over-limit completion at cursor %d = %#v, want non-nil empty", cursor, suggestions)
		}
	}
}
