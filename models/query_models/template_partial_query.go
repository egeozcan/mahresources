package query_models

// TemplatePartialEditor is the create/update DTO for a TemplatePartial. A zero
// ID creates; a non-zero ID updates the existing partial.
type TemplatePartialEditor struct {
	ID          uint
	Name        string
	Description string
	Content     string
}

// TemplatePartialQuery filters the template-partial list by name/description.
type TemplatePartialQuery struct {
	Name        string
	Description string
}
