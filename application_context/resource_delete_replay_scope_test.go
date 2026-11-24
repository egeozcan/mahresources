package application_context

import (
	"errors"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// deleteProbePlugin records, from an after_resource_delete hook, how many times it fired
// and what mah.db showed it.
func deleteProbePlugin() string {
	return `plugin = { name = "hooktest", version = "1.0", description = "reports what a delete hook can see" }
local seen = { fired = {}, groups = {} }

function init()
    mah.on("after_resource_delete", function(data)
        seen.fired[#seen.fired + 1] = tostring(math.floor(data.id))
        local names = {}
        for _, row in ipairs(mah.db.query_groups({ limit = 100 }) or {}) do
            names[#names + 1] = row.name
        end
        seen.groups = names
        return data
    end)
    mah.inject("page_bottom", function(ctx)
        table.sort(seen.groups)
        return "fired:" .. table.concat(seen.fired, "|") .. " groups:" .. table.concat(seen.groups, "|")
    end)
end
`
}

// failManifestReads makes every read of the cleanup manifest fail until the returned
// func is called, so a committed delete leaves its manifest undrained, as a crash would.
func failManifestReads(t *testing.T, db *gorm.DB) (restore func()) {
	t.Helper()
	const name = "test:fail_manifest_reads"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "resource_cleanup_manifest" {
			_ = tx.AddError(errors.New("injected: storage unavailable"))
		}
	}))
	done := false
	restore = func() {
		if !done {
			done = true
			_ = db.Callback().Query().Remove(name)
		}
	}
	t.Cleanup(restore)
	return restore
}

type strandedDelete struct {
	location string
	userID   uint
}

func strandScopedDelete(t *testing.T, ctx *MahresourcesContext) strandedDelete {
	t.Helper()
	principal, inside := scopeProbeFixture(t, ctx)
	res := &models.Resource{Name: "doomed", Hash: "hash-doomed", Location: "/resources/doomed", OwnerId: &inside.ID}
	require.NoError(t, ctx.db.Create(res).Error)
	require.NoError(t, afero.WriteFile(ctx.fs, res.Location, []byte("x"), 0o644))

	restore := failManifestReads(t, ctx.db)
	scoped := ctx.WithPrincipal(principal)
	require.NoError(t, scoped.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{res.ID}}))
	restore()

	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	require.EqualValues(t, 1, left, "the manifest must be stranded for the replay to have anything to do")
	require.Empty(t, nonEmpty(hookSaw(t, ctx, "fired")), "no hook may have run yet")
	return strandedDelete{location: res.Location, userID: principal.UserID}
}

// A replay runs the after-delete hook as the principal that made the delete, not
// unscoped: a plugin's mah.db read must not widen to groups the deleter could not see.
func TestReplayRunsHooksAsTheDeletingPrincipal(t *testing.T) {
	ctx := newPluginHookTestContext(t, deleteProbePlugin())
	s := strandScopedDelete(t, ctx)

	ctx.ReplayResourceCleanupManifest()

	assert.False(t, fileExists(ctx, s.location), "file cleanup still runs")
	assert.Len(t, nonEmpty(hookSaw(t, ctx, "fired")), 1, "the hook ran once")
	saw := hookSaw(t, ctx, "groups")
	assert.Contains(t, saw, "inside")
	assert.NotContains(t, saw, "outside", "the replayed hook saw a group outside the deleting user's scope")
}

// If the deleting account is gone by replay time the hook is skipped, never run
// unscoped; the file cleanup still happens and the manifest is drained.
func TestReplaySkipsHooksWhenTheDeletingUserIsGone(t *testing.T) {
	ctx := newPluginHookTestContext(t, deleteProbePlugin())
	s := strandScopedDelete(t, ctx)
	require.NoError(t, ctx.db.Exec("DELETE FROM users WHERE id = ?", s.userID).Error)

	ctx.ReplayResourceCleanupManifest()

	assert.False(t, fileExists(ctx, s.location))
	assert.Empty(t, nonEmpty(hookSaw(t, ctx, "fired")), "a hook ran for a deleter that no longer exists")
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	assert.Zero(t, left)
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
