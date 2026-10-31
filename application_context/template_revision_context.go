package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/contracts"
	"mahresources/models"
)

// Template Revisions: every save that changes a Template Carrier's tracked
// fields records one. The design is in docs/plans/2026-10-31-template-revisions.md
// and docs/adr/0008-template-revisions-are-json-snapshots-in-one-table.md.

// ErrTemplateRevisionNotFound is returned when a carrier has no such revision.
var ErrTemplateRevisionNotFound = errors.New("template revision not found")

// ErrTemplateRevisionInvalid is returned when a revision cannot be restored or
// labelled as asked. The message names the offending field or limit.
var ErrTemplateRevisionInvalid = errors.New("template revision rejected")

// clientHeader is the advisory header the mr CLI sends, so a save made through
// it is recorded with source "cli". It is provenance, never an authorization input.
const clientHeader = "X-Mahresources-Client"

// revisionSourceForRequest classifies an HTTP request for the revision it may
// record: a browser form (the request accepts HTML, the test
// http_utils.RequestAcceptsHTML applies), the mr CLI, or any other API client.
func revisionSourceForRequest(r *http.Request) string {
	for _, accept := range r.Header["Accept"] {
		if strings.Contains(accept, "text/html") {
			return models.RevisionSourceForm
		}
	}
	if r.Header.Get(clientHeader) == "mr" {
		return models.RevisionSourceCLI
	}
	return models.RevisionSourceAPI
}

// templateRevisionSource is the source a save on this context records.
func (ctx *MahresourcesContext) templateRevisionSource() string {
	if ctx.revisionSource != "" {
		return ctx.revisionSource
	}
	return models.RevisionSourceAPI
}

// requireTemplateCarrierEdit demands the carrier's edit capability: admin for
// Categories and Resource Categories, editor for Note Types. Viewing history
// needs it too, because the edit pages are deliberately readable by lower
// roles and a revision exposes content the author may have meant to remove.
func (ctx *MahresourcesContext) requireTemplateCarrierEdit(carrierType, op string) error {
	switch carrierType {
	case models.TemplateCarrierCategory, models.TemplateCarrierResourceCategory:
		return ctx.requireTaxonomyRole(op)
	case models.TemplateCarrierNoteType:
		return ctx.requireEditorRole(op)
	}
	return fmt.Errorf("%w: unknown carrier type %q", ErrTemplateRevisionInvalid, carrierType)
}

func newTemplateCarrier(carrierType string) any {
	switch carrierType {
	case models.TemplateCarrierCategory:
		return &models.Category{}
	case models.TemplateCarrierResourceCategory:
		return &models.ResourceCategory{}
	case models.TemplateCarrierNoteType:
		return &models.NoteType{}
	}
	return nil
}

// lockTemplateCarrier reads a carrier inside tx and, on Postgres, holds its row
// for the transaction. Save, restore and delete all start here: the pre-save
// state is read under the lock, so two concurrent changing saves serialise and
// the second one's baseline is the first one's content. SQLite needs nothing
// more, because the server driver begins every write transaction IMMEDIATE.
func lockTemplateCarrier(tx *gorm.DB, dest any, id uint) error {
	q := tx
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q.First(dest, id).Error
}

// recordTemplateRevision writes the revision a save owes, inside tx. A nil
// error with no revision means the save changed no tracked field.
func (ctx *MahresourcesContext) recordTemplateRevision(tx *gorm.DB, kind, carrierType string, carrierID uint, before, after models.TemplateSnapshot, applied []string) (*models.TemplateRevision, error) {
	return models.RecordTemplateRevision(tx, models.RecordTemplateRevisionInput{
		Kind:        kind,
		CarrierType: carrierType,
		CarrierID:   carrierID,
		Before:      before,
		After:       after,
		Source:      ctx.templateRevisionSource(),
		Applied:     models.SanitizeRevisionApplied(carrierType, applied),
	})
}

// validateCarrierTemplates runs the ordinary save validation over a target
// state. Restore never bypasses it: a revision whose raw content no longer
// validates is refused, naming the field, and the user repairs it in the editor.
func validateCarrierTemplates(carrierType string, snap models.TemplateSnapshot) error {
	if s, _ := snap["MetaSchema"].(string); s != "" {
		if err := ValidateMetaSchema(s); err != nil {
			return fmt.Errorf("%w: MetaSchema is invalid: %v", ErrTemplateRevisionInvalid, err)
		}
	}
	if carrierType == models.TemplateCarrierResourceCategory {
		if s, _ := snap["AutoDetectRules"].(string); s != "" {
			if err := ValidateAutoDetectRules(s); err != nil {
				return fmt.Errorf("%w: AutoDetectRules is invalid: %v", ErrTemplateRevisionInvalid, err)
			}
		}
	}
	return nil
}

func carrierName(carrier any) string {
	return reflect.ValueOf(carrier).Elem().FieldByName("Name").String()
}

