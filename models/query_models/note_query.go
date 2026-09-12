package query_models

type NoteCreator struct {
	Name        string
	Description string
	Tags        []uint
	Groups      []uint
	Resources   []uint
	Meta        string
	StartDate   string
	EndDate     string
	OwnerId     uint
	NoteTypeId  uint
}

type NoteEditor struct {
	NoteCreator
	ID uint
}

type NoteQuery struct {
	Name            string
	Description     string
	OwnerId         uint
	Groups          []uint
	Tags            []uint
	CreatedBefore   string
	CreatedAfter    string
	UpdatedBefore   string
	UpdatedAfter    string
	StartDateBefore string
	StartDateAfter  string
	EndDateBefore   string
	EndDateAfter    string
	SortBy          []string
	Ids             []uint
	MetaQuery       []ColumnMeta
	NoteTypeId      uint
	NoteTypeIds     []uint
	Shared          *bool
	// MRQL is an optional MRQL filter expression (package 5 list-page bar),
	// parsed with mrql.ParseFilter (type = "note" implied). Empty = no filter.
	MRQL string
}

type NoteTypeEditor struct {
	ID                          uint
	Name                        string
	Description                 string
	CustomHeader                string
	CustomHeaderCSS             string
	CustomSidebar               string
	CustomSidebarCSS            string
	CustomSummary               string
	CustomSummaryCSS            string
	CustomAvatar                string
	CustomAvatarCSS             string
	CustomListHeader            string
	CustomListHeaderCSS         string
	CustomDetailFooter          string
	CustomDetailFooterCSS       string
	CustomListFooter            string
	CustomListFooterCSS         string
	CustomHoverCard             string
	CustomHoverCardCSS          string
	ApplyTemplatesToShares      bool
	CustomMRQLResult            string
	CustomMRQLResultCSS         string
	CustomEntityPickerResult    string
	CustomEntityPickerResultCSS string
	CustomCSS                   string
	MetaSchema                  string
	MetadataIndexes             *string
	SectionConfig               string
}

type NoteTypeQuery struct {
	Name        string
	Description string
}
