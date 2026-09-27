//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestAGroupSelectionIsReadFromOneSnapshotPG is TestAGroupSelectionIsReadFromOneSnapshot
// on Postgres, where the snapshot has to be asked for: under the default READ
// COMMITTED every chunk's query would see the move the instant it commits.
func TestAGroupSelectionIsReadFromOneSnapshotPG(t *testing.T) {
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	require.NoError(t, db.AutoMigrate(&models.Group{}, &models.Resource{}, &models.ResourceCategory{}, &models.ResourceReduction{}))
	readOnly, err := sqlx.Connect("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { readOnly.Close() })
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})

	root := &models.Group{Name: "root"}
	require.NoError(t, db.Create(root).Error)
	children := make([]models.Group, idChunk+1)
	for i := range children {
		children[i] = models.Group{Name: fmt.Sprintf("child %d", i), OwnerId: &root.ID}
	}
	require.NoError(t, db.CreateInBatches(children, 200).Error)
	owners, err := ctx.collectSubtreeGroupIDs(root.ID)
	require.NoError(t, err)
	require.Greater(t, len(owners), idChunk, "the subtree has to span two chunks")
	earlier, later := owners[1], owners[len(owners)-1]
	moved := &models.Resource{Name: "moved", OwnerId: &later}
	require.NoError(t, db.Create(moved).Error)

	mover, err := sqlx.Connect("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { mover.Close() })
	var once sync.Once
	const name = "test:move_between_chunks_pg"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "resources" {
			return
		}
		once.Do(func() {
			if _, err := mover.Exec(`UPDATE resources SET owner_id = $1 WHERE id = $2`, earlier, moved.ID); err != nil {
				t.Errorf("competing move: %v", err)
			}
		})
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })

	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		OwnerId: root.ID, IncludeDescendants: true,
	}, nil, false)
	require.NoError(t, err)
	extent, err := DecodeReductionExtent(created.Extent)
	require.NoError(t, err)
	assert.Equal(t, []uint{moved.ID}, extent.ResourceIDs)
}
