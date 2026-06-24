package application_context

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

// B1: Adding the first text block to a note that already has a description must
// migrate the description into the block, not wipe it.
func TestCreateBlock_PreservesExistingDescription(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNoteWithDescription(ctx, "n", "Hello world")
	require.NoError(t, err)

	block, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID:  note.ID,
		Type:    "text",
		Content: json.RawMessage(`{"text":""}`),
	})
	require.NoError(t, err)

	var reloaded models.Note
	require.NoError(t, ctx.db.First(&reloaded, note.ID).Error)
	assert.Equal(t, "Hello world", reloaded.Description,
		"adding an empty first text block must not wipe the existing description")

	var content struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(block.Content, &content))
	assert.Equal(t, "Hello world", content.Text,
		"the new first text block should be seeded from the existing description")
}

// A second empty text block must NOT overwrite the description that the first
// text block owns.
func TestCreateBlock_SecondEmptyTextBlockKeepsDescription(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "d",
		Content: json.RawMessage(`{"text":"first"}`),
	})
	require.NoError(t, err)
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "h",
		Content: json.RawMessage(`{"text":""}`),
	})
	require.NoError(t, err)

	var reloaded models.Note
	require.NoError(t, ctx.db.First(&reloaded, note.ID).Error)
	assert.Equal(t, "first", reloaded.Description,
		"the first text block remains the description source after a second text block is added")
}

// B1 (review follow-up): inserting an EMPTY text block *before* the current
// first text block must not clear the description — the new first block inherits
// it. Counting only existing text blocks would miss this; we check position.
func TestCreateBlock_EmptyTextBlockInsertedBeforeFirstPreservesDescription(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	// First text block at position "h" becomes the description source.
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "h", Content: json.RawMessage(`{"text":"Hello"}`),
	})
	require.NoError(t, err)
	var n1 models.Note
	require.NoError(t, ctx.db.First(&n1, note.ID).Error)
	require.Equal(t, "Hello", n1.Description)

	// Insert an EMPTY text block BEFORE it (position "d" < "h").
	inserted, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "d", Content: json.RawMessage(`{"text":""}`),
	})
	require.NoError(t, err)

	var n2 models.Note
	require.NoError(t, ctx.db.First(&n2, note.ID).Error)
	assert.Equal(t, "Hello", n2.Description,
		"inserting an empty text block before the first must not clear the description")

	var content struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(inserted.Content, &content))
	assert.Equal(t, "Hello", content.Text,
		"the new first text block should be seeded from the description")
}

// B2: ReorderBlocks must reject empty-string positions (which sort first and can
// wipe the description via the first-text-block sync).
func TestReorderBlocks_RejectsEmptyPosition(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)
	b1, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "d",
		Content: json.RawMessage(`{"text":"Important"}`),
	})
	require.NoError(t, err)

	err = ctx.ReorderBlocks(note.ID, map[uint]string{b1.ID: ""})
	assert.Error(t, err, "reorder with an empty position must be rejected")

	var reloaded models.Note
	require.NoError(t, ctx.db.First(&reloaded, note.ID).Error)
	assert.Equal(t, "Important", reloaded.Description,
		"a rejected reorder must not wipe the description")

	var reloadedBlock models.NoteBlock
	require.NoError(t, ctx.db.First(&reloadedBlock, b1.ID).Error)
	assert.Equal(t, "d", reloadedBlock.Position, "rejected reorder must not change positions")
}

// B5: ReorderBlocks must reject a position that collides with a block not
// included in the submitted (partial) map.
func TestReorderBlocks_RejectsCollisionWithUnsubmittedBlock(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)
	a, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "d", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "h", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "p", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)

	// Move A onto C's position ("p") via a single-entry partial map.
	err = ctx.ReorderBlocks(note.ID, map[uint]string{a.ID: "p"})
	assert.Error(t, err, "reorder colliding with an unsubmitted block's position must be rejected")
}

// A legitimate full-permutation reorder must still succeed.
func TestReorderBlocks_AllowsValidSwap(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)
	a, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "d", Content: json.RawMessage(`{}`)})
	b, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "h", Content: json.RawMessage(`{}`)})

	err = ctx.ReorderBlocks(note.ID, map[uint]string{a.ID: "h", b.ID: "d"})
	assert.NoError(t, err, "a valid position swap must still be accepted")
}

// B3: UpdateBlockState must reject a literal JSON null (which would wipe state,
// including via the anonymous share-server path).
func TestUpdateBlockState_RejectsNull(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)
	b, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "todos", Position: "d",
		Content: json.RawMessage(`{"items":[{"id":"t1","label":"x"}]}`),
	})
	require.NoError(t, err)

	_, err = ctx.UpdateBlockState(b.ID, json.RawMessage(`{"checked":["t1"]}`))
	require.NoError(t, err)

	_, err = ctx.UpdateBlockState(b.ID, json.RawMessage(`null`))
	assert.Error(t, err, "null state must be rejected")

	var reloaded models.NoteBlock
	require.NoError(t, ctx.db.First(&reloaded, b.ID).Error)
	assert.JSONEq(t, `{"checked":["t1"]}`, string(reloaded.State),
		"a rejected null state write must not wipe the saved state")
}

