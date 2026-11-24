//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"mahresources/constants"
	"mahresources/models"
)

// newReductionPGContext is a Postgres context with the plan tables migrated and
// the Cluster key collated as a server start does.
func newReductionPGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	require.NoError(t, db.AutoMigrate(
		&models.Resource{}, &models.Note{}, &models.Tag{}, &models.Group{},
		&models.Category{}, &models.NoteType{}, &models.TemplateRevision{}, &models.ResourceCategory{},
		&models.Series{}, &models.Preview{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.ImageHash{}, &models.ResourceSimilarity{},
		&models.ResourceVersion{}, &models.NoteBlock{}, &models.LogEntry{}, &models.User{},
	))
	require.NoError(t, db.AutoMigrate(reductionTables...))
	require.NoError(t, models.EnsureReductionKeyCollation(db))
	readOnly, err := sqlx.Connect("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { readOnly.Close() })
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	require.NoError(t, db.FirstOrCreate(defaultRC, 1).Error)
	return ctx
}

func TestReductionStoreRoundTripPG(t *testing.T) {
	testReductionStoreRoundTrip(t, newReductionPGContext(t))
}

func TestReductionStoreReadsPG(t *testing.T) {
	testReductionStoreReads(t, newReductionPGContext(t))
}

func TestReductionVisibilityMatchesScopeCallbackPG(t *testing.T) {
	testReductionVisibilityMatchesScopeCallback(t, newReductionPGContext, []int64{1, 2, 3})
}

func TestReductionSubqueryArgGetsScopeCallbackPG(t *testing.T) {
	testReductionSubqueryArgGetsScopeCallback(t, newReductionPGContext(t))
}

func TestReductionConfinedShapesNeverShowTheTwinPG(t *testing.T) {
	testReductionConfinedShapesNeverShowTheTwin(t, newReductionPGContext(t))
}

func TestReductionTwentyThousandGroupSubtreePG(t *testing.T) {
	testReductionTwentyThousandGroupSubtree(t, newReductionPGContext(t))
}

func TestSeedKeysOrderAsBytesDoPG(t *testing.T) {
	testSeedKeysOrderAsBytesDo(t, newReductionPGContext(t))
}

func TestReductionStoreMatchesOraclePG(t *testing.T) {
	testReductionStoreMatchesOracle(t, newReductionPGContext, []int64{1, 2, 3, 4})
}
