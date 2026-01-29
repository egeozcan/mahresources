package query_models

import "encoding/json"

// NoteBlockEditor is used for creating/updating blocks
type NoteBlockEditor struct {
	ID       uint            `schema:"id"`
	NoteID   uint            `schema:"noteId"`
	Type     string          `schema:"type"`
	Position string          `schema:"position"`
	Content  json.RawMessage `schema:"-"`
}

// NoteBlockStateEditor is used for updating block state only
type NoteBlockStateEditor struct {
	ID    uint            `schema:"id"`
	State json.RawMessage `schema:"-"`
}

// NoteBlockReorderEditor is used for batch reordering
type NoteBlockReorderEditor struct {
	NoteID    uint            `json:"noteId"`
	Positions map[uint]string `json:"positions"` // blockId -> new position
}
