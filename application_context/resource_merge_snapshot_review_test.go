package application_context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func snapshotText(t *testing.T, ctx *MahresourcesContext, id uint) string {
	t.Helper()
	var got struct{ Snap string }
	require.NoError(t, ctx.db.Raw("SELECT CAST("+resourceSnapshotExpr("sqlite", "l")+" AS TEXT) AS snap FROM resources l WHERE l.id = ?", id).Scan(&got).Error)
	return got.Snap
}

// The snapshot's times read exactly as json.Marshal writes the time the driver loads
// from the stored text, for every form SQLite can hold one in (a date alone, no seconds,
// a T separator, an offset, no offset).
func TestSnapshotTimesMatchMarshalForEveryStoredFormSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	for _, stored := range []string{
		"2024-06-01",
		"2024-06-01 12:30",
		"2024-06-01T12:30",
		"2024-06-01 12:30:45",
		"2024-06-01T12:30:45",
		"2024-06-01T12:30:45Z",
		"2024-06-01 12:30:45.5",
		"2024-06-01 12:30:45.123456789+02:00",
		"2024-06-01 12:30:45-07:00",
		"2024-06-01 12:30:45+00:00",
	} {
		r := mkResource(t, ctx, models.Resource{Name: "t" + stored})
		require.NoError(t, ctx.db.Exec("UPDATE resources SET created_at = ?, updated_at = ? WHERE id = ?", stored, stored, r.ID).Error)
		var loaded models.Resource
		require.NoError(t, ctx.db.First(&loaded, r.ID).Error)
		want, err := json.Marshal(loaded.CreatedAt)
		require.NoError(t, err)

		var snap map[string]any
		require.NoError(t, json.Unmarshal([]byte(snapshotText(t, ctx, r.ID)), &snap))
		got, err := json.Marshal(snap["CreatedAt"])
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "stored %q", stored)
	}
}

// A Loser's meta that repeats the backups key is rebuilt without it, and no unrelated
// number in it is re-rendered (9.1234567891234549 has more digits than a double prints
// through SQLite's real formatting).
func TestSnapshotRepeatedBackupsKeyKeepsUnrelatedNumbersSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	r := mkResource(t, ctx, models.Resource{Name: "rep"})
	const meta = `{"backups":{"a":1},"depth":9.1234567891234549,"big":1e999,"backups":{"b":2},"n":12345678901234567890}`
	require.NoError(t, ctx.db.Exec("UPDATE resources SET meta = ? WHERE id = ?", meta, r.ID).Error)

	snap := snapshotText(t, ctx, r.ID)
	assert.Contains(t, snap, "9.1234567891234549")
	assert.Contains(t, snap, "12345678901234567890")
	assert.Contains(t, snap, "1e999")
	assert.NotContains(t, snap, `"backups"`)
}
