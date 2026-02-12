package application_context

import (
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createBlockTestContext creates a test context with NoteBlock migration
func createBlockTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Run migrations including NoteBlock
	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.LogEntry{},
		&models.NoteBlock{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	config := &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	}

	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	return NewMahresourcesContext(fs, db, readOnlyDB, config)
}

// createTestNote is a helper that creates a note for testing
func createTestNote(ctx *MahresourcesContext, name string) (*models.Note, error) {
	editor := query_models.NoteEditor{}
	editor.Name = name
	return ctx.CreateOrUpdateNote(&editor)
}

// createTestNoteWithDescription creates a note with a description for testing
func createTestNoteWithDescription(ctx *MahresourcesContext, name, description string) (*models.Note, error) {
	editor := query_models.NoteEditor{}
	editor.Name = name
	editor.Description = description
	return ctx.CreateOrUpdateNote(&editor)
}

func TestBlockContext_CreateBlock(t *testing.T) {
	ctx := createBlockTestContext(t)

	// Create a note first
	note, err := createTestNote(ctx, "Test Note")
	assert.NoError(t, err)

	// Create a block
	editor := &query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "text",
		Position: "n",
		Content:  json.RawMessage(`{"text": "Hello"}`),
	}
	block, err := ctx.CreateBlock(editor)
	assert.NoError(t, err)
	assert.Equal(t, "text", block.Type)
	assert.Equal(t, "n", block.Position)
}

func TestBlockContext_CreateBlock_DefaultContent(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Create block without content - should use default
	editor := &query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "text",
		Position: "n",
	}
	block, err := ctx.CreateBlock(editor)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"text": ""}`, string(block.Content))
}

func TestBlockContext_CreateBlock_InvalidType(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	editor := &query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "unknown_type",
		Position: "n",
	}
	_, err := ctx.CreateBlock(editor)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown block type")
}

func TestBlockContext_CreateBlock_InvalidContent(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Text block requires "text" field
	editor := &query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "text",
		Position: "n",
		Content:  json.RawMessage(`{"invalid": "content"}`),
	}
	_, err := ctx.CreateBlock(editor)
	assert.Error(t, err)
}

func TestBlockContext_GetBlock(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	created, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Hello"}`),
	})

	block, err := ctx.GetBlock(created.ID)
	assert.NoError(t, err)
	assert.Equal(t, created.ID, block.ID)
	assert.Equal(t, "text", block.Type)
}

func TestBlockContext_GetBlock_NotFound(t *testing.T) {
	ctx := createBlockTestContext(t)

	_, err := ctx.GetBlock(99999)
	assert.Error(t, err)
}

func TestBlockContext_GetBlocksForNote(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Create blocks
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "b", Content: json.RawMessage(`{"text": "Second"}`),
	})
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "First"}`),
	})

	blocks, err := ctx.GetBlocksForNote(note.ID)
	assert.NoError(t, err)
	assert.Len(t, blocks, 2)
	// Should be ordered by position
	assert.Equal(t, "a", (blocks)[0].Position)
	assert.Equal(t, "b", (blocks)[1].Position)
}

func TestBlockContext_GetBlocksForNote_Empty(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	blocks, err := ctx.GetBlocksForNote(note.ID)
	assert.NoError(t, err)
	assert.Len(t, blocks, 0)
}

func TestBlockContext_UpdateBlockContent(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Old"}`),
	})

	newContent := json.RawMessage(`{"text": "New"}`)
	updated, err := ctx.UpdateBlockContent(block.ID, newContent)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"text": "New"}`, string(updated.Content))
}

func TestBlockContext_UpdateBlockContent_InvalidContent(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Hello"}`),
	})

	// Text block requires "text" field
	invalidContent := json.RawMessage(`{"invalid": "content"}`)
	_, err := ctx.UpdateBlockContent(block.ID, invalidContent)
	assert.Error(t, err)
}

func TestBlockContext_UpdateBlockState(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "todos", Position: "n",
		Content: json.RawMessage(`{"items": [{"id": "x1", "label": "Task"}]}`),
	})

	newState := json.RawMessage(`{"checked": ["x1"]}`)
	updated, err := ctx.UpdateBlockState(block.ID, newState)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"checked": ["x1"]}`, string(updated.State))
}

func TestBlockContext_UpdateBlockState_InvalidState(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "todos", Position: "n",
		Content: json.RawMessage(`{"items": []}`),
	})

	// Invalid JSON should fail
	invalidState := json.RawMessage(`{invalid json}`)
	_, err := ctx.UpdateBlockState(block.ID, invalidState)
	assert.Error(t, err)
}

func TestBlockContext_DeleteBlock(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Delete me"}`),
	})

	err := ctx.DeleteBlock(block.ID)
	assert.NoError(t, err)

	blocks, _ := ctx.GetBlocksForNote(note.ID)
	assert.Len(t, blocks, 0)
}

func TestBlockContext_DeleteBlock_NotFound(t *testing.T) {
	ctx := createBlockTestContext(t)

	err := ctx.DeleteBlock(99999)
	assert.Error(t, err)
}

