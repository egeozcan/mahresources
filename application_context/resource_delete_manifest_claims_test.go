package application_context

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func manifestClaimPlugin() string {
	return hookTagPlugin(map[string]string{"after_resource_delete": "after"})
}

// A manifest row whose file step can never succeed (its storage is gone) still gets its
// after_resource_delete hook: after manifestMaxFileAttempts failed steps the drain
// leaves the file, runs the hook once and drops the row.
func testManifestHookRunsWhenFileStepNeverSucceeds(t *testing.T, ctx *MahresourcesContext) {
	gone := "storage-that-is-gone"
	require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{
		Op: "stuck", ResourceID: 901, Hash: "hstuck", Location: "/x/stuck", Name: "stuck",
		StorageLocation: &gone, Attempts: manifestMaxFileAttempts - 2,
	}).Error)

	// An earlier failure: the row is kept, backed off, its hook not yet run.
	ctx.ReplayResourceCleanupManifest()
	var row models.ResourceCleanupManifest
	require.NoError(t, ctx.db.Where("op = ?", "stuck").First(&row).Error)
	assert.Equal(t, manifestMaxFileAttempts-1, row.Attempts)
	assert.Equal(t, 0, hookFires(t, ctx, "after"), "the hook must wait while the file step may still succeed")

	// The bounded attempt: give up on the file, run the hook, drop the row.
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Where("op = ?", "stuck").Update("next_attempt_at", 0).Error)
	ctx.ReplayResourceCleanupManifest()
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Where("op = ?", "stuck").Count(&left).Error)
	assert.Zero(t, left, "the row is dropped once the file step is given up on")
	assert.Equal(t, 1, hookFires(t, ctx, "after"))

	ctx.ReplayResourceCleanupManifest()
	assert.Equal(t, 1, hookFires(t, ctx, "after"), "the hook runs at most once")
}

// Each row's claim is renewed just before its own work, so a chunk that takes longer
// than the claim TTL is not reclaimable row by row.
func testManifestClaimIsRenewedPerRow(t *testing.T, ctx *MahresourcesContext) {
	old := manifestClaimTTL
	manifestClaimTTL = 2 * time.Second
	t.Cleanup(func() { manifestClaimTTL = old })

	for i, n := range []string{"r1", "r2"} {
		loc := "/resources/renew" + n
		require.NoError(t, afero.WriteFile(ctx.fs, loc, []byte("x"), 0o644))
		require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{Op: "renew", ResourceID: uint(910 + i), Hash: "hrenew" + n, Location: loc, Name: n}).Error)
	}
	var stale []string
	call := 0
	resourceFileRemovalGap = func() {
		// The row being worked on is the call-th by id; rows already done may sit out
		// the rest of the chunk, but the one in hand must hold a live claim.
		var rows []models.ResourceCleanupManifest
		require.NoError(t, ctx.db.Where("op = ?", "renew").Order("id ASC").Find(&rows).Error)
		if cur := rows[call]; cur.ClaimedUntil <= time.Now().Unix() {
			stale = append(stale, cur.Name)
		}
		call++
		time.Sleep(2500 * time.Millisecond)
	}
	t.Cleanup(func() { resourceFileRemovalGap = nil })

	ctx.ReplayResourceCleanupManifest()
	assert.Empty(t, stale, "a row's claim had expired by the time its file step ran: another instance could take it over")
}

// A row another drain took over (our claim expired) is not worked on: its hook does not
// run here, and its row is not dropped from under the new holder.
func testManifestLostClaimSkipsHook(t *testing.T, ctx *MahresourcesContext) {
	for i, n := range []string{"l1", "l2"} {
		require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{
			Op: "lost", ResourceID: uint(920 + i), Hash: "hlost" + n, Location: "/resources/lost" + n, Name: n, FilesDone: true,
		}).Error)
	}
	var second models.ResourceCleanupManifest
	require.NoError(t, ctx.db.Where("op = ? AND name = ?", "lost", "l2").First(&second).Error)

	// The moment the drain has read its hook batch, another instance takes row 2.
	const cb = "test:steal_after_hook_read"
	var stolen bool
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(cb, func(tx *gorm.DB) {
		if stolen || tx.Statement.Table != "resource_cleanup_manifest" || !strings.Contains(tx.Statement.SQL.String(), "files_done") {
			return
		}
		stolen = true
		raw := tx.Session(&gorm.Session{NewDB: true})
		require.NoError(t, raw.Exec("UPDATE resource_cleanup_manifest SET claimed_by = 'other', claimed_until = ? WHERE id = ?",
			time.Now().Add(time.Hour).Unix(), second.ID).Error)
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(cb) })

	ctx.ReplayResourceCleanupManifest()
	require.True(t, stolen, "the steal never fired; the test no longer exercises the hook pass")
	assert.Equal(t, 1, hookFires(t, ctx, "after"), "only the row this drain still held may run its hook")
	var rows []models.ResourceCleanupManifest
	require.NoError(t, ctx.db.Where("op = ?", "lost").Find(&rows).Error)
	require.Len(t, rows, 1, "the taken-over row stays for its new holder")
	assert.Equal(t, "other", rows[0].ClaimedBy)
}

