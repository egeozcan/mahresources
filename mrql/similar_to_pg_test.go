//go:build postgres

package mrql

import (
	"slices"
	"testing"

	"gorm.io/gorm"
)

// Package 3 SIMILAR TO against real Postgres: the UNION ALL + COALESCE
// predicate and the correlated ORDER BY distance subquery must behave
// identically to SQLite. Same seed pairs as similar_to_test.go.

func setupSimilarityPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupPostgresTestDB(t)
	if err := db.AutoMigrate(&testResourceSimilarity{}); err != nil {
		t.Fatalf("auto-migrate resource_similarities failed: %v", err)
	}
	pairs := []testResourceSimilarity{
		{ResourceID1: 1, ResourceID2: 2, HammingDistance: 3, PDistance: u8(2), ADistance: u8(1)},
		{ResourceID1: 1, ResourceID2: 3, HammingDistance: 8},
		{ResourceID1: 2, ResourceID2: 4, HammingDistance: 0, PDistance: u8(11), ADistance: u8(9)},
		{ResourceID1: 3, ResourceID2: 4, HammingDistance: 5, PDistance: u8(5), ADistance: u8(7)},
	}
	if err := db.Create(&pairs).Error; err != nil {
		t.Fatalf("seed pairs failed: %v", err)
	}
	return db
}

func TestPG_SimilarTo(t *testing.T) {
	db := setupSimilarityPGTestDB(t)

	cases := []struct {
		name  string
		query string
		opts  TranslateOptions
		want  []uint
	}{
		{"default threshold", `SIMILAR TO resource(1)`, TranslateOptions{}, []uint{2, 3}},
		{"WITHIN 5", `SIMILAR TO resource(1) WITHIN 5`, TranslateOptions{}, []uint{2}},
		{"both directions", `SIMILAR TO resource(4) WITHIN 11`, TranslateOptions{}, []uint{2, 3}},
		{"aHash filter", `SIMILAR TO resource(4) WITHIN 11`, TranslateOptions{AHashThreshold: 8}, []uint{3}},
		{"NOT similar", `NOT SIMILAR TO resource(1)`, TranslateOptions{}, []uint{1, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := similarResourceIDs(t, db, tc.query, tc.opts)
			if !slices.Equal(got, tc.want) {
				t.Errorf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		})
	}

	// ORDER BY distance: nearest first; pairless rows (via OR branch) last —
	// the COALESCE(..., 255) sentinel must neutralize the PG NULLS LAST /
	// SQLite NULLS FIRST divergence.
	got := orderedSimilarResourceIDs(t, db, `SIMILAR TO resource(1) ORDER BY distance ASC`, TranslateOptions{})
	if !slices.Equal(got, []uint{2, 3}) {
		t.Errorf("ORDER BY distance ASC: got %v want [2 3]", got)
	}
	got = orderedSimilarResourceIDs(t, db, `SIMILAR TO resource(1) WITHIN 5 OR name = "untagged_file.txt" ORDER BY distance ASC`, TranslateOptions{})
	if !slices.Equal(got, []uint{2, 4}) {
		t.Errorf("pairless-last: got %v want [2 4]", got)
	}
}
