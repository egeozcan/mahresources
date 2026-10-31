package contracts

import (
	"time"

	"mahresources/models"
)

// TemplateRevisionSummary is a Template Revision without its content: what the
// History list shows.
type TemplateRevisionSummary struct {
	ID              uint      `json:"id"`
	Number          uint      `json:"number"`
	CarrierType     string    `json:"carrierType"`
	CarrierID       uint      `json:"carrierId"`
	CreatedAt       time.Time `json:"createdAt"`
	CreatedByUserId *uint     `json:"createdByUserId,omitempty"`
	Label           string    `json:"label"`
	Source          string    `json:"source"`
	// Applied lists the preset, generated, copied and loaded entries a form
	// save reported, e.g. "preset:dashboard" or "loaded:3:CustomHeader".
	Applied []string `json:"applied"`
	// ChangedSlots names the slots that differ from the state before this save.
	ChangedSlots []string `json:"changedSlots"`
	RestoredFrom *uint    `json:"restoredFrom,omitempty"`
}

// TemplateRevisionDetail is one revision with its content.
type TemplateRevisionDetail struct {
	TemplateRevisionSummary
	// Snapshot holds the raw stored value of every tracked field the revision
	// recorded. A key it lacks is a slot added after the revision was taken.
	Snapshot map[string]any `json:"snapshot"`
	// Editor holds the value of every tracked field as the edit form must be
	// loaded with it: missing keys as empty, and SectionConfig as the Go
	// resolver's output, never the raw snapshot.
	Editor map[string]any `json:"editor"`
}

// TemplateRevisionDiff is a slot-grouped line diff. ToNumber is nil when the
// right side is the carrier's current state.
type TemplateRevisionDiff struct {
	CarrierType string            `json:"carrierType"`
	CarrierID   uint              `json:"carrierId"`
	FromNumber  uint              `json:"from"`
	ToNumber    *uint             `json:"to,omitempty"`
	Slots       []models.SlotDiff `json:"slots"`
}

// TemplateRevisionReader lists, fetches and diffs a carrier's revisions. Every
// method needs the carrier's edit capability, not read access.
type TemplateRevisionReader interface {
	ListTemplateRevisions(carrierType string, carrierID uint) ([]TemplateRevisionSummary, error)
	GetTemplateRevision(carrierType string, carrierID uint, number uint) (*TemplateRevisionDetail, error)
	DiffTemplateRevisions(carrierType string, carrierID uint, from uint, to *uint) (*TemplateRevisionDiff, error)
}

// TemplateRevisionWriter labels and restores revisions.
type TemplateRevisionWriter interface {
	LabelTemplateRevision(carrierType string, carrierID uint, number uint, label string) (*TemplateRevisionSummary, error)
	// RestoreTemplateRevision saves a revision as the carrier's current
	// templates and records a new revision for it. A nil slot restores the
	// whole snapshot; otherwise only that slot, with its CSS when includeCSS.
	RestoreTemplateRevision(carrierType string, carrierID uint, number uint, slot *string, includeCSS bool) (*TemplateRevisionSummary, error)
}
