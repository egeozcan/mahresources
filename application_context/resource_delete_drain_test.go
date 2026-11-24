package application_context

import (
	"context"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// cancelOnManifestRead cancels the request the first time the drain reads the
// cleanup manifest, i.e. the moment a client disconnect would land after the commit.
func cancelOnManifestRead(t *testing.T, db *gorm.DB, cancel context.CancelFunc) {
	t.Helper()
	var once sync.Once
	const name = "test:cancel_on_manifest_read"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "resource_cleanup_manifest" {
			once.Do(cancel)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

func drainFixture(t *testing.T, ctx *MahresourcesContext, name string) models.Resource {
	t.Helper()
	r := mkResource(t, ctx, models.Resource{Name: name, Hash: "drain-" + name, Location: "/resources/" + name})
	require.NoError(t, afero.WriteFile(ctx.fs, r.Location, []byte(name), 0o644))
	return r
}

func fileExists(ctx *MahresourcesContext, p string) bool {
	ok, _ := afero.Exists(ctx.fs, p)
	return ok
}

// A client that disconnects after the delete commits must not strand the files or
// the manifest: the drain runs on a context detached from the request's.
func testDrainSurvivesRequestCancellation(t *testing.T, base *MahresourcesContext) {
	a := drainFixture(t, base, "drain-a")
	b := drainFixture(t, base, "drain-b")
	c := drainFixture(t, base, "drain-c")
	d := drainFixture(t, base, "drain-d")

	for _, run := range []struct {
		name string
		do   func(ctx *MahresourcesContext) error
		gone []models.Resource
	}{
		{"bulk", func(ctx *MahresourcesContext) error {
			return ctx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{a.ID, b.ID}})
		}, []models.Resource{a, b}},
		{"merge", func(ctx *MahresourcesContext) error { return ctx.MergeResources(c.ID, []uint{d.ID}, false) }, []models.Resource{d}},
	} {
		reqCtx, cancel := context.WithCancel(context.Background())
		cp := *base
		cp.db = base.db.WithContext(reqCtx)
		cancelOnManifestRead(t, base.db, cancel)
		require.NoError(t, run.do(&cp), run.name)
		cancel()

		var left int64
		require.NoError(t, base.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
		assert.Zero(t, left, "%s: the manifest is drained despite the cancelled request", run.name)
		for _, r := range run.gone {
			assert.False(t, fileExists(base, r.Location), "%s: %s's file is removed", run.name, r.Name)
		}
	}
}

func TestDrainSurvivesRequestCancellationSQLite(t *testing.T) {
	testDrainSurvivesRequestCancellation(t, newReductionSQLiteContext(t))
}
