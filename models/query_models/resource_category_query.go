package query_models

type ResourceCategoryCreator struct {
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
	CustomPreview               string
	CustomPreviewCSS            string
	CustomLightbox              string
	CustomLightboxCSS           string
	CustomCell                  string
	CustomCellCSS               string
	CustomMRQLResult            string
	CustomMRQLResultCSS         string
	CustomEntityPickerResult    string
	CustomEntityPickerResultCSS string
	CustomCSS                   string
	MetaSchema                  string
	MetadataIndexes             *string
	AutoDetectRules             string
	SectionConfig               string
}

type ResourceCategoryEditor struct {
	ResourceCategoryCreator
	ID uint
}

type ResourceCategoryQuery struct {
	Name        string
	Description string
}
