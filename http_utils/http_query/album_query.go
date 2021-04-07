package http_query

type AlbumCreator struct {
	Name               string
	Description        string
	Meta               string
	Preview            string
	PreviewContentType string
	OwnerId            uint
}

type AlbumQuery struct {
	Name          string
	OwnerId       uint
	People        []uint
	Tags          []uint
	CreatedBefore string
	CreatedAfter  string
	HasThumbnail  bool
}
