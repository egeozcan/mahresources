package query_models

type resourceQueryBase struct {
	Name            string
	Description     string
	Groups          []uint
	Tags            []uint
	Notes           []uint
	Meta            string
	ContentCategory string
	Category        string
	OwnerId         uint
}

type ResourceCreator struct {
	resourceQueryBase
	Name string
}

type ResourceEditor struct {
	resourceQueryBase
	ID uint
}

type ResourceQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	Groups        []uint
	Tags          []uint
	Notes         []uint
	CreatedBefore string
	CreatedAfter  string
	MetaQuery     []ColumnMeta
}

type ResourceThumbnailQuery struct {
	ID     uint
	Width  uint
	Height uint
}
