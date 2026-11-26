package application_context

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// A new models.Resource field fails here until the SQL snapshot maps it (design
// D13). The mapping is checked against the field's own json tag: key name and
// omitempty.
func TestSnapshotFieldsCoverResource(t *testing.T) {
	rt := reflect.TypeOf(models.Resource{})
	byField := map[string]snapField{}
	for _, f := range resourceSnapshotFields {
		byField[f.Field] = f
	}
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		mapped, ok := byField[sf.Name]
		require.True(t, ok, "models.Resource.%s has no entry in resourceSnapshotFields: map it, or the merge backup silently drops it", sf.Name)
		name, opts, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "" {
			name = sf.Name
		}
		assert.Equal(t, name, mapped.Key, "JSON key of %s", sf.Name)
		omitempty := strings.Contains(opts, "omitempty")
		isAbsent := mapped.Kind == snapAbsent || mapped.Kind == snapOmitNil
		assert.Equal(t, omitempty, isAbsent, "omitempty of %s", sf.Name)
		if mapped.Column != "" {
			gormName := strings.ToLower(snakeCase(sf.Name))
			assert.Equal(t, gormName, mapped.Column, "column of %s", sf.Name)
		}
	}
	assert.Len(t, resourceSnapshotFields, rt.NumField(), "an entry for a field that no longer exists")
}

// snakeCase is GORM's naming for the fields in play here (ID, GUID, Id, ID1).
func snakeCase(s string) string {
	switch s {
	case "ID":
		return "id"
	case "GUID":
		return "guid"
	case "CreatedByUserId":
		return "created_by_user_id"
	case "SeriesID":
		return "series_id"
	case "CurrentVersionID":
		return "current_version_id"
	}
	var out []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_')
		}
		out = append(out, r)
	}
	return string(out)
}

func dropNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e == nil {
				delete(x, k)
			} else {
				x[k] = dropNulls(e)
			}
		}
		return x
	case []any:
		for i := range x {
			x[i] = dropNulls(x[i])
		}
		return x
	}
	return v
}

// normalizeSnapshot decodes a snapshot, replaces the two time fields by their
// instants (the SQL form is UTC, the Go form carries the loaded offset) and, on
// SQLite, drops nulls as json_patch does at the final write, today as well.
func normalizeSnapshot(t *testing.T, raw []byte, dropNull bool) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m), string(raw))
	for _, k := range []string{"CreatedAt", "UpdatedAt"} {
		s, ok := m[k].(string)
		require.True(t, ok, "%s is a string in %s", k, string(raw))
		ts, err := time.Parse(time.RFC3339Nano, s)
		require.NoError(t, err, s)
		m[k] = ts.UnixNano()
	}
	if dropNull {
		dropNulls(m)
	}
	return m
}

func metaOf(t *testing.T, ctx *MahresourcesContext, id uint) map[string]any {
	t.Helper()
	var raw struct{ Meta string }
	require.NoError(t, ctx.db.Raw("SELECT CAST(meta AS TEXT) AS meta FROM resources WHERE id = ?", id).Scan(&raw).Error)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw.Meta), &m), raw.Meta)
	return m
}

func mkResource(t *testing.T, ctx *MahresourcesContext, r models.Resource) models.Resource {
	t.Helper()
	if r.Hash == "" {
		r.Hash = fmt.Sprintf("hash-%s", r.Name)
	}
	if r.Location == "" {
		r.Location = "/resources/" + r.Name
	}
	if r.ResourceCategoryId == 0 {
		r.ResourceCategoryId = 1
	}
	require.NoError(t, ctx.db.Create(&r).Error)
	var loaded models.Resource
	require.NoError(t, ctx.db.First(&loaded, r.ID).Error)
	return loaded
}

