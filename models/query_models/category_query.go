package query_models

type CategoryCreator struct {
	Name        string
	Description string

	CustomHeader  string
	CustomSidebar string
	CustomSummary string
	CustomAvatar  string
	MetaSchema    string
}

type CategoryEditor struct {
	CategoryCreator
	ID uint
}

type CategoryQuery struct {
	Name        string
	Description string
}