// Each row is dropped right after its own hook, not at the end of the chunk: when a later
// row's hook outlasts the claim TTL and a competing claimer arrives, it finds only the
// row that is still undone, so an already-fired hook never replays.
func testManifestFiredHookDoesNotReplayAfterStall(t *testing.T, ctx *MahresourcesContext) {
	old := manifestClaimTTL
	manifestClaimTTL = time.Second
	t.Cleanup(func() { manifestClaimTTL = old })

	for i, n := range []string{"a", "b"} {
		require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{
			Op: "stall", ResourceID: uint(930 + i), Hash: "hstall" + n, Location: "/resources/stall" + n, Name: n, FilesDone: true,
		}).Error)
	}
	var rival bool
	manifestAfterHookGap = func(resourceID uint) {
		if resourceID != 931 || rival {
			return
		}
		rival = true
		time.Sleep(2200 * time.Millisecond) // the second hook outlasts the claim
		ctx.drainManifestOp("stall", true)  // a competing claimer arrives
	}
	t.Cleanup(func() { manifestAfterHookGap = nil })

	ctx.ReplayResourceCleanupManifest()
	require.True(t, rival, "the stall never fired")
	assert.Equal(t, 3, hookFires(t, ctx, "after"), "the first row's hook must not run again; only the stalled row may repeat (at least once)")
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Where("op = ?", "stall").Count(&left).Error)
	assert.Zero(t, left)
}

// A drain whose claim lapsed during a file step must not write files_done onto the row a
// successor now holds.
func testManifestExpiredDrainDoesNotMarkSuccessorsFilesDone(t *testing.T, ctx *MahresourcesContext) {
	old := manifestClaimTTL
	manifestClaimTTL = time.Second
	t.Cleanup(func() { manifestClaimTTL = old })

	loc := "/resources/successor"
	require.NoError(t, afero.WriteFile(ctx.fs, loc, []byte("x"), 0o644))
	require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{Op: "succ", ResourceID: 940, Hash: "hsucc", Location: loc, Name: "s"}).Error)
	var stolen bool
	resourceFileRemovalGap = func() {
		if stolen {
			return
		}
		stolen = true
		time.Sleep(2200 * time.Millisecond)
		require.NoError(t, ctx.db.Exec("UPDATE resource_cleanup_manifest SET claimed_by = 'successor', claimed_until = ? WHERE op = 'succ'",
			time.Now().Add(time.Hour).Unix()).Error)
	}
	t.Cleanup(func() { resourceFileRemovalGap = nil })

	ctx.ReplayResourceCleanupManifest()
	require.True(t, stolen)
	var row models.ResourceCleanupManifest
	require.NoError(t, ctx.db.Where("op = ?", "succ").First(&row).Error)
	assert.Equal(t, "successor", row.ClaimedBy, "the successor's claim must survive")
	assert.False(t, row.FilesDone, "an expired drain must not mark the successor's row files_done")
	assert.Equal(t, 0, hookFires(t, ctx, "after"))
}

func TestManifestFiredHookDoesNotReplayAfterStallSQLite(t *testing.T) {
	testManifestFiredHookDoesNotReplayAfterStall(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestExpiredDrainDoesNotMarkSuccessorsFilesDoneSQLite(t *testing.T) {
	testManifestExpiredDrainDoesNotMarkSuccessorsFilesDone(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestHookRunsWhenFileStepNeverSucceedsSQLite(t *testing.T) {
	testManifestHookRunsWhenFileStepNeverSucceeds(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestClaimIsRenewedPerRowSQLite(t *testing.T) {
	testManifestClaimIsRenewedPerRow(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestLostClaimSkipsHookSQLite(t *testing.T) {
	testManifestLostClaimSkipsHook(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

// A row that only became due after startup (a failed file step's backoff elapsed) is drained
// by the periodic replay with no restart, and the loop stops when its context ends.
func testManifestPeriodicReplayDrainsWithoutRestart(t *testing.T, ctx *MahresourcesContext) {
	require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{
		Op: "periodic", ResourceID: 950, Hash: "hperiodic", Location: "/resources/periodic", Name: "p",
		FilesDone: true, NextAttemptAt: time.Now().Add(time.Hour).Unix(),
	}).Error)

	loop, stop := context.WithCancel(context.Background())
	done := ctx.StartResourceManifestReplay(loop, 50*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, hookFires(t, ctx, "after"), "a row backed off into the future is not drained yet")

	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Where("op = ?", "periodic").Update("next_attempt_at", 0).Error)
	require.Eventually(t, func() bool { return hookFires(t, ctx, "after") == 1 }, 5*time.Second, 25*time.Millisecond,
		"the due row was never drained by the periodic replay")

	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the replay loop did not stop with its context")
	}
}

func TestManifestPeriodicReplayDrainsWithoutRestartSQLite(t *testing.T) {
	testManifestPeriodicReplayDrainsWithoutRestart(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}
