package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type NoteReader interface {
	GetNotes(offset, maxResults int, query *query_models.NoteQuery) ([]models.Note, error)
	GetNote(id uint) (*models.Note, error)
}

type NoteWriter interface {
	CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error)
	BulkNoteTagEditor
	BulkNoteGroupEditor
	BulkNoteMetaEditor
}

// NoteWriteReader combines writing with reading for partial-update support
type NoteWriteReader interface {
	NoteWriter
	GetNote(id uint) (*models.Note, error)
}

type NoteDeleter interface {
	DeleteNote(noteId uint) error
	BulkDeleteNotes(query *query_models.BulkQuery) error
}

type NoteTypeWriter interface {
	CreateOrUpdateNoteType(query *query_models.NoteTypeEditor) (*models.NoteType, error)
	GetNoteType(id uint) (*models.NoteType, error)
}

type NoteTypeDeleter interface {
	DeleteNoteType(noteTypeId uint) error
}

// NoteMetaReader provides access to note metadata keys
type NoteMetaReader interface {
	NoteMetaKeys() ([]MetaKey, error)
}

// NoteTypeReader provides read access to note types
type NoteTypeReader interface {
	GetNoteTypes(query *query_models.NoteTypeQuery, offset, maxResults int) ([]models.NoteType, error)
}

// NoteSharer provides note sharing operations via share tokens
type NoteSharer interface {
	ShareNote(noteId uint) (string, error)
	UnshareNote(noteId uint) error
	GetNoteByShareToken(token string) (*models.Note, error)
}

// BulkNoteTagEditor handles bulk tag operations on notes
type BulkNoteTagEditor interface {
	BulkAddTagsToNotes(query *query_models.BulkEditQuery) error
	BulkRemoveTagsFromNotes(query *query_models.BulkEditQuery) error
}

// BulkNoteGroupEditor handles bulk group operations on notes
type BulkNoteGroupEditor interface {
	BulkAddGroupsToNotes(query *query_models.BulkEditQuery) error
}

// BulkNoteMetaEditor handles bulk meta operations on notes
type BulkNoteMetaEditor interface {
	BulkAddMetaToNotes(query *query_models.BulkEditMetaQuery) error
}
