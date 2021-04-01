package query

type AlbumCreator struct {
	Name               string
	Meta               string
	Preview            string
	PreviewContentType string
	OwnerId            uint
}

type AlbumQuery struct {
	Name    string
	OwnerId uint
	Tags    []uint
}
