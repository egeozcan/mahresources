package application_context

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// The SQL snapshot decodes to what json.Marshal of the loaded row writes, shape for
// shape: a NULL non-pointer column is its Go zero value (not null, not absent), a
// SQLite timestamp keeps the offset it was stored with, and every repeated backups key
// of meta is stripped. The earlier test compares instants and drops nulls; this one
// does neither.
func testMergeSnapshotShapes(t *testing.T, ctx *MahresourcesContext) {
	sqlite := ctx.db.Dialector.Name() != "postgres"
	zone := func(h, m int) *time.Location { return time.FixedZone("z", h*3600+m*60) }

	type shape struct {
		name string
		r    models.Resource
		sql  []string // statements run on the row after it is created
	}
	shapes := []shape{
		{"null text and number columns", models.Resource{Name: "nulls"}, []string{
			"UPDATE resources SET description = NULL, original_name = NULL, original_location = NULL, category = NULL, content_category = NULL, content_type = NULL, width = NULL, height = NULL, file_size = NULL WHERE id = ?",
		}},
		{"offsets and fractions", models.Resource{Name: "times", CreatedAt: time.Date(2024, 3, 9, 8, 7, 6, 123456789, zone(5, 30)), UpdatedAt: time.Date(2024, 12, 31, 23, 59, 59, 120000000, zone(-3, 0))}, nil},
		{"utc and no fraction", models.Resource{Name: "utc", CreatedAt: time.Date(2024, 3, 9, 8, 7, 6, 0, time.UTC), UpdatedAt: time.Date(2024, 3, 9, 8, 7, 6, 500, time.UTC)}, nil},
		{"duplicate backups keys", models.Resource{Name: "dups", Meta: []byte(`{"backups":{"a":1},"x":1,"backups":{"b":2},"y":[1,2]}`)}, nil},
	}
	for _, s := range shapes {
		loaded := mkResource(t, ctx, s.r)
		for _, stmt := range s.sql {
			require.NoError(t, ctx.db.Exec(stmt, loaded.ID).Error, s.name)
		}
		var fresh models.Resource
		require.NoError(t, ctx.db.First(&fresh, loaded.ID).Error)
		snap := fresh
		snap.Meta = metaWithoutBackups(fresh.Meta)
		want, err := json.Marshal(&snap)
		require.NoError(t, err)

		var got struct{ Snap string }
		require.NoError(t, ctx.db.Raw("SELECT CAST("+resourceSnapshotExpr(ctx.db.Dialector.Name(), "l")+" AS TEXT) AS snap FROM resources l WHERE l.id = ?", loaded.ID).Scan(&got).Error, s.name)

		var w, g map[string]any
		require.NoError(t, json.Unmarshal(want, &w))
		require.NoError(t, json.Unmarshal([]byte(got.Snap), &g), got.Snap)
		if !sqlite {
			// Postgres hands the driver an instant in the server's zone; the snapshot is UTC.
			for _, k := range []string{"CreatedAt", "UpdatedAt"} {
				for _, m := range []map[string]any{w, g} {
					ts, err := time.Parse(time.RFC3339Nano, m[k].(string))
					require.NoError(t, err)
					m[k] = ts.UnixNano()
				}
			}
		}
		assert.Equal(t, w, g, "%s\nwant %s\n got %s", s.name, want, got.Snap)
	}
}

func TestMergeSnapshotShapesSQLite(t *testing.T) {
	testMergeSnapshotShapes(t, newReductionSQLiteContext(t))
}
