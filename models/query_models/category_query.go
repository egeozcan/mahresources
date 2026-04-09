package query_models

type CategoryCreator struct {
	Name        string
	Description string

	CustomHeader  string
	CustomSidebar string
	CustomSummary string
	CustomAvatar  string
	MetaSchema    string
	SectionConfig string
}

type CategoryEditor struct {
	CategoryCreator
	ID uint
}

type CategoryQuery struct {
	Name          string
	Description   string
	CreatedBefore string
	CreatedAfter  string
	UpdatedBefore string
	UpdatedAfter  string
	SortBy        []string
}
