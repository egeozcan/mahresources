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
	SeriesSlug         string
	SeriesId           uint
}

type ResourceCreator struct {
	ResourceQueryBase
	PathName string // BH-023: optional alt-fs key; empty = default filesystem
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
	PathName          string // BH-023: optional alt-fs key; empty = default filesystem
}

type ResourceEditor struct {
	ResourceQueryBase
	ID uint
}

type ResourceSearchQuery struct {
	Name               string
	Description        string
	ContentType        string
	ContentTypes       []string
	OwnerId            uint
	ResourceCategoryId uint
	Groups             []uint
	Tags               []uint
	Notes              []uint
	Ids              []uint
	CreatedBefore    string
	CreatedAfter     string
	UpdatedBefore    string
	UpdatedAfter     string
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
	// BH-037: filter resources whose perceptual DHash is zero — these are
	// usually BH-018 solid-colour images that pollute similarity matches.
	// The admin-overview drill-down links here.
	ShowDhashZero bool
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

type CropResourceQuery struct {
	ID      uint
	X       int
	Y       int
	Width   int
	Height  int
	Comment string
}
