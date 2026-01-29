package interfaces

import (
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/query_models"
)

type BlockReader interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	GetBlocksForNote(noteID uint) (*[]models.NoteBlock, error)
}

type BlockWriter interface {
	CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error)
	UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error)
	ReorderBlocks(noteID uint, positions map[uint]string) error
}

type BlockStateWriter interface {
	UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error)
}

type BlockDeleter interface {
	DeleteBlock(blockID uint) error
}

type BlockRebalancer interface {
	RebalanceBlockPositions(noteID uint) error
}

// TableBlockQueryRunner combines block reading and query execution for table blocks.
type TableBlockQueryRunner interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	RunReadOnlyQuery(queryId uint, params map[string]any) (*sqlx.Rows, error)
}
