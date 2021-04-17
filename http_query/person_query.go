package http_query

type PersonCreator struct {
	Name        string
	Surname     string
	Description string
	Tags        []uint
}

type PersonQuery struct {
	Name          string
	Surname       string
	Description   string
	Tags          []uint
	Albums        []uint
	CreatedBefore string
	CreatedAfter  string
}
