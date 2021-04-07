package http_query

type ResourceCreator struct {
}

type ResourceQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	People        []uint
	Tags          []uint
	Albums        []uint
	CreatedBefore string
	CreatedAfter  string
	HasThumbnail  bool
}
