package http_query

type ResourceQueryBase struct {
	Name            string
	Description     string
	Groups          []uint
	Tags            []uint
	Albums          []uint
	Meta            string
	ContentCategory string
	Category        string
	OwnerId         uint
}

type ResourceCreator struct {
	ResourceQueryBase
	Name               string
	Preview            string
	PreviewContentType string
}

type ResourceEditor struct {
	ResourceQueryBase
	ID uint
}

type ResourceQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	Groups        []uint
	Tags          []uint
	Albums        []uint
	CreatedBefore string
	CreatedAfter  string
	HasThumbnail  bool
}
