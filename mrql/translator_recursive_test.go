package mrql

import (
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// Package 2: Hierarchy Traversal — ancestors. / descendants. recursive roots.
//
// Seed hierarchy (from setupTestDB):
//   Vacation(1) [root, tag "photo", meta.region=europe]
//     ├─ Work(2)     [tag "document"]
//     │    └─ Sub-Work(4)
//     └─ Photos(5)
//   Archive(3) [root, isolated]
//
// Resources: r1 owner=Vacation(1), r3 owner=Work(2); r2, r4 owner-less.
// Notes:     n1 owner=Vacation(1), n2 owner=Work(2).

// runGroupIDs runs an MRQL query for the group entity and returns sorted result IDs.
func runGroupIDs(t *testing.T, db *gorm.DB, input string) []uint {
	t.Helper()
	result := parseAndTranslate(t, input, EntityGroup, db)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error for %q: %v", input, err)
	}
	ids := make([]uint, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	slices.Sort(ids)
	return ids
}

func runResourceIDs(t *testing.T, db *gorm.DB, input string) []uint {
	t.Helper()
	result := parseAndTranslate(t, input, EntityResource, db)
	var rows []testResource
	if err := result.Find(&rows).Error; err != nil {
		t.Fatalf("query error for %q: %v", input, err)
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	slices.Sort(ids)
	return ids
}

func eqIDs(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRecursiveAncestorsGroups(t *testing.T) {
	db := setupTestDB(t)
	cases := []struct {
		name  string
		query string
		want  []uint
	}{
		{"ancestors name Vacation", `type = "group" AND ancestors.name = "Vacation"`, []uint{2, 4, 5}},
		{"ancestors name Work", `type = "group" AND ancestors.name = "Work"`, []uint{4}},
		{"ancestors name Archive (leaf, no children)", `type = "group" AND ancestors.name = "Archive"`, []uint{}},
		{"ancestors tags photo", `type = "group" AND ancestors.tags = "photo"`, []uint{2, 4, 5}},
		{"ancestors tags document", `type = "group" AND ancestors.tags = "document"`, []uint{4}},
		{"ancestors meta.region", `type = "group" AND ancestors.meta.region = "europe"`, []uint{2, 4, 5}},
		{"ancestors id", `type = "group" AND ancestors.id = 2`, []uint{4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runGroupIDs(t, db, tc.query)
			if !eqIDs(got, tc.want) {
				t.Errorf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestRecursiveDescendantsGroups(t *testing.T) {
	db := setupTestDB(t)
	cases := []struct {
		name  string
		query string
		want  []uint
	}{
		{"descendants name Sub-Work", `type = "group" AND descendants.name = "Sub-Work"`, []uint{1, 2}},
		{"descendants name Work", `type = "group" AND descendants.name = "Work"`, []uint{1}},
		{"descendants name Photos", `type = "group" AND descendants.name = "Photos"`, []uint{1}},
		{"descendants name Vacation (root, no ancestors)", `type = "group" AND descendants.name = "Vacation"`, []uint{}},
		{"descendants tags document", `type = "group" AND descendants.tags = "document"`, []uint{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runGroupIDs(t, db, tc.query)
			if !eqIDs(got, tc.want) {
				t.Errorf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestRecursiveAncestorsResources(t *testing.T) {
	db := setupTestDB(t)
	// r1 owner=Vacation(1) — Vacation is not a strict ancestor of itself → no match.
	// r3 owner=Work(2) — Work's ancestor is Vacation → match.
	if got := runResourceIDs(t, db, `type = "resource" AND ancestors.name = "Vacation"`); !eqIDs(got, []uint{3}) {
		t.Errorf("ancestors.name=Vacation: got %v want [3]", got)
	}
	if got := runResourceIDs(t, db, `type = "resource" AND ancestors.tags = "photo"`); !eqIDs(got, []uint{3}) {
		t.Errorf("ancestors.tags=photo: got %v want [3]", got)
	}
	// Nothing sits under Work except Sub-Work (a group); no resource owner is under Work.
	if got := runResourceIDs(t, db, `type = "resource" AND ancestors.name = "Work"`); !eqIDs(got, []uint{}) {
		t.Errorf("ancestors.name=Work: got %v want []", got)
	}
}

func TestRecursiveNegation(t *testing.T) {
	db := setupTestDB(t)

	// "no descendant is named Sub-Work" — excludes strict ancestors of Sub-Work {1,2}.
	if got := runGroupIDs(t, db, `type = "group" AND descendants.name != "Sub-Work"`); !eqIDs(got, []uint{3, 4, 5}) {
		t.Errorf("descendants.name!=Sub-Work: got %v want [3 4 5]", got)
	}

	// "owner has no ancestor named Vacation, OR owner-less".
	// r1 owner=Vacation (not a strict ancestor of itself) → matches.
	// r3 owner=Work → has ancestor Vacation → excluded.
	// r2, r4 owner-less → match.
	if got := runResourceIDs(t, db, `type = "resource" AND ancestors.name != "Vacation"`); !eqIDs(got, []uint{1, 2, 4}) {
		t.Errorf("ancestors.name!=Vacation: got %v want [1 2 4]", got)
	}
}

func TestRecursiveNotComposition(t *testing.T) {
	db := setupTestDB(t)
	// NOT (has ancestor Vacation): ancestors.name=Vacation matches {2,4,5}; NOT → {1,3}.
	if got := runGroupIDs(t, db, `type = "group" AND NOT ancestors.name = "Vacation"`); !eqIDs(got, []uint{1, 3}) {
		t.Errorf("NOT ancestors.name=Vacation: got %v want [1 3]", got)
	}
}

func TestRecursiveNumericMeta(t *testing.T) {
	db := setupTestDB(t)
	// Vacation(1) has meta.priority=3; its descendants match ancestors.meta.priority = 3.
	if got := runGroupIDs(t, db, `type = "group" AND ancestors.meta.priority = 3`); !eqIDs(got, []uint{2, 4, 5}) {
		t.Errorf("ancestors.meta.priority=3: got %v want [2 4 5]", got)
	}
	// No group has meta.priority=99, so no descendants match.
	if got := runGroupIDs(t, db, `type = "group" AND ancestors.meta.priority = 99`); !eqIDs(got, []uint{}) {
		t.Errorf("ancestors.meta.priority=99: got %v want []", got)
	}
}

func TestRecursiveCategoryScalar(t *testing.T) {
	db := setupTestDB(t)
	// Give Vacation(1) a category; its descendants should match ancestors.category = 7.
	cat := uint(7)
	db.Model(&testGroup{}).Where("id = ?", 1).Update("category_id", cat)

	if got := runGroupIDs(t, db, `type = "group" AND ancestors.category = 7`); !eqIDs(got, []uint{2, 4, 5}) {
		t.Errorf("ancestors.category=7: got %v want [2 4 5]", got)
	}
	if got := runGroupIDs(t, db, `type = "group" AND descendants.category = 7`); !eqIDs(got, []uint{}) {
		t.Errorf("descendants.category=7 (Vacation is root): got %v want []", got)
	}
}

func TestRecursiveComposesWithOr(t *testing.T) {
	db := setupTestDB(t)
	// "in Vacation, or anywhere below it": owner=Vacation OR ancestors under Vacation.
	// r1 owner=Vacation → owner match. r3 owner=Work (under Vacation) → ancestors match.
	got := runResourceIDs(t, db, `type = "resource" AND (owner.name = "Vacation" OR ancestors.name = "Vacation")`)
	if !eqIDs(got, []uint{1, 3}) {
		t.Errorf("owner or ancestors Vacation: got %v want [1 3]", got)
	}
}

func TestRecursiveValidationErrors(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		entityType EntityType
	}{
		{"bare ancestors", `ancestors = "x"`, EntityGroup},
		{"ancestors IN", `ancestors.name IN ("a", "b")`, EntityGroup},
		{"ancestors IS EMPTY", `ancestors IS EMPTY`, EntityGroup},
		{"ancestors.name IS NULL", `ancestors.name IS NULL`, EntityGroup},
		{"multi-level chain", `ancestors.parent.name = "x"`, EntityGroup},
		{"unknown leaf", `ancestors.bogus = "x"`, EntityGroup},
		{"relation leaf (children)", `ancestors.children = "x"`, EntityGroup},
		{"relation leaf (resources)", `descendants.resources = "x"`, EntityGroup},
		{"meta without key", `ancestors.meta = "x"`, EntityGroup},
		{"ORDER BY recursive", `type = "group" AND name ~ "a" ORDER BY ancestors.name`, EntityGroup},
		{"cross-entity (no type)", `ancestors.name = "x"`, EntityUnspecified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				return // parse-level rejection is acceptable
			}
			q.EntityType = tc.entityType
			if err := Validate(q); err == nil {
				t.Errorf("query %q: expected validation error, got none", tc.query)
			}
		})
	}
}

func TestRecursiveValidEntities(t *testing.T) {
	// ancestors/descendants are valid roots on resource, note, and group.
	cases := []struct {
		query      string
		entityType EntityType
	}{
		{`type = "resource" AND ancestors.name = "x"`, EntityResource},
		{`type = "note" AND descendants.tags = "x"`, EntityNote},
		{`type = "group" AND ancestors.meta.k = "v"`, EntityGroup},
		{`type = "resource" AND descendants.category = 3`, EntityResource},
	}
	for _, tc := range cases {
		q, err := Parse(tc.query)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.query, err)
		}
		q.EntityType = tc.entityType
		if err := Validate(q); err != nil {
			t.Errorf("query %q: unexpected validation error: %v", tc.query, err)
		}
	}
}

func TestRecursiveCompletion(t *testing.T) {
	// Field position offers the recursive roots on every entity type.
	for _, prefix := range []string{`type = "resource" AND `, `type = "note" AND `, `type = "group" AND `} {
		suggestions := Complete(prefix, len(prefix))
		if !hasSuggestion(suggestions, "ancestors.name") {
			t.Errorf("%q: expected ancestors.name suggestion, got %v", prefix, suggestions)
		}
		if !hasSuggestion(suggestions, "descendants.name") {
			t.Errorf("%q: expected descendants.name suggestion, got %v", prefix, suggestions)
		}
	}

	// After "ancestors." → group leaf fields (name, tags, category, meta.), no chaining.
	q := `type = "resource" AND ancestors.`
	suggestions := Complete(q, len(q))
	for _, want := range []string{"name", "tags", "category", "meta."} {
		if !hasSuggestion(suggestions, want) {
			t.Errorf("after ancestors.: expected %q, got %v", want, suggestions)
		}
	}
	if hasSuggestion(suggestions, "parent") || hasSuggestion(suggestions, "children") {
		t.Errorf("after ancestors.: should not offer further chaining, got %v", suggestions)
	}
}

// TestRecursiveSQLShapes pins the generated SQL for refactor safety.
func TestRecursiveSQLShapes(t *testing.T) {
	db := setupTestDB(t)
	cases := []struct {
		name       string
		query      string
		entityType EntityType
		wantSubstr string
	}{
		{
			name: "group ancestors scalar", query: `ancestors.name = "x"`, entityType: EntityGroup,
			wantSubstr: `groups.id IN (WITH RECURSIVE _mrql_anc`,
		},
		{
			name: "resource ancestors scalar", query: `ancestors.name = "x"`, entityType: EntityResource,
			wantSubstr: `resources.owner_id IN (WITH RECURSIVE _mrql_anc`,
		},
		{
			name: "group descendants scalar", query: `descendants.name = "x"`, entityType: EntityGroup,
			wantSubstr: `groups.id IN (WITH RECURSIVE _mrql_desc`,
		},
		{
			name: "resource ancestors negated adds null clause", query: `ancestors.name != "x"`, entityType: EntityResource,
			wantSubstr: `resources.owner_id IS NULL`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql := dryRunSQL(t, db, tc.query, tc.entityType)
			if !strings.Contains(sql, tc.wantSubstr) {
				t.Errorf("query %q:\n got: %s\n want substring: %s", tc.query, sql, tc.wantSubstr)
			}
		})
	}
}
