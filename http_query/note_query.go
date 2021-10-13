package http_query

type NoteCreator struct {
	Name        string
	Description string
	Tags        []uint
	Groups      []uint
	Meta        string
	OwnerId     uint
}

type NoteEditor struct {
	NoteCreator
	ID uint
}

type NoteQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	Groups        []uint
	Tags          []uint
	CreatedBefore string
	CreatedAfter  string
}