// B4: UpdateBlockContent must reject a literal JSON null content.
func TestUpdateBlockContent_RejectsNull(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)
	b, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "references", Position: "d",
		Content: json.RawMessage(`{"groupIds":[]}`),
	})
	require.NoError(t, err)

	_, err = ctx.UpdateBlockContent(b.ID, json.RawMessage(`null`))
	assert.Error(t, err, "null content must be rejected")
}

// D3: creating a block whose note has an over-long position string triggers an
// automatic rebalance, so positions never grow unbounded toward the size:64 cap.
func TestCreateBlock_AutoRebalancesLongPositions(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	// Seed a block with an artificially long position (simulates accumulated
	// growth from many same-spot insertions). 12 > rebalanceThreshold (8).
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "divider", Position: "aaaaaaaaaaaa", Content: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	var positions []string
	require.NoError(t, ctx.db.Model(&models.NoteBlock{}).
		Where("note_id = ?", note.ID).Pluck("position", &positions).Error)
	require.NotEmpty(t, positions)
	for _, p := range positions {
		assert.LessOrEqual(t, len(p), 8, "positions should be auto-rebalanced under the threshold")
	}
}

// Review #13: auto-rebalance triggered by a create must PRESERVE block order, not
// just shorten positions. Uses 3 ordered blocks so order is actually verifiable.
func TestCreateBlock_AutoRebalancePreservesOrder(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	a, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "d", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)
	b, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "h", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)
	// "zzzzzzzzzzzz" (12 chars) sorts last and exceeds the rebalance threshold,
	// so creating it triggers the auto-rebalance over all three blocks.
	c, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "divider", Position: "zzzzzzzzzzzz", Content: json.RawMessage(`{}`)})
	require.NoError(t, err)

	blocks, err := ctx.GetBlocksForNote(note.ID)
	require.NoError(t, err)
	require.Len(t, blocks, 3)
	assert.Equal(t, a.ID, blocks[0].ID, "order A,B,C must be preserved through auto-rebalance")
	assert.Equal(t, b.ID, blocks[1].ID)
	assert.Equal(t, c.ID, blocks[2].ID)
	for _, bl := range blocks {
		assert.LessOrEqual(t, len(bl.Position), 8, "positions must be rebalanced under the threshold")
	}
}

// Review #3: two text blocks at the SAME position must resolve the first text
// block deterministically (position ASC, id ASC), so the older non-empty block
// keeps owning the description instead of an empty later block wiping it.
func TestCreateBlock_TiedPositionKeepsDescriptionDeterministic(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "text", Position: "d", Content: json.RawMessage(`{"text":"Hello"}`)})
	require.NoError(t, err)
	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "text", Position: "d", Content: json.RawMessage(`{"text":""}`)})
	require.NoError(t, err)

	var n models.Note
	require.NoError(t, ctx.db.First(&n, note.ID).Error)
	assert.Equal(t, "Hello", n.Description,
		"with the id-ASC tiebreak the older 'Hello' block is first, so the description survives")
}

// Review #6: the ICS scheme allowlist accepts http(s) and rejects everything else.
func TestICS_AllowedScheme(t *testing.T) {
	for _, ok := range []string{"http://x/c.ics", "https://x/c.ics", "HTTPS://X/C.ics"} {
		assert.True(t, allowedICSScheme(ok), "scheme should be allowed: %s", ok)
	}
	for _, bad := range []string{"file:///etc/passwd", "gopher://x/", "ftp://x/c.ics", "", "javascript:alert(1)", "not a url"} {
		assert.False(t, allowedICSScheme(bad), "scheme should be rejected: %s", bad)
	}
}

// Review #6: the runtime fetch must re-validate redirect targets, so an http(s)
// URL that 302-redirects to a non-http(s) scheme is blocked (SSRF defense).
func TestFetchICS_RejectsRedirectToNonHttpScheme(t *testing.T) {
	ctx := createBlockTestContext(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()

	_, _, err := ctx.fetchAndCacheICS(srv.URL, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect to non-http(s) URL blocked")
}

// B4: CreateBlock must also reject explicit null content (non-empty "null").
func TestCreateBlock_RejectsNullContent(t *testing.T) {
	ctx := createBlockTestContext(t)
	note, err := createTestNote(ctx, "n")
	require.NoError(t, err)

	_, err = ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "references", Position: "d",
		Content: json.RawMessage(`null`),
	})
	assert.Error(t, err, "explicit null content on create must be rejected")
}
