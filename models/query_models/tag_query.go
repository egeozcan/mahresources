package query_models

type TagCreator struct {
	Name        string
	Description string
	ID          uint
}

type TagQuery struct {
	Name          string
	Description   string
	CreatedBefore string
	CreatedAfter  string
}