func TestBlockContext_ReorderBlocks(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	block1, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "First"}`),
	})
	block2, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "b", Content: json.RawMessage(`{"text": "Second"}`),
	})

	// Swap positions
	positions := map[uint]string{
		block1.ID: "b",
		block2.ID: "a",
	}
	err := ctx.ReorderBlocks(note.ID, positions)
	assert.NoError(t, err)

	// Verify new order
	blocks, _ := ctx.GetBlocksForNote(note.ID)
	assert.Equal(t, block2.ID, (blocks)[0].ID) // block2 now first (position "a")
	assert.Equal(t, block1.ID, (blocks)[1].ID) // block1 now second (position "b")
}

func TestBlockContext_ReorderBlocks_EmptyPositions(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Empty positions should succeed (no-op)
	err := ctx.ReorderBlocks(note.ID, map[uint]string{})
	assert.NoError(t, err)
}

func TestBlockContext_ReorderBlocks_InvalidBlockID(t *testing.T) {
	ctx := createBlockTestContext(t)

	note1, _ := createTestNote(ctx, "Note 1")
	note2, _ := createTestNote(ctx, "Note 2")

	// Create block in note2
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note2.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "Hello"}`),
	})

	// Try to reorder note1 with a block from note2 - should fail
	positions := map[uint]string{
		block.ID: "z",
	}
	err := ctx.ReorderBlocks(note1.ID, positions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "do not belong to the specified note")
}

func TestBlockContext_ReorderBlocks_MixedValidInvalid(t *testing.T) {
	ctx := createBlockTestContext(t)

	note1, _ := createTestNote(ctx, "Note 1")
	note2, _ := createTestNote(ctx, "Note 2")

	// Create block in each note
	block1, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note1.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "Block 1"}`),
	})
	block2, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note2.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "Block 2"}`),
	})

	// Try to reorder note1 with a mix of valid and invalid blocks - should fail
	positions := map[uint]string{
		block1.ID: "z",
		block2.ID: "y", // This block belongs to note2
	}
	err := ctx.ReorderBlocks(note1.ID, positions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "do not belong to the specified note")
}

func TestBlockContext_SyncDescriptionOnCreate(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Create first text block
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n",
		Content: json.RawMessage(`{"text": "This is the first paragraph"}`),
	})

	// Note description should be synced
	var updatedNote models.Note
	ctx.db.First(&updatedNote, note.ID)
	assert.Equal(t, "This is the first paragraph", updatedNote.Description)
}

func TestBlockContext_SyncDescriptionOnUpdate(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n",
		Content: json.RawMessage(`{"text": "Original"}`),
	})

	// Update the block content
	ctx.UpdateBlockContent(block.ID, json.RawMessage(`{"text": "Updated text"}`))

	// Note description should be synced
	var updatedNote models.Note
	ctx.db.First(&updatedNote, note.ID)
	assert.Equal(t, "Updated text", updatedNote.Description)
}

func TestBlockContext_SyncDescriptionOnDelete(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Create two text blocks
	block1, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "a",
		Content: json.RawMessage(`{"text": "First block"}`),
	})
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "b",
		Content: json.RawMessage(`{"text": "Second block"}`),
	})

	// Delete first block - description should sync to second block
	ctx.DeleteBlock(block1.ID)

	var updatedNote models.Note
	ctx.db.First(&updatedNote, note.ID)
	assert.Equal(t, "Second block", updatedNote.Description)
}

func TestBlockContext_SyncDescriptionSkipsNonTextBlocks(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNoteWithDescription(ctx, "Test", "Original description")

	// Create a non-text block (heading)
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "heading", Position: "a",
		Content: json.RawMessage(`{"text": "Heading", "level": 1}`),
	})

	// Create text block after
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "b",
		Content: json.RawMessage(`{"text": "First text block"}`),
	})

	// Description should sync to first TEXT block, not heading
	var updatedNote models.Note
	ctx.db.First(&updatedNote, note.ID)
	assert.Equal(t, "First text block", updatedNote.Description)
}

func TestBlockContext_RebalanceBlockPositions(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Create blocks with long position strings (simulating many insertions)
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "aaaa",
		Content: json.RawMessage(`{"text": "First"}`),
	})
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "aaab",
		Content: json.RawMessage(`{"text": "Second"}`),
	})
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "zzzz",
		Content: json.RawMessage(`{"text": "Third"}`),
	})

	// Rebalance positions
	err := ctx.RebalanceBlockPositions(note.ID)
	assert.NoError(t, err)

	// Fetch blocks and verify
	blocks, _ := ctx.GetBlocksForNote(note.ID)
	assert.Len(t, blocks, 3)

	// Positions should now be single characters and evenly distributed
	for _, block := range blocks {
		assert.Len(t, block.Position, 1, "position should be single character after rebalancing")
	}

	// Order should be preserved
	assert.Equal(t, "First", extractText((blocks)[0].Content))
	assert.Equal(t, "Second", extractText((blocks)[1].Content))
	assert.Equal(t, "Third", extractText((blocks)[2].Content))
}

func TestBlockContext_RebalanceBlockPositions_Empty(t *testing.T) {
	ctx := createBlockTestContext(t)

	note, _ := createTestNote(ctx, "Test")

	// Rebalancing empty note should succeed (no-op)
	err := ctx.RebalanceBlockPositions(note.ID)
	assert.NoError(t, err)
}

func extractText(content []byte) string {
	var c struct {
		Text string `json:"text"`
	}
	json.Unmarshal(content, &c)
	return c.Text
}
