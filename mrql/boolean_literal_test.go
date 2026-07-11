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

func TestSharedBooleanTranslation(t *testing.T) {
	db := setupTestDB(t)
	sql := dryRunSQL(t, db, `shared = true`, EntityNote)
	if !containsSQL(sql, "notes.share_token IS NOT NULL") {
		t.Fatalf("shared=true SQL = %s", sql)
	}
	sql = dryRunSQL(t, db, `shared != true`, EntityNote)
	if !containsSQL(sql, "notes.share_token IS NULL") {
		t.Fatalf("shared!=true SQL = %s", sql)
	}
}

func containsSQL(sql, want string) bool {
	for i := 0; i+len(want) <= len(sql); i++ {
		if sql[i:i+len(want)] == want {
			return true
		}
	}
	return false
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

func TestTypeFieldRejectsBooleanLiteral(t *testing.T) {
	q, err := Parse(`type = true`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected type=true validation error")
	}
}
