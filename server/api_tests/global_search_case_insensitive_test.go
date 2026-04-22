//go:build json1 && fts5

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BH-005a: Global search must be case-insensitive on SQLite.
//
// Findings while writing these tests:
//   - FTS5's default unicode61 tokenizer case-folds tokens at index time, so
//     the FTS exact/prefix path is already case-insensitive (including for
//     most Unicode). TestGlobalSearch_CaseInsensitive_FTS is a regression
//     guard — flips if someone swaps in a different tokenizer.
//   - SQLite's built-in LIKE is case-insensitive for ASCII *only*, which
//     covers the main BH-005a scenario ("pasta" finds "Pasta"). The plain
//     LIKE test passes before *and* after the fix.
//   - The FTS fuzzy fallback (fts/sqlite.go `fuzzyFallback`) uses LIKE with
//     single-char wildcards for typo tolerance. Before BH-005a, "~PXSTA"
//     failed to match "Pasta" — the one-char typo substitution worked, but
//     uppercase-vs-lowercase didn't because the raw LIKE pattern "P_STA"
//     required the exact case in the name column.
//   - Non-ASCII (Unicode) case-insensitive LIKE on SQLite needs the ICU
//     extension (not in a stock build). Out of scope for BH-005a; deferred
//     to BH-005b alongside fuzzy typo tolerance.
//
// Fix: wrap both column and pattern in LOWER() in `searchEntitiesLike` and
// `fts.SQLiteFTS.fuzzyFallback` on SQLite. Postgres uses ILIKE via
// getLikeOperator() and is already case-folded end-to-end.

// TestGlobalSearch_CaseInsensitive_FTS documents that with FTS enabled, the
// unicode61 tokenizer case-folds the index — searching "pasta" or "PASTA"
// already matches a tag named "Pasta" because FTS5 stores case-folded tokens.
// This test guards against regressions if we ever change the tokenizer.
func TestGlobalSearch_CaseInsensitive_FTS(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	err = tc.AppCtx.InitFTS()
	require.NoError(t, err, "FTS5 must be available (build with -tags 'json1 fts5')")

	tc.DB.Create(&models.Tag{Name: "Pasta"})

	for _, q := range []string{"Pasta", "pasta", "PASTA"} {
		result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: q,
			Limit: 50,
			Types: []string{"tag"},
		})
		require.NoError(t, err, "query=%q", q)

		found := false
		names := make([]string, 0, len(result.Results))
		for _, r := range result.Results {
			names = append(names, r.Name)
			if r.Name == "Pasta" {
				found = true
			}
		}
		assert.True(t, found,
			"FTS case-insensitive: query %q should match tag 'Pasta'; got %v", q, names)
	}
}

// TestGlobalSearch_CaseInsensitive_LIKEFallback exercises the LIKE fallback
// by NOT calling InitFTS (leaves ctx.ftsEnabled=false). GlobalSearch dispatches
// to searchEntityType → searchEntitiesLike.
//
// SQLite's built-in LIKE and LOWER() are ASCII-only — Unicode case-folding
// requires the ICU extension, which is not part of a stock build. So this
// test covers only the ASCII case. Unicode case-insensitive LIKE on SQLite
// is out of scope for BH-005a and is deferred to BH-005b alongside fuzzy
// typo tolerance.
func TestGlobalSearch_CaseInsensitive_LIKEFallback(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Deliberately DO NOT call InitFTS — forces the LIKE fallback path.
	tc.DB.Create(&models.Tag{Name: "Pasta"})

	for _, q := range []string{"Pasta", "pasta", "PASTA"} {
		result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: q,
			Limit: 50,
			Types: []string{"tag"},
		})
		require.NoError(t, err, "query=%q", q)

		found := false
		names := make([]string, 0, len(result.Results))
		for _, r := range result.Results {
			names = append(names, r.Name)
			if r.Name == "Pasta" {
				found = true
			}
		}
		assert.True(t, found,
			"LIKE fallback case-insensitive: query %q should match tag 'Pasta'; got %v", q, names)
	}
}

// TestGlobalSearch_CaseInsensitive_FTSFuzzy exercises the fts.SQLiteFTS
// fuzzyFallback path, which uses LIKE for basic typo tolerance. The fuzzy
// path also needs LOWER() so that "Pxsta" (one-char typo for "Pasta")
// matches when searched in lowercase.
func TestGlobalSearch_CaseInsensitive_FTSFuzzy(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	err = tc.AppCtx.InitFTS()
	require.NoError(t, err)

	tc.DB.Create(&models.Tag{Name: "Pasta"})

	// Fuzzy mode is triggered by a leading "~".
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "~PXSTA",
		Limit: 50,
		Types: []string{"tag"},
	})
	require.NoError(t, err)

	found := false
	names := make([]string, 0, len(result.Results))
	for _, r := range result.Results {
		names = append(names, r.Name)
		if r.Name == "Pasta" {
			found = true
		}
	}
	assert.True(t, found,
		"FTS fuzzy case-insensitive: query '~PXSTA' should match tag 'Pasta' via one-char typo + case-fold; got %v", names)
}
