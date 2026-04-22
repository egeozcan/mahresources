package application_context

import (
	"fmt"

	"gorm.io/gorm"
	"mahresources/application_context/validation"
	"mahresources/models"
)

// MaxEntityNameLength is the maximum allowed length for entity names.
// Names longer than this are rejected to prevent database bloat and rendering issues.
const MaxEntityNameLength = 1000

// ValidateEntityName checks that a name is well-formed for the given entity type.
//
// A name is rejected if it:
//   - exceeds MaxEntityNameLength bytes
//   - contains NUL bytes, C0 controls (except TAB), C1 controls, CR/LF,
//     or Unicode directional overrides/isolates (BH-019)
//
// Empty names are accepted here so that callers preserving backward-compatible
// "name is optional on update" semantics can continue to do so; callers that
// require a non-empty name must still check for that themselves (the existing
// call sites do, and they check before invoking this function).
func ValidateEntityName(name, entityType string) error {
	if len(name) > MaxEntityNameLength {
		return fmt.Errorf("%s name must not exceed %d characters", entityType, MaxEntityNameLength)
	}
	// Allow empty names through — the existing "name optional on update"
	// semantics in some context files rely on that. Non-empty names must
	// pass the control-character / bidi-override checks.
	if name == "" {
		return nil
	}
	if _, err := validation.SanitizeEntityName(name); err != nil {
		return fmt.Errorf("%s %s", entityType, err.Error())
	}
	return nil
}

// ValidateAssociationIDs checks that all given IDs exist in the database for the model type T.
// This prevents phantom entity creation via GORM's many-to-many Association().Append/Replace.
func ValidateAssociationIDs[T any](db *gorm.DB, ids []uint, entityName string) error {
	if len(ids) == 0 {
		return nil
	}
	unique := make(map[uint]bool)
	for _, id := range ids {
		unique[id] = true
	}
	uniqueSlice := make([]uint, 0, len(unique))
	for id := range unique {
		uniqueSlice = append(uniqueSlice, id)
	}
	var count int64
	if err := db.Model(new(T)).Where("id IN ?", uniqueSlice).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniqueSlice) {
		return fmt.Errorf("one or more %s not found", entityName)
	}
	return nil
}

// BuildAssociationSlice converts a slice of IDs to a slice of model structs.
// The factory function creates a model instance from an ID.
func BuildAssociationSlice[T any](ids []uint, factory func(uint) T) []T {
	result := make([]T, len(ids))
	for i, id := range ids {
		result[i] = factory(id)
	}
	return result
}

// BuildAssociationSlicePtr converts a slice of IDs to a slice of model struct pointers.
// The factory function creates a model instance pointer from an ID.
func BuildAssociationSlicePtr[T any](ids []uint, factory func(uint) *T) []*T {
	result := make([]*T, len(ids))
	for i, id := range ids {
		result[i] = factory(id)
	}
	return result
}

// Factory functions for creating model instances from IDs

func TagFromID(id uint) models.Tag {
	return models.Tag{ID: id}
}

func TagPtrFromID(id uint) *models.Tag {
	return &models.Tag{ID: id}
}

func GroupFromID(id uint) models.Group {
	return models.Group{ID: id}
}

func GroupPtrFromID(id uint) *models.Group {
	return &models.Group{ID: id}
}

func NoteFromID(id uint) models.Note {
	return models.Note{ID: id}
}

func NotePtrFromID(id uint) *models.Note {
	return &models.Note{ID: id}
}

func ResourceFromID(id uint) models.Resource {
	return models.Resource{ID: id}
}

func ResourcePtrFromID(id uint) *models.Resource {
	return &models.Resource{ID: id}
}

// deduplicateUints returns a new slice with duplicate values removed, preserving order.
func deduplicateUints(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}
