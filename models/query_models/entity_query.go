package query_models

type EntityIdQuery struct {
	ID uint
}

type BulkQuery struct {
	ID []uint
}

type BulkEditQuery struct {
	BulkQuery
	EditedId []uint
}

type BulkEditMetaQuery struct {
	BulkQuery
	Meta string
}

type MergeQuery struct {
	Winner uint
	Losers []uint
}
