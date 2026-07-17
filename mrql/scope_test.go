package mrql

import (
	"testing"
)

func TestResolveScopeNil(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for nil scope, got %d", id)
	}
}

func TestResolveScopeNumericExists(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 1},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeNumericNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 9999},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for nonexistent group ID")
	}
}

func TestResolveScopeStringExists(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Vacation"},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "vacation"},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Errorf("expected 1, got %d", id)
	}
}

func TestResolveScopeStringNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Nonexistent"},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for nonexistent group name")
	}
}

func TestResolveScopeAmbiguousName(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&testGroup{ID: 100, Name: "vacation", Meta: `{}`})
	q := &Query{
		Scope: &ScopeClause{
			Value: &StringLiteral{Value: "Vacation"},
		},
	}
	_, err := ResolveScope(q, db)
	if err == nil {
		t.Fatal("expected error for ambiguous group name")
	}
}

func TestResolveScopeWithinSubtree(t *testing.T) {
	db := setupTestDB(t)
	inside := &Query{Scope: &ScopeClause{Value: &StringLiteral{Value: "Work"}}}
	id, err := ResolveScopeWithin(inside, db, 1)
	if err != nil || id != 2 {
		t.Fatalf("resolve in-scope group = %d, %v; want 2", id, err)
	}

	for _, outside := range []Node{
		&NumberLiteral{Value: 3},
		&StringLiteral{Value: "Archive"},
	} {
		q := &Query{Scope: &ScopeClause{Value: outside}}
		if _, err := ResolveScopeWithin(q, db, 1); err == nil {
			t.Fatalf("expected out-of-scope value %#v to be hidden", outside)
		}
	}
	allowed, err := ScopeContains(db, 1, 4)
	if err != nil || !allowed {
		t.Fatalf("nested group should be in scope: allowed=%v err=%v", allowed, err)
	}
	allowed, err = ScopeContains(db, 1, 3)
	if err != nil || allowed {
		t.Fatalf("sibling root should be out of scope: allowed=%v err=%v", allowed, err)
	}
}

func TestResolveScopeZero(t *testing.T) {
	db := setupTestDB(t)
	q := &Query{
		Scope: &ScopeClause{
			Value: &NumberLiteral{Value: 0},
		},
	}
	id, err := ResolveScope(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for SCOPE 0, got %d", id)
	}
}

func TestApplyScopeCTEResourceSubtree(t *testing.T) {
	db := setupTestDB(t)
	result := db.Table("resources")
	result = ApplyScopeCTE(result, EntityResource, 1)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Errorf("expected 2, got %d", len(resources))
	}
}

func TestApplyScopeCTEGroupInclusive(t *testing.T) {
	db := setupTestDB(t)
	result := db.Table("groups")
	result = ApplyScopeCTE(result, EntityGroup, 1)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 4 {
		t.Errorf("expected 4, got %d", len(groups))
	}
}

func TestApplyScopeCTENonexistentIDEmpty(t *testing.T) {
	db := setupTestDB(t)
	result := db.Table("resources")
	result = ApplyScopeCTE(result, EntityResource, 99999)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 for nonexistent scope, got %d", len(resources))
	}
}
