//go:build postgres

package api_tests

import (
	"mahresources/fts"
	"mahresources/models"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The index checks find an index through the table it belongs to, not through
// current_schema(). A search_path whose first schema exists but does not hold
// the tables made current_schema() name that schema: the planned-index check
// missed a wrongly shaped index and left it in place (CREATE INDEX IF NOT EXISTS
// then skipped the name), and FTS setup missed its existing indexes and went
// back to the plain CREATE INDEX that waits on a concurrent build.
func TestIndexChecksFollowTheTableNotCurrentSchema(t *testing.T) {
	base, dsn := pgContainer.CreateTestDBWithDSN(t)
	require.NoError(t, base.Exec("CREATE SCHEMA ahead").Error)
	require.NoError(t, base.Exec("CREATE TABLE resources (id bigserial PRIMARY KEY, name text, description text, original_name text, owner_id bigint, created_at timestamptz)").Error)
	require.NoError(t, base.Exec("CREATE INDEX idx_resources_owner_created ON resources (name)").Error)
	// A same-named index in the schema ahead on the search_path, which the
	// repair must not touch: an unqualified DROP resolved to it.
	require.NoError(t, base.Exec("CREATE TABLE ahead.other (v int)").Error)
	require.NoError(t, base.Exec("CREATE UNIQUE INDEX idx_resources_owner_created ON ahead.other (v)").Error)

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", "ahead,public")
	u.RawQuery = q.Encode()
	db, err := gorm.Open(pgdriver.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	var current string
	require.NoError(t, db.Raw("SELECT current_schema()").Scan(&current).Error)
	require.Equal(t, "ahead", current, "the test needs current_schema() to differ from the tables' schema")

	require.NoError(t, models.EnsureResourceListIndexes(db))
	var def string
	require.NoError(t, base.Raw("SELECT pg_get_indexdef('public.idx_resources_owner_created'::regclass)").Scan(&def).Error)
	require.Contains(t, def, "USING btree (owner_id, created_at)", "the wrongly shaped index was left in place")
	var untouched string
	require.NoError(t, base.Raw("SELECT pg_get_indexdef('ahead.idx_resources_owner_created'::regclass)").Scan(&untouched).Error)
	require.Contains(t, untouched, "UNIQUE INDEX idx_resources_owner_created ON ahead.other", "the repair dropped another schema's index")

	require.NoError(t, base.Exec("CREATE INDEX idx_resources_fts_probe ON resources (id)").Error)
	exists, err := fts.PostgresIndexExistsOn(db, "resources", "idx_resources_fts_probe")
	require.NoError(t, err)
	require.True(t, exists, "an index on the table was not found through the table")
}
