package query_models

type QueryCreator struct {
	Name      string
	QueryText string
}

type QueryEditor struct {
	QueryCreator
	ID uint
}

type QueryQuery struct {
	Name string
	Text string
}

type QueryParameters = map[string]any
