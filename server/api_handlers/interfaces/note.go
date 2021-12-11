package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type NoteReader interface {
	GetNotes(offset, maxResults int, query *query_models.NoteQuery) (*[]models.Note, error)
	GetNote(id uint) (*models.Note, error)
}

type NoteWriter interface {
	CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error)
}

type NoteDeleter interface {
	DeleteNote(noteId uint) error
}
