package models

import "time"

// Template Carrier types. A Template Revision names its carrier by one of
// these plus the carrier's ID, so one table serves all three.
const (
	TemplateCarrierCategory         = "category"
	TemplateCarrierResourceCategory = "resourceCategory"
	TemplateCarrierNoteType         = "noteType"
)

// Template Revision sources: what produced the save.
const (
	RevisionSourceCreate   = "create"
	RevisionSourceForm     = "form"
	RevisionSourceAPI      = "api"
	RevisionSourceCLI      = "cli"
	RevisionSourcePlugin   = "plugin"
	RevisionSourceImport   = "import"
	RevisionSourceRestore  = "restore"
	RevisionSourceBaseline = "baseline"
)

// RevisionLabelMaxLength bounds TemplateRevision.Label.
const RevisionLabelMaxLength = 200

// TemplateRevision is one saved state of a Template Carrier's whole tracked
// template set (see TemplateSlots). Its content never changes after it is
// written; only Label may be edited.
//
// Snapshot is a JSON object of the raw field values keyed by Go field name,
// stored as text so the bytes survive the database unchanged (a JSONB column
// would reorder keys). The decision is recorded in
// docs/adr/0008-template-revisions-are-json-snapshots-in-one-table.md.
type TemplateRevision struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"createdAt"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`

	CarrierType string `gorm:"size:32;not null;uniqueIndex:unique_template_revision_number,priority:1" json:"carrierType"`
	CarrierID   uint   `gorm:"not null;uniqueIndex:unique_template_revision_number,priority:2" json:"carrierId"`
	Number      uint   `gorm:"not null;uniqueIndex:unique_template_revision_number,priority:3" json:"number"`

	// Snapshot, Applied and ChangedSlots hold JSON documents as plain strings.
	// types.JSON would migrate as JSONB on Postgres whatever the gorm tag says,
	// and JSONB reorders keys and renormalises numbers.
	Snapshot     string `gorm:"type:text" json:"-"`
	Source       string `gorm:"size:16;not null" json:"source"`
	Applied      string `gorm:"type:text" json:"-"`
	ChangedSlots string `gorm:"type:text" json:"-"`
	Label        string `gorm:"size:200" json:"label"`
	RestoredFrom *uint  `json:"restoredFrom,omitempty"`
}
