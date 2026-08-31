package contracts

import "mahresources/models/query_models"

// MassEditOpResult reports what one op of a mass edit did.
//
// rowsAffected means join rows for relation ops (a tag added to N resources
// counts N) and entity rows for owner and meta ops, hence the qualified op
// names ("tags.add" rather than "add").
type MassEditOpResult struct {
	Op           string `json:"op"`
	RowsAffected int64  `json:"rowsAffected"`
}

// MassEditResult is the response of every mass-edit endpoint. matched is what
// targeting resolved to (the number ExpectedCount was compared against);
// affected is the number of entities the ops were applied to.
type MassEditResult struct {
	Entity   string             `json:"entity"`
	Matched  int64              `json:"matched"`
	Affected int64              `json:"affected"`
	Ops      []MassEditOpResult `json:"ops"`
	DryRun   bool               `json:"dryRun"`
}

// MassResourceEditor performs one mass edit over resources, in one transaction.
type MassResourceEditor interface {
	MassEditResources(*query_models.MassEditQuery) (*MassEditResult, error)
}

// MassNoteEditor performs one mass edit over notes, in one transaction.
type MassNoteEditor interface {
	MassEditNotes(*query_models.MassEditQuery) (*MassEditResult, error)
}

// MassGroupEditor performs one mass edit over groups, in one transaction.
type MassGroupEditor interface {
	MassEditGroups(*query_models.MassEditQuery) (*MassEditResult, error)
}
