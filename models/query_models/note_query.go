package query_models

type NoteCreator struct {
	Name        string
	Description string
	Tags        []uint
	Groups      []uint
	Resources   []uint
	Meta        string
	StartDate   string
	EndDate     string
	OwnerId     uint
	NoteTypeId  uint
}

type NoteEditor struct {
	NoteCreator
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
	UpdatedBefore   string
	UpdatedAfter    string
	StartDateBefore string
	StartDateAfter  string
	EndDateBefore   string
	EndDateAfter    string
	SortBy          []string
	Ids             []uint
	MetaQuery       []ColumnMeta
	NoteTypeId      uint
	NoteTypeIds     []uint
	Shared          *bool
}

type NoteTypeEditor struct {
	ID            uint
	Name          string
	Description   string
	CustomHeader     string
	CustomSidebar    string
	CustomSummary    string
	CustomAvatar     string
	CustomMRQLResult string
	MetaSchema       string
	SectionConfig    string
}

type NoteTypeQuery struct {
	Name        string
	Description string
}
