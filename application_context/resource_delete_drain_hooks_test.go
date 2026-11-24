package application_context

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// Within a drain every file is processed before any after-delete hook runs, as the
// request-sized slices did before the manifest: a hook sees the whole delete finished.
func TestDrainFinishesAllFilesBeforeAnyAfterHook(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	var ids []uint
	for _, n := range []string{"o1", "o2", "o3"} {
		r := makeHookTestResource(t, ctx, n)
		ids = append(ids, r.ID)
	}
	var hooksAtFile []int
	resourceFileRemovalGap = func() { hooksAtFile = append(hooksAtFile, countTagsWithPrefix(t, ctx, "after-")) }
	t.Cleanup(func() { resourceFileRemovalGap = nil })

	require.NoError(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: ids}))
	require.Len(t, hooksAtFile, 3)
	assert.Equal(t, []int{0, 0, 0}, hooksAtFile, "no after_resource_delete hook ran before the last file was handled")
	assert.Equal(t, 3, hookFires(t, ctx, "after"))
}

// A manifest left by a crash between the commit and the drain is finished by the
// startup replay: file removed, hook fired, rows gone.
func TestReplayFinishesAnUndrainedManifest(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	for i, op := range []string{"aa", "bb"} {
		loc := "/resources/stranded" + op
		require.NoError(t, afero.WriteFile(ctx.fs, loc, []byte("x"), 0o644))
		require.NoError(t, ctx.db.Create(&models.ResourceCleanupManifest{Op: op, ResourceID: uint(900 + i), Hash: "h" + op, Location: loc, Name: "n" + op}).Error)
	}
	ctx.ReplayResourceCleanupManifest()
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	assert.Zero(t, left)
	assert.False(t, fileExists(ctx, "/resources/strandedaa"))
	assert.False(t, fileExists(ctx, "/resources/strandedbb"))
	assert.Equal(t, 2, hookFires(t, ctx, "after"))
}

// The hooks still fire for a request that disconnected after the commit.
func TestDrainRunsAfterHooksForACancelledRequest(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	a := makeHookTestResource(t, ctx, "ca")
	b := makeHookTestResource(t, ctx, "cb")
	reqCtx, cancel := context.WithCancel(context.Background())
	cp := *ctx
	cp.db = ctx.db.WithContext(reqCtx)
	cancelOnManifestRead(t, ctx.db, cancel)
	require.NoError(t, cp.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{a.ID, b.ID}}))
	assert.Equal(t, 2, hookFires(t, ctx, "after"))
}
