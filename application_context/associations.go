package application_context

import "mahresources/models"

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
