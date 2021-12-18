package query_models

type ResourceQueryBase struct {
	Name            string
	Description     string
	OwnerId         uint
	Groups          []uint
	Tags            []uint
	Notes           []uint
	Meta            string
	ContentCategory string
	Category        string
}

type ResourceCreator struct {
	ResourceQueryBase
}

type ResourceFromLocalCreator struct {
	ResourceQueryBase
	LocalPath string
	PathName  string
}

type ResourceFromRemoteCreator struct {
	ResourceQueryBase
	URL               string
	FileName          string
	GroupCategoryName string
	GroupName         string
}

type ResourceEditor struct {
	ResourceQueryBase
	ID uint
}

type ResourceSearchQuery struct {
	Name          string
	Description   string
	OwnerId       uint
	Groups        []uint
	Tags          []uint
	Notes         []uint
	CreatedBefore string
	CreatedAfter  string
	MetaQuery     []ColumnMeta
	SortBy        string
}

type ResourceThumbnailQuery struct {
	ID     uint
	Width  uint
	Height uint
}
