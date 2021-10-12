package http_query

type GroupCreator struct {
	Name        string
	Description string
	Tags        []uint
}

type GroupEditor struct {
	GroupCreator
	ID uint
}

type GroupQuery struct {
	Name          string
	Description   string
	Tags          []uint
	Notes         []uint
	CreatedBefore string
	CreatedAfter  string
}
