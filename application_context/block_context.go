package application_context

import (
	"encoding/json"
	"errors"
	"log"

	"gorm.io/gorm"
	"mahresources/lib"
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

	// Use transaction to ensure atomicity of block creation and description sync
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if editor.Type == "text" {
			if err := syncFirstTextBlockToDescriptionTx(tx, editor.NoteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", editor.NoteID, err)
				// Don't fail the transaction for sync errors
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
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

	// Use transaction to ensure atomicity of content update and description sync
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if block.Type == "text" {
			if err := syncFirstTextBlockToDescriptionTx(tx, block.NoteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", block.NoteID, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
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

	// Use transaction to ensure atomicity of deletion and description sync
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if isText {
			if err := syncFirstTextBlockToDescriptionTx(tx, noteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", noteID, err)
			}
		}
		return nil
	})
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

// syncFirstTextBlockToDescriptionTx syncs the first text block's content to the note's Description
// within an existing transaction. This ensures atomicity between block operations and description sync.
func syncFirstTextBlockToDescriptionTx(tx *gorm.DB, noteID uint) error {
	var blocks []models.NoteBlock
	if err := tx.Where("note_id = ? AND type = ?", noteID, "text").
		Order("position ASC").Limit(1).Find(&blocks).Error; err != nil {
		return err
	}

	if len(blocks) == 0 {
		return nil
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(blocks[0].Content, &content); err != nil {
		return err
	}

	return tx.Model(&models.Note{}).Where("id = ?", noteID).Update("description", content.Text).Error
}

// RebalanceBlockPositions normalizes block positions for a note to prevent position string growth.
// This reassigns positions using evenly distributed values (e.g., "d", "h", "l", "p", "t").
// Call this periodically or when positions become too long.
func (ctx *MahresourcesContext) RebalanceBlockPositions(noteID uint) error {
	var blocks []models.NoteBlock
	if err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error; err != nil {
		return err
	}

	if len(blocks) == 0 {
		return nil
	}

	// Generate evenly distributed positions
	newPositions := lib.GenerateEvenPositions(len(blocks))

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for i, block := range blocks {
			if err := tx.Model(&models.NoteBlock{}).Where("id = ?", block.ID).
				Update("position", newPositions[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