func summarizeTemplateRevision(rev *models.TemplateRevision) contracts.TemplateRevisionSummary {
	applied := []string{}
	_ = json.Unmarshal([]byte(rev.Applied), &applied)
	changed := []string{}
	_ = json.Unmarshal([]byte(rev.ChangedSlots), &changed)
	if applied == nil {
		applied = []string{}
	}
	if changed == nil {
		changed = []string{}
	}
	return contracts.TemplateRevisionSummary{
		ID:              rev.ID,
		Number:          rev.Number,
		CarrierType:     rev.CarrierType,
		CarrierID:       rev.CarrierID,
		CreatedAt:       rev.CreatedAt,
		CreatedByUserId: rev.CreatedByUserId,
		Label:           rev.Label,
		Source:          rev.Source,
		Applied:         applied,
		ChangedSlots:    changed,
		RestoredFrom:    rev.RestoredFrom,
	}
}

// requireCarrierExists answers a missing carrier with a not-found error rather
// than an empty history.
func (ctx *MahresourcesContext) requireCarrierExists(carrierType string, id uint) error {
	return ctx.db.Select("id").First(newTemplateCarrier(carrierType), id).Error
}

func (ctx *MahresourcesContext) findTemplateRevision(db *gorm.DB, carrierType string, id, number uint) (*models.TemplateRevision, error) {
	var rev models.TemplateRevision
	err := db.Where("carrier_type = ? AND carrier_id = ? AND number = ?", carrierType, id, number).First(&rev).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: #%d", ErrTemplateRevisionNotFound, number)
	}
	return &rev, err
}

// ListTemplateRevisions returns a carrier's revisions, newest first, without
// their content.
func (ctx *MahresourcesContext) ListTemplateRevisions(carrierType string, carrierID uint) ([]contracts.TemplateRevisionSummary, error) {
	if err := ctx.requireTemplateCarrierEdit(carrierType, "listing template revisions"); err != nil {
		return nil, err
	}
	if err := ctx.requireCarrierExists(carrierType, carrierID); err != nil {
		return nil, err
	}
	var revs []models.TemplateRevision
	err := ctx.db.Omit("snapshot").
		Where("carrier_type = ? AND carrier_id = ?", carrierType, carrierID).
		Order("number DESC").Find(&revs).Error
	if err != nil {
		return nil, err
	}
	out := make([]contracts.TemplateRevisionSummary, len(revs))
	for i := range revs {
		out[i] = summarizeTemplateRevision(&revs[i])
	}
	return out, nil
}

// GetTemplateRevision returns one revision with its content.
func (ctx *MahresourcesContext) GetTemplateRevision(carrierType string, carrierID uint, number uint) (*contracts.TemplateRevisionDetail, error) {
	if err := ctx.requireTemplateCarrierEdit(carrierType, "reading a template revision"); err != nil {
		return nil, err
	}
	if err := ctx.requireCarrierExists(carrierType, carrierID); err != nil {
		return nil, err
	}
	rev, err := ctx.findTemplateRevision(ctx.db, carrierType, carrierID, number)
	if err != nil {
		return nil, err
	}
	snap, err := models.ParseTemplateSnapshot(rev.Snapshot)
	if err != nil {
		return nil, err
	}
	editor := map[string]any{}
	for _, f := range models.TemplateFields(carrierType) {
		switch f.Kind {
		case models.TemplateFieldBool:
			b, _ := snap[f.Name].(bool)
			editor[f.Name] = b
		case models.TemplateFieldSectionConfig:
			editor[f.Name] = models.ResolvedSectionConfigJSON(carrierType, snap)
		default:
			s, _ := snap[f.Name].(string)
			editor[f.Name] = s
		}
	}
	return &contracts.TemplateRevisionDetail{
		TemplateRevisionSummary: summarizeTemplateRevision(rev),
		Snapshot:                snap,
		Editor:                  editor,
	}, nil
}

// DiffTemplateRevisions diffs revision from against revision to, or against the
// carrier's current state when to is nil. The server computes the diff and the
// UI renders it.
func (ctx *MahresourcesContext) DiffTemplateRevisions(carrierType string, carrierID uint, from uint, to *uint) (*contracts.TemplateRevisionDiff, error) {
	if err := ctx.requireTemplateCarrierEdit(carrierType, "diffing template revisions"); err != nil {
		return nil, err
	}
	carrier := newTemplateCarrier(carrierType)
	if err := ctx.db.First(carrier, carrierID).Error; err != nil {
		return nil, err
	}
	fromRev, err := ctx.findTemplateRevision(ctx.db, carrierType, carrierID, from)
	if err != nil {
		return nil, err
	}
	fromSnap, err := models.ParseTemplateSnapshot(fromRev.Snapshot)
	if err != nil {
		return nil, err
	}
	toSnap := models.NewTemplateSnapshot(carrierType, carrier)
	if to != nil {
		toRev, err := ctx.findTemplateRevision(ctx.db, carrierType, carrierID, *to)
		if err != nil {
			return nil, err
		}
		if toSnap, err = models.ParseTemplateSnapshot(toRev.Snapshot); err != nil {
			return nil, err
		}
	}
	return &contracts.TemplateRevisionDiff{
		CarrierType: carrierType,
		CarrierID:   carrierID,
		FromNumber:  from,
		ToNumber:    to,
		Slots:       models.DiffTemplateSnapshots(carrierType, fromSnap, toSnap),
	}, nil
}

