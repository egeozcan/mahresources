package query_models

type CategoryCreator struct {
	Name        string
	Description string

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
	CustomOwnEntities           string
	CustomOwnEntitiesCSS        string
	CustomMRQLResult            string
	CustomMRQLResultCSS         string
	CustomEntityPickerResult    string
	CustomEntityPickerResultCSS string
	CustomCSS                   string
	MetaSchema                  string
	MetadataIndexes             *string
	SectionConfig               string
	// RevisionApplied lists what the edit form applied before this save (a
	// preset, AI generation, copy from another carrier, a loaded revision).
	// The server keeps only the entries it recognises. See models.SanitizeRevisionApplied.
	RevisionApplied []string `json:"revisionApplied,omitempty" schema:"revisionApplied"`
}

type CategoryEditor struct {
	CategoryCreator
	ID uint
}

type CategoryQuery struct {
	Name          string
	Description   string
	CreatedBefore string
	CreatedAfter  string
	UpdatedBefore string
	UpdatedAfter  string
	SortBy        []string
}
