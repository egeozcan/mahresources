package application_context

import (
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

func TestManifestHookRunsWhenFileStepNeverSucceedsSQLite(t *testing.T) {
	testManifestHookRunsWhenFileStepNeverSucceeds(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestClaimIsRenewedPerRowSQLite(t *testing.T) {
	testManifestClaimIsRenewedPerRow(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}

func TestManifestLostClaimSkipsHookSQLite(t *testing.T) {
	testManifestLostClaimSkipsHook(t, newPluginHookTestContext(t, manifestClaimPlugin()))
}
