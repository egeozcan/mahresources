package application_context

import (
	"encoding/json"
	"errors"
	"log"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// CreateBlock creates a new block in a note
func (ctx *MahresourcesContext) CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error) {
	// Validate block type
	bt := block_types.GetBlockType(editor.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + editor.Type)
	}

	// Validate content or use default
	if len(editor.Content) == 0 {
		editor.Content = bt.DefaultContent()
	} else if err := bt.ValidateContent(editor.Content); err != nil {
		return nil, err
	}

	block := models.NoteBlock{
		NoteID:   editor.NoteID,
		Type:     editor.Type,
		Position: editor.Position,
		Content:  types.JSON(editor.Content),
		State:    types.JSON(bt.DefaultState()),
	}

	if err := ctx.db.Create(&block).Error; err != nil {
		return nil, err
	}

	// Sync first text block to note description
	if editor.Type == "text" {
		ctx.syncFirstTextBlockToDescription(editor.NoteID)
	}

	return &block, nil
}

// GetBlock retrieves a single block by ID
func (ctx *MahresourcesContext) GetBlock(id uint) (*models.NoteBlock, error) {
	var block models.NoteBlock
	return &block, ctx.db.First(&block, id).Error
}

// GetBlocksForNote retrieves all blocks for a note, ordered by position
func (ctx *MahresourcesContext) GetBlocksForNote(noteID uint) (*[]models.NoteBlock, error) {
	var blocks []models.NoteBlock
	err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error
	return &blocks, err
}

// UpdateBlockContent updates a block's content
func (ctx *MahresourcesContext) UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate content against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateContent(content); err != nil {
		return nil, err
	}

	block.Content = types.JSON(content)
	if err := ctx.db.Save(&block).Error; err != nil {
		return nil, err
	}

	// Sync first text block to note description
	if block.Type == "text" {
		ctx.syncFirstTextBlockToDescription(block.NoteID)
	}

	return &block, nil
}

// UpdateBlockState updates a block's state (for UI state like checked items)
func (ctx *MahresourcesContext) UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate state against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateState(state); err != nil {
		return nil, err
	}

	block.State = types.JSON(state)
	return &block, ctx.db.Save(&block).Error
}

// DeleteBlock removes a block
func (ctx *MahresourcesContext) DeleteBlock(blockID uint) error {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return err
	}

	noteID := block.NoteID
	isText := block.Type == "text"

	if err := ctx.db.Delete(&block).Error; err != nil {
		return err
	}

	// Sync first text block to note description
	if isText {
		ctx.syncFirstTextBlockToDescription(noteID)
	}

	return nil
}

// ReorderBlocks updates positions for multiple blocks in a single transaction
func (ctx *MahresourcesContext) ReorderBlocks(noteID uint, positions map[uint]string) error {
	if len(positions) == 0 {
		return nil
	}

	// Collect block IDs for validation
	blockIDs := make([]uint, 0, len(positions))
	for blockID := range positions {
		blockIDs = append(blockIDs, blockID)
	}

	// Verify all blocks belong to the specified note
	var count int64
	if err := ctx.db.Model(&models.NoteBlock{}).Where("id IN ? AND note_id = ?", blockIDs, noteID).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(positions) {
		return errors.New("one or more block IDs do not belong to the specified note")
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for blockID, position := range positions {
			if err := tx.Model(&models.NoteBlock{}).Where("id = ? AND note_id = ?", blockID, noteID).
				Update("position", position).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// syncFirstTextBlockToDescription syncs the first text block's content to the note's Description
func (ctx *MahresourcesContext) syncFirstTextBlockToDescription(noteID uint) {
	var blocks []models.NoteBlock
	if err := ctx.db.Where("note_id = ? AND type = ?", noteID, "text").
		Order("position ASC").Limit(1).Find(&blocks).Error; err != nil {
		log.Printf("Warning: failed to query text blocks for note %d: %v", noteID, err)
		return
	}

	if len(blocks) == 0 {
		return
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(blocks[0].Content, &content); err != nil {
		log.Printf("Warning: failed to unmarshal text block content for note %d: %v", noteID, err)
		return
	}

	if err := ctx.db.Model(&models.Note{}).Where("id = ?", noteID).Update("description", content.Text).Error; err != nil {
		log.Printf("Warning: failed to sync description for note %d: %v", noteID, err)
	}
}
