package http_query

type ResourceCreator struct {
	Name               string
	Description        string
	People             []uint
	Tags               []uint
	Albums             []uint
	Meta               string
	Preview            string
	ContentCategory    string
	Category           string
	PreviewContentType string
}

type ResourceEditor struct {
	ID              uint
	Name            string
	Description     string
	People          []uint
	Tags            []uint
	Albums          []uint
	Meta            string
	ContentCategory string
	Category        string
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