// The decoded snapshot equals today's json.Marshal of the loaded row, value types
// included, across the meta shapes a Loser can have; successive merges keep earlier
// snapshots; Winners with NULL, JSON null and backups:null meta take the backups.
func testMergeSnapshotMatchesMarshal(t *testing.T, ctx *MahresourcesContext) {
	sqlite := ctx.db.Dialector.Name() != "postgres"
	guid := "11111111-2222-3333-4444-555555555555"
	alt := "alt"
	group := &models.Group{Name: "owner"}
	require.NoError(t, ctx.db.Create(group).Error)

	type shape struct {
		name  string
		r     models.Resource
		merge bool // merged in the test; the non-object metas are only snapshotted
	}
	shapes := []shape{
		{"object with backups", models.Resource{Name: "obj <&> é", Description: "d\n\"q\"", Meta: []byte(`{"a":1,"n":1.5,"s":"v<","backups":{"resource_9":{"x":1}}}`), OwnMeta: []byte(`{"o":true,"l":[1,2]}`), GUID: &guid, OwnerId: &group.ID, Width: 640, Height: 480, FileSize: 1 << 40, ContentType: "image/png", Category: "c", ContentCategory: "cc", OriginalName: "on", OriginalLocation: "ol", HashType: "SHA1"}, true},
		{"array meta", models.Resource{Name: "arr", Meta: []byte(`[1,2]`), StorageLocation: &alt}, false},
		{"null meta", models.Resource{Name: "nul"}, true},
		{"json null meta", models.Resource{Name: "jn", Meta: []byte(`null`), OwnMeta: []byte(`{}`)}, true},
		{"string meta", models.Resource{Name: "str", Meta: []byte(`"x"`)}, false},
	}
	var loserIDs []uint
	expected := map[uint]map[string]any{}
	for _, s := range shapes {
		loaded := mkResource(t, ctx, s.r)
		snap := loaded
		snap.Meta = metaWithoutBackups(loaded.Meta)
		want, err := json.Marshal(&snap)
		require.NoError(t, err)
		expected[loaded.ID] = normalizeSnapshot(t, want, false)

		// The expression itself, before json_patch has a say: every value, type and
		// null as json.Marshal writes it.
		var got struct{ Snap string }
		require.NoError(t, ctx.db.Raw("SELECT CAST("+resourceSnapshotExpr(ctx.db.Dialector.Name(), "l")+" AS TEXT) AS snap FROM resources l WHERE l.id = ?", loaded.ID).Scan(&got).Error, s.name)
		assert.Equal(t, expected[loaded.ID], normalizeSnapshot(t, []byte(got.Snap), false), s.name)
		if s.merge {
			loserIDs = append(loserIDs, loaded.ID)
			expected[loaded.ID] = normalizeSnapshot(t, want, sqlite)
		}
	}

	check := func(winnerID uint, ids []uint) {
		backups, ok := metaOf(t, ctx, winnerID)["backups"].(map[string]any)
		require.True(t, ok, "the Winner has a backups object")
		for _, id := range ids {
			got, ok := backups[fmt.Sprintf("resource_%d", id)]
			require.True(t, ok, "snapshot of resource %d", id)
			raw, _ := json.Marshal(got)
			assert.Equal(t, expected[id], normalizeSnapshot(t, raw, sqlite), "snapshot of resource %d", id)
		}
	}

	winner := mkResource(t, ctx, models.Resource{Name: "winner", Meta: []byte(`{"keep":"me"}`)})
	require.NoError(t, ctx.MergeResources(winner.ID, loserIDs[:1], false))
	check(winner.ID, loserIDs[:1])

	// A second merge into the same Winner keeps the earlier snapshots.
	require.NoError(t, ctx.MergeResources(winner.ID, loserIDs[1:], false))
	check(winner.ID, loserIDs)
	assert.Equal(t, "me", metaOf(t, ctx, winner.ID)["keep"])

	// Winners whose meta is NULL, JSON null, or carries backups:null.
	for i, meta := range []string{"", "null", `{"backups":null}`, `{"x":1}`} {
		w := models.Resource{Name: fmt.Sprintf("w%d", i)}
		if meta != "" {
			w.Meta = []byte(meta)
		}
		wl := mkResource(t, ctx, w)
		l := mkResource(t, ctx, models.Resource{Name: fmt.Sprintf("l%d", i), Meta: []byte(`{"lm":1}`)})
		snap := l
		snap.Meta = metaWithoutBackups(l.Meta)
		want, err := json.Marshal(&snap)
		require.NoError(t, err)
		expected[l.ID] = normalizeSnapshot(t, want, sqlite)
		require.NoError(t, ctx.MergeResources(wl.ID, []uint{l.ID}, false), "winner meta %q", meta)
		check(wl.ID, []uint{l.ID})
		assert.EqualValues(t, 1, metaOf(t, ctx, wl.ID)["lm"], "the Loser's ordinary meta merged across")
	}
}

func TestMergeSnapshotMatchesMarshalSQLite(t *testing.T) {
	testMergeSnapshotMatchesMarshal(t, newReductionSQLiteContext(t))
}

// The merge entry refuses more Losers than a Cluster holds, whatever its caller.
func TestMergeRefusesMoreThanOneCapOfLosers(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	ids := make([]uint, reductionMemberCap)
	for i := range ids {
		ids[i] = uint(i + 2)
	}
	err := ctx.MergeResources(1, ids, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTooManyResources)
}
