package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func mkScrubBlock(t *testing.T, ctx *MahresourcesContext, noteID uint, n int, content string) uint {
	t.Helper()
	b := &models.NoteBlock{NoteID: noteID, Type: "gallery", Position: fmt.Sprintf("p%04d", n), Content: []byte(content)}
	require.NoError(t, ctx.db.Create(b).Error)
	return b.ID
}

func blockText(t *testing.T, ctx *MahresourcesContext, id uint) string {
	t.Helper()
	var got struct{ Content string }
	require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", id).Scan(&got).Error)
	return got.Content
}

// A number the scrub cannot represent (1e-1000 underflows a float8, 1e999 overflows one)
// in a block that does not reference a deleted id must never fail the delete, and the
// block it sits in must come through byte for byte.
func testScrubNeverFailsOnOddNumbers(t *testing.T, ctx *MahresourcesContext) {
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	odd1 := mkScrubBlock(t, ctx, note.ID, 1, `{"calendars":[{"source":{"resourceId":1e-1000}}]}`)
	odd2 := mkScrubBlock(t, ctx, note.ID, 2, `{"resourceIds":[1e-1000,1e999,5]}`)
	odd3 := mkScrubBlock(t, ctx, note.ID, 3, `{"calendars":[{"source":{"resourceId":1e999}}]}`)
	hit := mkScrubBlock(t, ctx, note.ID, 4, `{"resourceIds":[11,3]}`)
	before := map[uint]string{odd1: blockText(t, ctx, odd1), odd2: blockText(t, ctx, odd2), odd3: blockText(t, ctx, odd3)}

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{11}))
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error { return scrubDeletedResourcesFromBlocks(tx, op) }))

	decodedEqual(t, `{"resourceIds":[3]}`, blockText(t, ctx, hit), "the referencing block is scrubbed")
	for id, want := range before {
		assert.Equal(t, want, blockText(t, ctx, id), "block %d names no deleted id and must be untouched", id)
	}
}

func TestScrubNeverFailsOnOddNumbersSQLite(t *testing.T) {
	testScrubNeverFailsOnOddNumbers(t, newReductionSQLiteContext(t))
}

// A document SQLite reads differently from Go (a repeated key) goes to the Go scrub; a
// value elsewhere in it that float64 cannot hold must not fail that scrub.
func TestGoFallbackToleratesUnrelatedOverflowSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	dup := mkScrubBlock(t, ctx, note.ID, 1, `{"resourceIds":[11,3],"resourceIds":[12,11],"width":1e999,"depth":9.1234567891234549}`)

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{11}))
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error { return scrubDeletedResourcesFromBlocks(tx, op) }), "an unrelated 1e999 aborted the delete")

	got := blockText(t, ctx, dup)
	assert.Contains(t, got, `"resourceIds":[12]`)
	assert.NotContains(t, got, "11")
	assert.Contains(t, got, "1e999", "unrelated numbers keep their text")
	assert.Contains(t, got, "9.1234567891234549", "unrelated numbers keep their text")
}

// A document the Go scrub cannot read at all is left alone and logged, not turned into
// a failed delete.
func TestGoFallbackLeavesAnUnreadableBlockAloneSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	const broken = `{"resourceIds":[11],"resourceIds":[11],"x":}`
	id := mkScrubBlock(t, ctx, note.ID, 1, broken)
	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{11}))
	require.NoError(t, scrubBlockInGo(ctx.db, id, op))
	assert.Equal(t, broken, blockText(t, ctx, id))
}

// SQLite's text-to-integer cast saturates: "9223372036854775808" must not read as
// 9223372036854775807. Go reads the first as 2^63, which is not the staged id.
func TestSQLiteScrubDoesNotSaturateTextIDs(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	over := mkScrubBlock(t, ctx, note.ID, 1, `{"resourceIds":["9223372036854775808","9223372036854775810"]}`)
	exact := mkScrubBlock(t, ctx, note.ID, 2, `{"resourceIds":["9223372036854775807","1"]}`)
	trail := mkScrubBlock(t, ctx, note.ID, 3, `{"resourceIds":["9223372036854775807x"]}`)
	longer := mkScrubBlock(t, ctx, note.ID, 4, `{"resourceIds":["92233720368547758071"]}`)
	beforeOver, beforeLonger := blockText(t, ctx, over), blockText(t, ctx, longer)

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{9223372036854775807}))
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error { return scrubDeletedResourcesFromBlocks(tx, op) }))

	assert.Equal(t, beforeOver, blockText(t, ctx, over), `"9223372036854775808" is not 9223372036854775807`)
	assert.Equal(t, beforeLonger, blockText(t, ctx, longer), "a longer digit run is a different id")
	decodedEqual(t, `{"resourceIds":["1"]}`, blockText(t, ctx, exact), "the exact id is still removed")
	decodedEqual(t, `{"resourceIds":[]}`, blockText(t, ctx, trail), "Sscanf stops at the first non-digit, so the prefix is the id")
}

// The candidate walk bounds the rows it examines, not only the rows it returns: with a
// window of 2, eight blocks that name no staged id still take four windowed statements,
// where one unbounded statement used to parse them all.
func TestScrubCandidateWalkIsWindowed(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	for i := 0; i < 7; i++ {
		mkScrubBlock(t, ctx, note.ID, i, `{"resourceIds":[1,2,3]}`)
	}
	hit := mkScrubBlock(t, ctx, note.ID, 7, `{"resourceIds":[11]}`)

	old := scrubWindow
	scrubWindow = 2
	t.Cleanup(func() { scrubWindow = old })

	var mu sync.Mutex
	candidateStatements := 0
	const name = "test:count_candidates"
	count := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.HasPrefix(sql, "SELECT") && strings.Contains(sql, "FROM note_blocks") && strings.Contains(sql, "LIMIT") {
			mu.Lock()
			candidateStatements++
			mu.Unlock()
		}
	}
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(name+"_raw", count))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name+"_row", count))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name+"_query", count))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Raw().Remove(name + "_raw")
		_ = ctx.db.Callback().Row().Remove(name + "_row")
		_ = ctx.db.Callback().Query().Remove(name + "_query")
	})

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{11}))
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error { return scrubDeletedResourcesFromBlocks(tx, op) }))

	decodedEqual(t, `{"resourceIds":[]}`, blockText(t, ctx, hit), "a match after six empty windows is still found")
	assert.GreaterOrEqual(t, candidateStatements, 4, "the walk is not windowed")
}
