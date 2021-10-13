package http_query

type CategoryCreator struct {
	Name        string
	Description string
}

type CategoryEditor struct {
	CategoryCreator
	ID uint
}

type CategoryQuery struct {
	Name        string
	Description string
}
