package http_query

type AlbumCreator struct {
	Name               string
	Description        string
	Tags               []uint
	People             []uint
	Meta               string
	Preview            string
	PreviewContentType string
	OwnerId            uint
}

type AlbumEditor struct {
	AlbumCreator
	ID uint
}

type AlbumQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	People        []uint
	Tags          []uint
	CreatedBefore string
	CreatedAfter  string
	HasThumbnail  bool
}
