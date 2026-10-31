package query_models

// TemplateRevisionDiffQuery selects the two sides of a revision diff. From is a
// revision number; To is a revision number, or omitted for the carrier's
// current state.
type TemplateRevisionDiffQuery struct {
	From uint
	To   *uint
}

// TemplateRevisionLabelRequest is the body of the label endpoint. An empty
// Label clears the label; at most 200 characters.
type TemplateRevisionLabelRequest struct {
	Label string
}

// TemplateRevisionRestoreRequest is the body of the restore endpoint.
type TemplateRevisionRestoreRequest struct {
	// Slot restores only that slot (for example "CustomHeader"). Empty restores
	// the whole revision.
	Slot string
	// IncludeCss brings the slot's CSS sibling along. Defaults to true; only
	// meaningful with Slot.
	IncludeCss *bool
}
