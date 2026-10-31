package query_models

// TemplateRevisionLabelRequest is the body of the label endpoint. An empty
// Label clears the label; at most 200 characters.
type TemplateRevisionLabelRequest struct {
	Label string `json:"label" schema:"label"`
}

// TemplateRevisionRestoreRequest is the body of the restore endpoint.
type TemplateRevisionRestoreRequest struct {
	// Slot restores only that slot (for example "CustomHeader"). Empty restores
	// the whole revision.
	Slot string `json:"slot" schema:"slot"`
	// IncludeCss brings the slot's CSS sibling along. Defaults to true; only
	// meaningful with Slot.
	IncludeCss *bool `json:"includeCss" schema:"includeCss"`
}
