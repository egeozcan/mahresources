package application_context

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// cascadeChildrenOf is every (table.column) the database cascades from resources,
// read from the constraints the models declare (OnDelete:CASCADE).
func cascadeChildrenOf(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var out []string
	if db.Dialector.Name() == "postgres" {
		var rows []struct{ Tbl, Col string }
		require.NoError(t, db.Raw(`SELECT c.conrelid::regclass::text AS tbl, a.attname AS col
			FROM pg_constraint c JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
			WHERE c.contype = 'f' AND c.confrelid = 'resources'::regclass AND c.confdeltype = 'c'`).Scan(&rows).Error)
		for _, r := range rows {
			out = append(out, r.Tbl+"."+r.Col)
		}
	} else {
		var tables []string
		require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type = 'table'").Scan(&tables).Error)
		for _, tbl := range tables {
			var fks []struct {
				Table    string `gorm:"column:table"`
				From     string `gorm:"column:from"`
				OnDelete string `gorm:"column:on_delete"`
			}
			require.NoError(t, db.Raw(fmt.Sprintf("SELECT * FROM pragma_foreign_key_list('%s')", tbl)).Scan(&fks).Error)
			for _, fk := range fks {
				if fk.Table == "resources" && strings.EqualFold(fk.OnDelete, "CASCADE") {
					out = append(out, tbl+"."+fk.From)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// A new model that cascades from resources fails here until the delete path
// clears it in chunks (design D9).
func testCascadeListMatchesConstraints(t *testing.T, ctx *MahresourcesContext) {
	require.NoError(t, ctx.db.AutoMigrate(&models.Job{}, &models.JobResourceReceipt{}))
	var want []string
	for _, c := range resourceCascadeChildren {
		want = append(want, c.Table+"."+c.Column)
	}
	sort.Strings(want)
	assert.Equal(t, want, cascadeChildrenOf(t, ctx.db))
}

func TestCascadeListMatchesConstraintsSQLite(t *testing.T) {
	testCascadeListMatchesConstraints(t, newReductionSQLiteContext(t))
}

// Every Resource delete goes through the registered paths: a delete elsewhere would
// skip the Cluster hook, the children and the note-block scrub (design D9).
func TestResourceRowsAreDeletedOnlyInTheRegisteredPaths(t *testing.T) {
	pattern := regexp.MustCompile(`Delete\(&models\.Resource\{|Delete\(&resource\b|Delete\(&res\b|DELETE FROM resources\b|Delete\(&models\.Resource\)`)
	allowed := map[string]bool{"resource_delete_stage.go": true}
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || allowed[name] {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		require.NoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			assert.False(t, pattern.MatchString(line), "%s:%d deletes a Resource outside deleteResourceChunk: %s", name, i+1, strings.TrimSpace(line))
		}
	}
}

// deleteFixture is a Resource with at least one of every cascading child, plus
// more than a chunk of tags and versions.
func deleteFixture(t *testing.T, ctx *MahresourcesContext, name string, tags, versions int) models.Resource {
	t.Helper()
	r := mkResource(t, ctx, models.Resource{Name: name})
	for i := 0; i < tags; i++ {
		tag := &models.Tag{Name: fmt.Sprintf("%s-t%d", name, i)}
		require.NoError(t, ctx.db.Create(tag).Error)
		require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", r.ID, tag.ID).Error)
	}
	for i := 0; i < versions; i++ {
		require.NoError(t, ctx.db.Create(&models.ResourceVersion{ResourceID: r.ID, VersionNumber: i + 1, Hash: fmt.Sprintf("%s-v%d", name, i), HashType: "SHA1", Location: "/x"}).Error)
	}
	note := &models.Note{Name: name + "-note"}
	require.NoError(t, ctx.db.Create(note).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", r.ID, note.ID).Error)
	g := &models.Group{Name: name + "-g"}
	require.NoError(t, ctx.db.Create(g).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)", r.ID, g.ID).Error)
	require.NoError(t, ctx.db.Create(&models.Preview{ResourceId: &r.ID, Data: []byte("p"), ContentType: "image/png"}).Error)
	require.NoError(t, ctx.db.Create(&models.ImageHash{ResourceId: &r.ID, DHash: "d", AHash: "a"}).Error)
	other := mkResource(t, ctx, models.Resource{Name: name + "-other"})
	require.NoError(t, ctx.db.Create(&models.ResourceSimilarity{ResourceID1: r.ID, ResourceID2: other.ID, HammingDistance: 1}).Error)
	third := mkResource(t, ctx, models.Resource{Name: name + "-third"})
	require.NoError(t, ctx.db.Create(&models.ResourceSimilarity{ResourceID1: third.ID, ResourceID2: r.ID, HammingDistance: 2}).Error)
	return r
}

func childCount(t *testing.T, db *gorm.DB, resourceIDs ...uint) int64 {
	t.Helper()
	var total int64
	for _, c := range resourceCascadeChildren {
		var n int64
		require.NoError(t, db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s IN ?", c.Table, c.Column), resourceIDs).Scan(&n).Error)
		total += n
	}
	return total
}

// Each delete path clears the cascading children in statements of at most one
// chunk before the parent row goes, and the parent delete finds none left.
func testDeletePathsClearChildrenInChunks(t *testing.T, ctx *MahresourcesContext) {
	old := deleteChunkRows
	deleteChunkRows = 7
	t.Cleanup(func() { deleteChunkRows = old })

	var mu sync.Mutex
	var maxChild int64
	var leftBehind int64
	const raw = "test:child_delete_rows"
	const before = "test:children_before_parent"
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(raw, func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.HasPrefix(sql, "DELETE FROM ") && !strings.HasPrefix(sql, "DELETE FROM resources") {
			mu.Lock()
			if tx.RowsAffected > maxChild {
				maxChild = tx.RowsAffected
			}
			mu.Unlock()
		}
	}))
	require.NoError(t, ctx.db.Callback().Delete().Before("gorm:delete").Register(before, func(tx *gorm.DB) {
		if tx.Statement.Table != "resources" {
			return
		}
		ids, _ := tx.Statement.Vars, 0
		var flat []uint
		for _, v := range ids {
			switch x := v.(type) {
			case uint:
				flat = append(flat, x)
			case []uint:
				flat = append(flat, x...)
			}
		}
		var n int64
		for _, c := range resourceCascadeChildren {
			var k int64
			if err := tx.Session(&gorm.Session{NewDB: true}).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s IN ?", c.Table, c.Column), flat).Scan(&k).Error; err == nil {
				n += k
			}
		}
		mu.Lock()
		leftBehind += n
		mu.Unlock()
	}))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Raw().Remove(raw)
		_ = ctx.db.Callback().Delete().Remove(before)
	})

	a := deleteFixture(t, ctx, "a", 40, 30)
	b := deleteFixture(t, ctx, "b", 40, 30)
	c := deleteFixture(t, ctx, "c", 40, 30)
	d := deleteFixture(t, ctx, "d", 5, 5)
	e := deleteFixture(t, ctx, "e", 40, 30)
	for _, r := range []models.Resource{a, b, c, d, e} {
		require.Greater(t, childCount(t, ctx.db, r.ID), int64(8))
	}

	require.NoError(t, ctx.DeleteResource(a.ID))
	require.NoError(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{b.ID, c.ID}}))
	require.NoError(t, ctx.MergeResources(d.ID, []uint{e.ID}, false))

	for _, r := range []models.Resource{a, b, c, e} {
		var n int64
		require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id = ?", r.ID).Count(&n).Error)
		assert.Zero(t, n, "resource %d is gone", r.ID)
	}
	assert.Zero(t, childCount(t, ctx.db, a.ID, b.ID, c.ID, e.ID), "no child survives")
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, leftBehind, "no cascading child was left for the parent delete to cascade")
	assert.LessOrEqual(t, maxChild, int64(deleteChunkRows), "no child delete statement removed more than a chunk")
	assert.Greater(t, maxChild, int64(0))
}

