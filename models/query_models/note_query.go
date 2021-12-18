package query_models

type noteCreator struct {
	Name        string
	Description string
	Tags        []uint
	Groups      []uint
	Resources   []uint
	Meta        string
	StartDate   string
	EndDate     string
	OwnerId     uint
}

type NoteEditor struct {
	noteCreator
	ID uint
}

type NoteQuery struct {
	Name            string
	Description     string
	OwnerId         uint
	Groups          []uint
	Tags            []uint
	CreatedBefore   string
	CreatedAfter    string
	StartDateBefore string
	StartDateAfter  string
	EndDateBefore   string
	EndDateAfter    string
	MetaQuery       []ColumnMeta
}
