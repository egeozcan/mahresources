package query_models

type QueryCreator struct {
	Name     string
	Text     string
	Template string
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