// LabelTemplateRevision sets, changes or clears a revision's label, the only
// mutable part of a revision.
func (ctx *MahresourcesContext) LabelTemplateRevision(carrierType string, carrierID uint, number uint, label string) (*contracts.TemplateRevisionSummary, error) {
	if err := ctx.requireTemplateCarrierEdit(carrierType, "labelling a template revision"); err != nil {
		return nil, err
	}
	label = strings.TrimSpace(label)
	if n := utf8.RuneCountInString(label); n > models.RevisionLabelMaxLength {
		return nil, fmt.Errorf("%w: label is %d characters, the limit is %d", ErrTemplateRevisionInvalid, n, models.RevisionLabelMaxLength)
	}
	if err := ctx.requireCarrierExists(carrierType, carrierID); err != nil {
		return nil, err
	}
	rev, err := ctx.findTemplateRevision(ctx.db, carrierType, carrierID, number)
	if err != nil {
		return nil, err
	}
	if err := ctx.db.Model(&models.TemplateRevision{}).Where("id = ?", rev.ID).Update("label", label).Error; err != nil {
		return nil, err
	}
	rev.Label = label
	summary := summarizeTemplateRevision(rev)
	return &summary, nil
}

// RestoreTemplateRevision makes a revision's content, or one slot of it, the
// carrier's current templates and records a new revision for it. It moves
// forward: later revisions stay.
//
// It does not go through the shared save function's field mapping, because
// there an empty SectionConfig means "keep current" and a revision with no
// section config could never be restored. It builds the exact target state and
// persists that, under the same carrier lock and with the same validation as a
// normal save. A refused restore writes nothing.
func (ctx *MahresourcesContext) RestoreTemplateRevision(carrierType string, carrierID uint, number uint, slot *string, includeCSS bool) (*contracts.TemplateRevisionSummary, error) {
	if err := ctx.requireTemplateCarrierEdit(carrierType, "restoring a template revision"); err != nil {
		return nil, err
	}
	var slotDef models.TemplateSlot
	if slot != nil {
		var ok bool
		if slotDef, ok = models.FindTemplateSlot(carrierType, *slot); !ok {
			return nil, fmt.Errorf("%w: %q is not a template slot of a %s", ErrTemplateRevisionInvalid, *slot, carrierType)
		}
	}

	var carrier any
	var hookEvent string
	if carrierType == models.TemplateCarrierCategory {
		hookEvent = "before_category_update"
		var probe models.Category
		if err := ctx.db.First(&probe, carrierID).Error; err != nil {
			return nil, err
		}
		if _, err := ctx.RunBeforePluginHooks(hookEvent, map[string]any{
			"id": float64(probe.ID), "name": probe.Name, "description": probe.Description,
		}); err != nil {
			return nil, err
		}
	}

	var recorded *models.TemplateRevision
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		carrier = newTemplateCarrier(carrierType)
		if err := lockTemplateCarrier(tx, carrier, carrierID); err != nil {
			return err
		}
		rev, err := ctx.findTemplateRevision(tx, carrierType, carrierID, number)
		if err != nil {
			return err
		}
		target, err := models.ParseTemplateSnapshot(rev.Snapshot)
		if err != nil {
			return err
		}
		before := models.NewTemplateSnapshot(carrierType, carrier)
		if slot == nil {
			models.ApplyTemplateSnapshot(carrierType, carrier, target)
		} else {
			models.ApplyTemplateSlot(carrier, slotDef, target, includeCSS)
		}
		after := models.NewTemplateSnapshot(carrierType, carrier)
		if err := validateCarrierTemplates(carrierType, after); err != nil {
			return fmt.Errorf("cannot restore revision #%d: %w", number, err)
		}
		if err := tx.Save(carrier).Error; err != nil {
			return err
		}
		restoredFrom := number
		recorded, err = models.RecordTemplateRevision(tx, models.RecordTemplateRevisionInput{
			Kind:         models.RevisionKindRestore,
			CarrierType:  carrierType,
			CarrierID:    carrierID,
			Before:       before,
			After:        after,
			Source:       models.RevisionSourceRestore,
			RestoredFrom: &restoredFrom,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	id := carrierID
	ctx.Logger().Info(models.LogActionUpdate, carrierType, &id, carrierName(carrier),
		fmt.Sprintf("Restored template revision #%d", number),
		map[string]interface{}{"fromRevision": number, "toRevision": recorded.Number})
	if carrierType == models.TemplateCarrierCategory {
		c := carrier.(*models.Category)
		ctx.RunAfterPluginHooks("after_category_update", map[string]any{
			"id": float64(c.ID), "name": c.Name, "description": c.Description,
		})
	}
	summary := summarizeTemplateRevision(recorded)
	return &summary, nil
}
