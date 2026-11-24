package application_context

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func strandManifestRows(t *testing.T, ctx *MahresourcesContext, op string, n int) {
	t.Helper()
	rows := make([]models.ResourceCleanupManifest, 0, n)
	for i := 0; i < n; i++ {
		loc := fmt.Sprintf("/resources/%s-%d", op, i)
		require.NoError(t, afero.WriteFile(ctx.fs, loc, []byte("x"), 0o644))
		rows = append(rows, models.ResourceCleanupManifest{Op: op, ResourceID: uint(10000 + i), Hash: fmt.Sprintf("h-%s-%d", op, i), Location: loc, Name: "n"})
	}
	require.NoError(t, ctx.db.CreateInBatches(&rows, 100).Error)
}

// Two instances replaying one manifest each claim a row before acting on it, so a hook
// runs once per row, not once per instance.
func TestConcurrentReplaysRunEachHookOnce(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	const n = 60
	strandManifestRows(t, ctx, "concurrent", n)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx.ReplayResourceCleanupManifest()
		}()
	}
	wg.Wait()

	assert.Equal(t, n, hookFires(t, ctx, "after"), "each row's hook runs exactly once across both replays")
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	assert.Zero(t, left)
}

// failingRemoveFs fails Remove while fail is set.
type failingRemoveFs struct {
	afero.Fs
	mu   sync.Mutex
	fail bool
}

func (f *failingRemoveFs) set(v bool) { f.mu.Lock(); f.fail = v; f.mu.Unlock() }
func (f *failingRemoveFs) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("storage unavailable")
	}
	return f.Fs.Remove(name)
}

// A file step that fails must not be acknowledged: the row stays (attempt counted, backed
// off) and its hook has not run; once the storage is back the row finishes, and the hook
// runs exactly once overall.
func TestAFailedFileStepKeepsTheManifestRow(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	fs := &failingRemoveFs{Fs: ctx.fs}
	ctx.fs = fs
	strandManifestRows(t, ctx, "flaky", 1)
	fs.set(true)

	ctx.ReplayResourceCleanupManifest()

	var rows []models.ResourceCleanupManifest
	require.NoError(t, ctx.db.Find(&rows).Error)
	require.Len(t, rows, 1, "the row was acknowledged although its file was not removed")
	assert.Equal(t, 1, rows[0].Attempts)
	assert.False(t, rows[0].FilesDone)
	assert.Greater(t, rows[0].NextAttemptAt, int64(0), "a backoff is recorded")
	assert.Empty(t, rows[0].ClaimedBy, "the claim is released for the retry")
	assert.Zero(t, hookFires(t, ctx, "after"), "the hook runs only after the file step")

	// Inside the backoff nothing is retried.
	ctx.ReplayResourceCleanupManifest()
	require.NoError(t, ctx.db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].Attempts, "a replay inside the backoff retried the row")

	fs.set(false)
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Where("1 = 1").Update("next_attempt_at", 0).Error)
	ctx.ReplayResourceCleanupManifest()

	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	assert.Zero(t, left)
	assert.Equal(t, 1, hookFires(t, ctx, "after"))
	ok, _ := afero.Exists(ctx.fs, "/resources/flaky-0")
	assert.False(t, ok)
}

// Every file of an operation is handled before any hook runs, across the 500-row
// chunks of the drain, not only within one chunk.
func TestAllFilesPrecedeAllHooksAcrossChunks(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{"after_resource_delete": "after"}))
	n := 2*deleteStageChunk + 3
	strandManifestRows(t, ctx, "chunks", n)

	var mu sync.Mutex
	filesSeen, hooksAtLastFile := 0, 0
	resourceFileRemovalGap = func() {
		mu.Lock()
		defer mu.Unlock()
		filesSeen++
		if filesSeen == n {
			hooksAtLastFile = hookFires(t, ctx, "after")
		}
	}
	t.Cleanup(func() { resourceFileRemovalGap = nil })

	ctx.ReplayResourceCleanupManifest()

	assert.Equal(t, n, filesSeen)
	assert.Zero(t, hooksAtLastFile, "a hook ran before the last file of the operation was handled")
	assert.Equal(t, n, hookFires(t, ctx, "after"))
}
