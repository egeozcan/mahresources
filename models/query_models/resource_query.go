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
	PathName  string // BH-023: optional alt-fs key; empty = default filesystem
}

type ResourceFromRemoteCreator struct {
	ResourceQueryBase
	URL               string
	FileName          string
	GroupCategoryName string
	GroupName         string
	GroupMeta         string
	PathName          string // BH-023: optional alt-fs key; empty = default filesystem
	// Headers are extra request headers this one download sends, on top of the
	// deployment's User-Agent. They exist because a media endpoint may want a
	// Referer or a Cookie that no deployment-wide setting could carry.
	//
	// JSON only (`schema:"-"`): gorilla/schema decodes no map field, so a form
	// post cannot set one, and pretending otherwise would half-support the
	// create-resource form. They are persisted verbatim on the download
	// history row -- which is what makes a retry replay them -- so a Cookie
	// put here is stored in the database; the payload is never rendered to a
	// page.
	//
	// They are sent only to the submitted URL's own host. See hostfetch.
	Headers map[string]string `schema:"-"`
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
	// SeriesId restricts results to one series. It lives on ResourceQueryBase
	// (the create/edit shape) too, and its absence here meant the schema decoder
	// silently dropped ?seriesId= on every list request.
	SeriesId         uint
	Groups           []uint
	Tags             []uint
	Notes            []uint
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
	// Untagged restricts results to resources with zero rows in resource_tags.
	// Powers the lightbox "Tag untagged" launcher.
	Untagged bool
	// IncludeSubgroups widens the OwnerId filter to the whole group subtree
	// (owner and all descendant subgroups, recursively). No-op when OwnerId is 0.
	IncludeSubgroups bool
	// MRQL is an optional MRQL filter expression (package 5 list-page bar). It is
	// parsed with mrql.ParseFilter (WHERE-clause grammar only, type = "resource"
	// implied) and composed as an id-membership predicate. Empty = no MRQL filter.
	MRQL string
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
	// AsNewResource saves the crop as a separate resource and leaves the source
	// untouched. Omitted (false) keeps the historical behaviour: the crop becomes
	// a new version of the source.
	AsNewResource bool
}

type TrimVideoQuery struct {
	ID      uint
	Start   string
	End     string
	Comment string
}
