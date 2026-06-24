package application_context

import (
	"encoding/json"
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
