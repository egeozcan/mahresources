//go:build postgres

package mrql

import (
	"slices"
	"testing"

	"gorm.io/gorm"
)

// Package 2 hierarchy traversal against real Postgres. The recursive CTE and
// WITH-inside-subquery must behave identically to SQLite. Seed hierarchy matches
// setupPostgresTestDB: Vacation(1) → Work(2) → Sub-Work(4); Vacation(1) → Photos(5);
// Archive(3) isolated. Resources r1→Vacation, r3→Work; r2,r4 owner-less.

func pgGroupIDs(t *testing.T, db *gorm.DB, input string) []uint {
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

func pgResourceIDs(t *testing.T, db *gorm.DB, input string) []uint {
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

func TestPG_RecursiveAncestorsDescendants(t *testing.T) {
	db := setupPostgresTestDB(t)
	cases := []struct {
		query string
		group bool
		want  []uint
	}{
		{`type = "group" AND ancestors.name = "Vacation"`, true, []uint{2, 4, 5}},
		{`type = "group" AND ancestors.tags = "document"`, true, []uint{4}},
		{`type = "group" AND ancestors.meta.region = "europe"`, true, []uint{2, 4, 5}},
		{`type = "group" AND descendants.name = "Sub-Work"`, true, []uint{1, 2}},
		{`type = "group" AND descendants.tags = "document"`, true, []uint{1}},
		{`type = "group" AND descendants.name != "Sub-Work"`, true, []uint{3, 4, 5}},
		{`type = "resource" AND ancestors.name = "Vacation"`, false, []uint{3}},
		{`type = "resource" AND ancestors.name != "Vacation"`, false, []uint{1, 2, 4}},
	}
	for _, tc := range cases {
		var got []uint
		if tc.group {
			got = pgGroupIDs(t, db, tc.query)
		} else {
			got = pgResourceIDs(t, db, tc.query)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("query %q: got %v want %v", tc.query, got, tc.want)
		}
	}
}
