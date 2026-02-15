package query_models

type SeriesQuery struct {
	Name          string
	Slug          string
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string
}

type SeriesEditor struct {
	ID   uint
	Name string
	Meta string
}

type SeriesCreator struct {
	Name string
}
