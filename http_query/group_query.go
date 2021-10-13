package http_query

type GroupCreator struct {
	Name        string
	Description string
	Tags        []uint
	CategoryId  uint
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
	CategoryId    uint
	CreatedBefore string
	CreatedAfter  string
}
