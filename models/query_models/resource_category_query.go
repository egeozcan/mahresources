package query_models

type ResourceCategoryCreator struct {
	Name        string
	Description string

	CustomHeader  string
	CustomSidebar string
	CustomSummary string
	CustomAvatar  string
	MetaSchema    string
}

type ResourceCategoryEditor struct {
	ResourceCategoryCreator
	ID uint
}

type ResourceCategoryQuery struct {
	Name        string
	Description string
}
