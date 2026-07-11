package mrql

import "testing"

func TestBooleanMetadataLiteral(t *testing.T) {
	q, err := Parse(`meta.archived = true`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected comparison, got %T", q.Where)
	}
	lit, ok := cmp.Value.(*BooleanLiteral)
	if !ok || !lit.Value {
		t.Fatalf("expected true BooleanLiteral, got %#v", cmp.Value)
	}
}

func TestQuotedBooleanRemainsString(t *testing.T) {
	q, err := Parse(`meta.archived = "true"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmp := q.Where.(*ComparisonExpr)
	if _, ok := cmp.Value.(*StringLiteral); !ok {
		t.Fatalf("expected StringLiteral, got %T", cmp.Value)
	}
}