func TestDeletePathsClearChildrenInChunksSQLite(t *testing.T) {
	testDeletePathsClearChildrenInChunks(t, newReductionSQLiteContext(t))
}

// The three delete paths end the same way: Reduction members gone, blocks scrubbed,
// staged ids dropped, the manifest drained, and a bulk request over the cap refused.
func testDeletePathsFinishTheSameWay(t *testing.T, ctx *MahresourcesContext) {
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Model(r).Update("plan_generation", "g").Error)

	mk := func(name string) models.Resource { return mkResource(t, ctx, models.Resource{Name: name}) }
	single, bulk1, bulk2, winner, loser := mk("single"), mk("bulk1"), mk("bulk2"), mk("winner"), mk("loser")
	cl := hookCluster(t, ctx, r, "g", "k", winner.ID, member(winner.ID, "h", 1), member(loser.ID, "h", 1), member(single.ID, "h", 1), member(bulk1.ID, "h", 1), member(bulk2.ID, "h", 1))
	for i, ids := range [][]uint{{single.ID}, {bulk1.ID, bulk2.ID}, {loser.ID}} {
		require.NoError(t, ctx.db.Create(&models.NoteBlock{NoteID: note.ID, Type: "gallery", Position: fmt.Sprintf("p%d", i), Content: []byte(fmt.Sprintf(`{"resourceIds":[%d,%d,%d]}`, ids[0], winner.ID, ids[len(ids)-1]))}).Error)
	}

	require.NoError(t, ctx.DeleteResource(single.ID))
	require.NoError(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{bulk1.ID, bulk2.ID}}))
	require.NoError(t, ctx.MergeResources(winner.ID, []uint{loser.ID}, false))

	gone, n := goneOf(t, ctx, cl.Row.ID)
	assert.Equal(t, map[uint]bool{winner.ID: false, loser.ID: true, single.ID: true, bulk1.ID: true, bulk2.ID: true}, gone)
	assert.Equal(t, 4, n)

	var contents []string
	require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) FROM note_blocks ORDER BY id").Scan(&contents).Error)
	for _, c := range contents {
		decodedEqual(t, fmt.Sprintf(`{"resourceIds":[%d]}`, winner.ID), c, "every deleted id is scrubbed, the survivor stays")
	}
	var staged, manifest int64
	require.NoError(t, ctx.db.Model(&models.ReductionDeletedID{}).Count(&staged).Error)
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&manifest).Error)
	assert.Zero(t, staged, "the outer transaction drops the staged ids")
	assert.Zero(t, manifest, "the after-commit drain empties the manifest")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)

	tooMany := make([]uint, MaxBulkDeleteResources+1)
	for i := range tooMany {
		tooMany[i] = uint(i + 1)
	}
	assert.ErrorIs(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: tooMany}), ErrTooManyResources)
}

func TestDeletePathsFinishTheSameWaySQLite(t *testing.T) {
	testDeletePathsFinishTheSameWay(t, newReductionSQLiteContext(t))
}
