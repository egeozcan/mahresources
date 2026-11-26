//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func TestScrubLookupUsesStagedIDIndexPG(t *testing.T) {
	testScrubLookupUsesStagedIDIndex(t, newReductionPGContext(t))
}

func TestSQLScrubFollowsLastKeyWinsPG(t *testing.T) {
	testSQLScrubFollowsLastKeyWins(t, newReductionPGContext(t))
}

// Two deletes whose blocks cross: A's ids sit in P's resourceIds and Q's calendars, B's
// in Q's resourceIds and P's calendars. A scrub that makes a gallery pass and then a
// calendar pass locks P then Q for A and Q then P for B, and Postgres kills one with
// 40P01. One statement per block, in ascending block order, cannot cycle.
func TestConcurrentScrubsDoNotDeadlockPG(t *testing.T) {
	ctx := newReductionPGContext(t)
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	const a, b, c, d = 101, 102, 103, 104
	mk := func(pos string, content string) {
		require.NoError(t, ctx.db.Create(&models.NoteBlock{NoteID: note.ID, Type: "gallery", Position: pos, Content: []byte(content)}).Error)
	}
	// P: a in resourceIds, d in calendars. Q: b in resourceIds, c in calendars.
	mk("p", fmt.Sprintf(`{"resourceIds":[%d],"calendars":[{"source":{"resourceId":%d}}]}`, a, d))
	mk("q", fmt.Sprintf(`{"resourceIds":[%d],"calendars":[{"source":{"resourceId":%d}}]}`, b, c))

	// After a transaction's first UPDATE of note_blocks, wait (bounded) for the other
	// to have made its first: the schedule that crosses the two passes.
	var mu sync.Mutex
	seen := map[any]bool{}
	arrived := 0
	cond := sync.NewCond(&mu)
	const name = "test:scrub_barrier"
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(name, func(tx *gorm.DB) {
		if !strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE note_blocks") {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if seen[tx.Statement.ConnPool] {
			return
		}
		seen[tx.Statement.ConnPool] = true
		arrived++
		cond.Broadcast()
		deadline := time.Now().Add(1500 * time.Millisecond)
		timer := time.AfterFunc(1500*time.Millisecond, cond.Broadcast)
		defer timer.Stop()
		for arrived < 2 && time.Now().Before(deadline) {
			cond.Wait()
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(name) })

	errs := make(chan error, 2)
	run := func(ids []uint) {
		errs <- ctx.db.Transaction(func(tx *gorm.DB) error {
			op := newDeleteOp()
			if err := stageDeletedIDs(tx, op, ids); err != nil {
				return err
			}
			if err := scrubDeletedResourcesFromBlocks(tx, op); err != nil {
				return err
			}
			return purgeDeletedIDs(tx, op)
		})
	}
	go run([]uint{a, c})
	go run([]uint{b, d})
	for i := 0; i < 2; i++ {
		assert.NoError(t, <-errs, "a concurrent scrub failed (40P01 is a deadlock)")
	}

	var contents []string
	require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) FROM note_blocks ORDER BY id").Scan(&contents).Error)
	for _, c := range contents {
		decodedEqual(t, `{"resourceIds":[],"calendars":[{"source":{}}]}`, c, "every reference scrubbed")
	}
}
