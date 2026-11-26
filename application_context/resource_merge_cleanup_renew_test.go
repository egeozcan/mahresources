package application_context

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// A merge whose Loser has many cascading children and many note blocks naming it
// runs long cleanup loops inside its transaction. The caller's lease is judged by
// Renew, so every chunk of those loops must call it, not only the stages before.
func testMergeCleanupRenewsBetweenChunks(t *testing.T, ctx *MahresourcesContext) {
	oldRows, oldWindow := deleteChunkRows, scrubWindow
	deleteChunkRows, scrubWindow = 2, 1
	t.Cleanup(func() { deleteChunkRows, scrubWindow = oldRows, oldWindow })

	count := func(loaded bool) int {
		w := makeResource(t, ctx.db, "w", opsRes{Hash: "h"})
		l := makeResource(t, ctx.db, "l", opsRes{Hash: "h"})
		if loaded {
			for i := 0; i < 7; i++ {
				lid := l
				require.NoError(t, ctx.db.Create(&models.Preview{Data: []byte{1}, ContentType: "image/png", ResourceId: &lid}).Error)
			}
			note := &models.Note{Name: "n"}
			require.NoError(t, ctx.db.Create(note).Error)
			for i := 0; i < 5; i++ {
				content := fmt.Sprintf(`{"resourceIds":[%d]}`, l)
				require.NoError(t, ctx.db.Create(&models.NoteBlock{NoteID: note.ID, Type: "gallery", Position: fmt.Sprintf("p%03d", i), Content: []byte(content)}).Error)
			}
		}
		n := 0
		require.NoError(t, ctx.MergeResourcesLocking(w, []uint{l}, false, nil, false, MergeHooks{
			Renew: func(*MahresourcesContext) error { n++; return nil },
		}))
		return n
	}
	base := count(false)
	loaded := count(true)
	// 7 previews in chunks of 2 are 4 statements (3 more than the single empty pass
	// each child table already costs), and 5 gallery blocks in windows of 1 are 5.
	require.GreaterOrEqual(t, loaded-base, 3+4, "child deletes and the block scrub renew per chunk: base=%d loaded=%d", base, loaded)
}

func TestMergeCleanupRenewsBetweenChunksSQLite(t *testing.T) {
	testMergeCleanupRenewsBetweenChunks(t, withSettings(newReductionSQLiteContext(t)))
}
