package query_models

type ResourceQueryBase struct {
	Name               string
	Description        string
	OwnerId            uint
	Groups             []uint
	Tags               []uint
	Notes              []uint
	Meta               string
	ContentCategory    string
	Category           string
	ResourceCategoryId uint
	OriginalName       string
	OriginalLocation   string
	Width              uint
	Height             uint
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
	GroupMeta         string
}

type ResourceEditor struct {
	ResourceQueryBase
	ID uint
}

type ResourceSearchQuery struct {
	Name               string
	Description        string
	ContentType        string
	OwnerId            uint
	ResourceCategoryId uint
	Groups             []uint
	Tags               []uint
	Notes              []uint
	Ids              []uint
	CreatedBefore    string
	CreatedAfter     string
	MetaQuery        []ColumnMeta
	SortBy           []string
	MaxResults       uint
	OriginalName     string
	OriginalLocation string
	Hash             string
	ShowWithoutOwner bool
	ShowWithSimilar  bool
	MinWidth         uint
	MinHeight        uint
	MaxWidth         uint
	MaxHeight        uint
}

type ResourceThumbnailQuery struct {
	ID     uint
	Width  uint
	Height uint
}

type RotateResourceQuery struct {
	ID      uint
	Degrees int
}
