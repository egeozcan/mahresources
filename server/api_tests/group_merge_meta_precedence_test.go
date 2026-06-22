package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeGroups_MetaPrecedence verifies the meta-merge precedence rules of
// the batched MergeGroups implementation:
//
//  1. The winner's keys always win a conflict against any loser.
//  2. Among losers, the lowest-id loser wins a conflict — deterministically and
//     independent of the order loserIds are passed in. This guarantees the
//     Postgres and SQLite paths produce identical results.
//  3. All non-conflicting keys from winner and every loser are unioned in.
//  4. The backups key is still written after the merge.
//
// loserHigh is passed BEFORE loserLow in the loserIds slice on purpose, to prove
// precedence is decided by id (not by argument order).
func TestMergeGroups_MetaPrecedence(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	winner := &models.Group{
		Name: "winner-precedence",
		Meta: types.JSON(`{"shared":"W","wonly":"W"}`),
	}
	tc.DB.Create(winner)
	require.NotZero(t, winner.ID)

	// Created first -> lower id. Should win loser-vs-loser conflicts.
	loserLow := &models.Group{
		Name: "loser-low",
		Meta: types.JSON(`{"shared":"low","conflict":"low","lowonly":"low"}`),
	}
	tc.DB.Create(loserLow)
	require.NotZero(t, loserLow.ID)

	// Created second -> higher id.
	loserHigh := &models.Group{
		Name: "loser-high",
		Meta: types.JSON(`{"shared":"high","conflict":"high","highonly":"high"}`),
	}
	tc.DB.Create(loserHigh)
	require.NotZero(t, loserHigh.ID)
	require.Greater(t, loserHigh.ID, loserLow.ID, "loserHigh must have the higher id")

	// Pass higher-id loser first to prove ordering is id-based, not arg-based.
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loserHigh.ID, loserLow.ID})
	require.NoError(t, err, "MergeGroups should succeed")

	var updated models.Group
	require.NoError(t, tc.DB.First(&updated, winner.ID).Error)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(updated.Meta, &meta), "winner meta should be valid JSON")

	// (1) Winner beats every loser on shared keys.
	assert.Equal(t, "W", meta["shared"], "winner key must win over both losers")
	assert.Equal(t, "W", meta["wonly"], "winner-only key must survive")

	// (2) Among losers, the lowest-id loser wins, regardless of arg order.
	assert.Equal(t, "low", meta["conflict"],
		"lowest-id loser must win loser-vs-loser conflicts (deterministic, arg-order independent)")

	// (3) Non-conflicting loser keys are unioned in.
	assert.Equal(t, "low", meta["lowonly"], "lower loser's unique key must be merged in")
	assert.Equal(t, "high", meta["highonly"], "higher loser's unique key must be merged in")

	// (4) Backups are written.
	assert.NotNil(t, meta["backups"], "backups key should be present after merge")

	// Losers are gone.
	var remaining int64
	tc.DB.Model(&models.Group{}).Where("id IN ?", []uint{loserLow.ID, loserHigh.ID}).Count(&remaining)
	assert.Zero(t, remaining, "both losers should be deleted after merge")
}
