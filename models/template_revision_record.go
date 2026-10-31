package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
)

// Recording kinds, from the table in the plan's decision 8.
const (
	// RevisionKindCreate and RevisionKindImport write #1 unconditionally.
	RevisionKindCreate = "create"
	RevisionKindImport = "import"
	// RevisionKindRestore always writes the next number, even when identical.
	RevisionKindRestore = "restore"
	// RevisionKindUpdate writes only when a tracked field changed, preceded by
	// a baseline when the carrier has no revisions yet.
	RevisionKindUpdate = "update"
)

// RecordTemplateRevisionInput describes one save for RecordTemplateRevision.
type RecordTemplateRevisionInput struct {
	Kind        string
	CarrierType string
	CarrierID   uint
	// Before is the carrier's state under the lock, before the save. Unused by
	// create and import.
	Before TemplateSnapshot
	After  TemplateSnapshot
	// Source is stored for create, update and restore. Import always stores
	// "import" and create always "create".
	Source       string
	Applied      []string
	RestoredFrom *uint
}

// RecordTemplateRevision writes the Template Revision a save owes, if any.
//
// tx must be the transaction that saves the carrier, and the caller must hold
// the carrier's row lock (SELECT ... FOR UPDATE on Postgres; SQLite's
// BEGIN IMMEDIATE already serialises writers). Under that lock MAX(number)+1
// cannot collide. The unique index on (carrier type, carrier id, number) is a
// backstop that fails the whole transaction; there is deliberately no retry,
// because a Postgres unique violation aborts the transaction.
//
// It returns the revision written for this save, or nil when the save changed
// no tracked field.
func RecordTemplateRevision(tx *gorm.DB, in RecordTemplateRevisionInput) (*TemplateRevision, error) {
	if TemplateSlots(in.CarrierType) == nil {
		return nil, fmt.Errorf("unknown template carrier type %q", in.CarrierType)
	}
	empty := TemplateSnapshot{}

	switch in.Kind {
	case RevisionKindCreate, RevisionKindImport:
		source := RevisionSourceCreate
		if in.Kind == RevisionKindImport {
			source = RevisionSourceImport
		}
		return writeTemplateRevision(tx, in, 1, source, empty, in.After, nil)

	case RevisionKindRestore:
		n, err := nextTemplateRevisionNumber(tx, in.CarrierType, in.CarrierID)
		if err != nil {
			return nil, err
		}
		return writeTemplateRevision(tx, in, n, RevisionSourceRestore, in.Before, in.After, in.RestoredFrom)

	case RevisionKindUpdate:
		if TemplateSnapshotsEqual(in.CarrierType, in.Before, in.After) {
			return nil, nil
		}
		n, err := nextTemplateRevisionNumber(tx, in.CarrierType, in.CarrierID)
		if err != nil {
			return nil, err
		}
		if n == 1 {
			// No revisions: an exempt or pre-feature carrier. Record what it
			// was, so the first change has something to diff against.
			if _, err := writeTemplateRevision(tx, in, 1, RevisionSourceBaseline, empty, in.Before, nil); err != nil {
				return nil, err
			}
			n = 2
		}
		return writeTemplateRevision(tx, in, n, in.Source, in.Before, in.After, nil)
	}
	return nil, fmt.Errorf("unknown template revision kind %q", in.Kind)
}

func writeTemplateRevision(tx *gorm.DB, in RecordTemplateRevisionInput, number uint, source string, prev, state TemplateSnapshot, restoredFrom *uint) (*TemplateRevision, error) {
	snapshot, err := state.Marshal()
	if err != nil {
		return nil, err
	}
	changed, err := json.Marshal(ChangedTemplateSlots(in.CarrierType, prev, state))
	if err != nil {
		return nil, err
	}
	applied := []string{}
	// A create is the first save of a preset or a generation, so it keeps the
	// form's provenance; a baseline and an import were not produced by a form.
	if source != RevisionSourceBaseline && source != RevisionSourceImport {
		applied = in.Applied
		if applied == nil {
			applied = []string{}
		}
	}
	appliedJSON, err := json.Marshal(applied)
	if err != nil {
		return nil, err
	}
	rev := &TemplateRevision{
		CarrierType:  in.CarrierType,
		CarrierID:    in.CarrierID,
		Number:       number,
		Snapshot:     snapshot,
		Source:       source,
		Applied:      string(appliedJSON),
		ChangedSlots: string(changed),
		RestoredFrom: restoredFrom,
	}
	if err := tx.Create(rev).Error; err != nil {
		return nil, fmt.Errorf("recording template revision #%d: %w", number, err)
	}
	return rev, nil
}

func nextTemplateRevisionNumber(tx *gorm.DB, carrierType string, carrierID uint) (uint, error) {
	var max uint
	err := tx.Model(&TemplateRevision{}).
		Where("carrier_type = ? AND carrier_id = ?", carrierType, carrierID).
		Select("COALESCE(MAX(number), 0)").Scan(&max).Error
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

// DeleteTemplateRevisions removes a carrier's revisions. Delete functions call
// it inside their delete transaction.
func DeleteTemplateRevisions(tx *gorm.DB, carrierType string, carrierID uint) error {
	return tx.Where("carrier_type = ? AND carrier_id = ?", carrierType, carrierID).
		Delete(&TemplateRevision{}).Error
}

// ErrTemplateRevisionNotFound is returned for a revision number a carrier does not have.
var ErrTemplateRevisionNotFound = errors.New("template revision not found")

// CreateTemplateCarrierWithRevision inserts a carrier and its first revision in
// one transaction, for the creators that bypass the shared save functions
// (group import). If the revision insert fails the carrier does not commit, so
// a retry creates both rather than reusing a carrier that has no history.
// carrier is a pointer to a Category, ResourceCategory or NoteType; kind is
// RevisionKindCreate or RevisionKindImport.
func CreateTemplateCarrierWithRevision(db *gorm.DB, carrierType string, carrier any, kind string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(carrier).Error; err != nil {
			return err
		}
		id := uint(reflect.ValueOf(carrier).Elem().FieldByName("ID").Uint())
		_, err := RecordTemplateRevision(tx, RecordTemplateRevisionInput{
			Kind:        kind,
			CarrierType: carrierType,
			CarrierID:   id,
			After:       NewTemplateSnapshot(carrierType, carrier),
		})
		return err
	})
}
