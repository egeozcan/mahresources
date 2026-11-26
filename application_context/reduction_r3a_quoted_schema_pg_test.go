//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mahresources/models"
)

// A library whose Postgres schema needs quoting spells its indexes
// `ON "my library".resource_similarities`; readiness must still read them valid
// and the boot-time ensure must not drop and rebuild them.
func TestSimilarityIndexReadinessInAQuotedSchemaPG(t *testing.T) {
	admin, dsn := pgContainer.CreateTestDBWithDSN(t)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "my library"`).Error)
	var name string
	require.NoError(t, admin.Raw(`SELECT current_database()`).Scan(&name).Error)
	// New connections (the ensure opens its own) resolve unqualified names there.
	require.NoError(t, admin.Exec(`ALTER DATABASE "`+name+`" SET search_path = "my library"`).Error)

	open := func() *gorm.DB {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { sqlDB.Close() })
		return db
	}
	db := open()
	require.NoError(t, db.Exec(`CREATE TABLE resource_similarities (resource_id1 bigint, resource_id2 bigint, p_distance bigint, hamming_distance bigint)`).Error)
	var schema string
	require.NoError(t, db.Raw(`SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relname = 'resource_similarities'`).Scan(&schema).Error)
	require.Equal(t, "my library", schema, "the table landed in the quoted schema")

	require.NoError(t, models.EnsureSimilarityDistanceIndexesOnOwnConnection(dsn))
	ok, err := models.SimilarityDistanceIndexesValid(db)
	require.NoError(t, err)
	require.True(t, ok, "valid indexes in a schema that needs quoting read as not ready")

	oid := func() uint32 {
		var v uint32
		require.NoError(t, db.Raw(`SELECT c.oid::bigint FROM pg_class c WHERE c.relname = 'idx_sim_r1_eff_dist'`).Scan(&v).Error)
		return v
	}
	before := oid()
	require.NotZero(t, before)
	require.NoError(t, models.EnsureSimilarityDistanceIndexesOnOwnConnection(dsn))
	require.Equal(t, before, oid(), "a valid index was dropped and rebuilt on the next boot")
}
