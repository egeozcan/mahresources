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

// capturedStmt is one statement the scrub issued, with its bind values.
type capturedStmt struct {
	SQL  string
	Vars []any
}

// captureScrubStatements records every statement that reads reduction_deleted_ids
// and touches note_blocks, on whichever callback chain runs it.
func captureScrubStatements(t *testing.T, db *gorm.DB) func() []capturedStmt {
	t.Helper()
	var mu sync.Mutex
	var got []capturedStmt
	record := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "reduction_deleted_ids") && strings.Contains(sql, "note_blocks") {
			mu.Lock()
			got = append(got, capturedStmt{SQL: sql, Vars: append([]any(nil), tx.Statement.Vars...)})
			mu.Unlock()
		}
	}
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register("test:capture_scrub_raw", record))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:capture_scrub_row", record))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture_scrub_query", record))
	t.Cleanup(func() {
		_ = db.Callback().Raw().Remove("test:capture_scrub_raw")
		_ = db.Callback().Row().Remove("test:capture_scrub_row")
		_ = db.Callback().Query().Remove("test:capture_scrub_query")
	})
	return func() []capturedStmt {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedStmt(nil), got...)
	}
}

// scrubFixture is blocks that name the staged ids, so every statement of the scrub
// runs against rows that match.
func scrubFixture(t *testing.T, ctx *MahresourcesContext, staged []uint) {
	t.Helper()
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	for i := 0; i < 30; i++ {
		id := staged[i%len(staged)]
		content := fmt.Sprintf(`{"resourceIds":[%d,%d],"calendars":[{"source":{"type":"resource","resourceId":%d}}]}`, id, 900+i, id)
		require.NoError(t, ctx.db.Create(&models.NoteBlock{NoteID: note.ID, Type: "gallery", Position: fmt.Sprintf("p%03d", i), Content: []byte(content)}).Error)
	}
}

// The staged-id lookup must be a primary-key probe on (op, resource_id), never a scan
// of every id the operation staged per reference (references x deleted ids).
func testScrubLookupUsesStagedIDIndex(t *testing.T, ctx *MahresourcesContext) {
	staged := []uint{3, 5, 7, 11}
	scrubFixture(t, ctx, staged)
	captured := captureScrubStatements(t, ctx.db)
	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, staged))
	require.NoError(t, scrubDeletedResourcesFromBlocks(ctx.db, op))
	stmts := captured()
	require.NotEmpty(t, stmts)

	probed := 0
	for _, s := range stmts {
		var plan []string
		if ctx.db.Dialector.Name() == "postgres" {
			require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
					return err
				}
				return tx.Raw("EXPLAIN "+s.SQL, s.Vars...).Scan(&plan).Error
			}))
		} else {
			var rows []struct {
				Detail string `gorm:"column:detail"`
			}
			require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+s.SQL, s.Vars...).Scan(&rows).Error)
			for _, r := range rows {
				plan = append(plan, r.Detail)
			}
		}
		for _, line := range plan {
			if ctx.db.Dialector.Name() == "postgres" {
				if strings.Contains(line, "reduction_deleted_ids_pkey") {
					probed++
				}
				assert.NotContains(t, line, "Seq Scan on reduction_deleted_ids", "%s", s.SQL)
				continue
			}
			if strings.Contains(line, "SEARCH d ") || strings.Contains(line, "SCAN d") {
				probed++
				assert.NotContains(t, line, "SCAN d", "the staged ids are scanned: %s\n%s", line, s.SQL)
				assert.Contains(t, line, "resource_id=?", "the staged-id lookup uses only op: %s\n%s", line, s.SQL)
			}
		}
	}
	assert.Greater(t, probed, 0, "no plan mentioned the staged ids")
}

func TestScrubLookupUsesStagedIDIndexSQLite(t *testing.T) {
	testScrubLookupUsesStagedIDIndex(t, newReductionSQLiteContext(t))
}

// scrubDocs are documents with duplicate keys. Go's decoder keeps the last value of a
// repeated key; the SQL scrub must read the same document, so deleting 11 never erases
// a live 12 and a repeated resourceIds is judged by its last array.
var scrubDocs = []string{
	`{"calendars":[{"source":{"type":"resource","resourceId":11,"resourceId":12}}]}`,
	`{"calendars":[{"source":{"type":"resource","resourceId":12,"resourceId":11}}]}`,
	`{"calendars":[{"source":{"type":"resource","resourceId":11}, "source":{"type":"resource","resourceId":12}}]}`,
	`{"calendars":[{"source":{"type":"resource","resourceId":12}, "source":{"type":"resource","resourceId":11}}]}`,
	`{"resourceIds":[11,3],"resourceIds":[12,4]}`,
	`{"resourceIds":[12,4],"resourceIds":[11,3]}`,
	`{"resourceIds":[11],"resourceIds":"x"}`,
	`{"calendars":[{"source":{"resourceId":11}}],"calendars":[{"source":{"resourceId":12}}]}`,
	`{"calendars":[{"source":{"resourceId":12}}],"calendars":[{"source":{"resourceId":11}}],"resourceIds":[11]}`,
	`{"resourceIds":[11,12],"caption":"a","caption":"b"}`,
}

func testSQLScrubFollowsLastKeyWins(t *testing.T, ctx *MahresourcesContext) {
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	type blk struct {
		id      uint
		content string
	}
	var blocks []blk
	for i, doc := range scrubDocs {
		b := &models.NoteBlock{NoteID: note.ID, Type: "gallery", Position: fmt.Sprintf("p%03d", i), Content: []byte(doc)}
		require.NoError(t, ctx.db.Create(b).Error)
		blocks = append(blocks, blk{b.ID, doc})
	}
	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{11}))
	require.NoError(t, scrubDeletedResourcesFromBlocks(ctx.db, op))
	for _, b := range blocks {
		// What the block held when it was stored: jsonb has already applied last-wins.
		var stored struct{ Content string }
		want, _, err := scrubResourcesFromBlockContent(storedBefore(t, ctx, b.id, b.content), map[uint]bool{11: true})
		require.NoError(t, err)
		require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", b.id).Scan(&stored).Error)
		decodedEqual(t, want, stored.Content, "block "+b.content)
	}
}

// storedBefore is the document as the database holds it before the scrub: SQLite keeps
// the text, Postgres's jsonb has dropped repeated keys already. The block is read back
// from a copy staged under another id, so the scrub's own result is never the input.
func storedBefore(t *testing.T, ctx *MahresourcesContext, _ uint, original string) string {
	t.Helper()
	if ctx.db.Dialector.Name() != "postgres" {
		return original
	}
	var s struct{ Content string }
	require.NoError(t, ctx.db.Raw("SELECT CAST(?::jsonb AS TEXT) AS content", original).Scan(&s).Error)
	return s.Content
}

func TestSQLScrubFollowsLastKeyWinsSQLite(t *testing.T) {
	testSQLScrubFollowsLastKeyWins(t, newReductionSQLiteContext(t))
}
