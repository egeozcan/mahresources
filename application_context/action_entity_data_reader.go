package application_context

import (
	"fmt"

	"mahresources/models"
	"mahresources/plugin_system"
)

type actionEntityDataReader struct {
	ctx *MahresourcesContext
}

// NewActionEntityDataReader returns an ActionEntityDataReader backed by the
// given application context. Used by GetActionRunHandler to re-check an
// action's own Filters against the entities the caller submitted.
func NewActionEntityDataReader(ctx *MahresourcesContext) plugin_system.ActionEntityDataReader {
	return &actionEntityDataReader{ctx: ctx}
}

// Only the filter-relevant columns are read: this runs on every action request
// that declares filters, and the entity's other columns (meta blobs above all)
// would be paid for on every one of them.
//
// The queries go through .Model(), not .Table(): the model is what the subtree
// scope callback keys on, so an entity outside a group-limited principal's
// subtree comes back absent — and absent is a mismatch.
func (a *actionEntityDataReader) EntityFilterData(entity string, ids []uint) (map[uint]map[string]any, error) {
	out := make(map[uint]map[string]any, len(ids))

	switch entity {
	case "resource":
		type row struct {
			ID                 uint
			ContentType        *string
			ResourceCategoryId *uint
		}
		for _, chunk := range chunkUints(ids, entityRefChunkSize) {
			var rows []row
			if err := a.ctx.db.Model(&models.Resource{}).
				Select("id", "content_type", "resource_category_id").
				Where("id IN ?", chunk).
				Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, r := range rows {
				data := map[string]any{}
				if r.ContentType != nil {
					data["content_type"] = *r.ContentType
				}
				// The resource's category lives under the key
				// actionMatchesFilters reads, which is "category_id" for every
				// entity type.
				if r.ResourceCategoryId != nil {
					data["category_id"] = *r.ResourceCategoryId
				}
				out[r.ID] = data
			}
		}
	case "note":
		type row struct {
			ID         uint
			NoteTypeId *uint
		}
		for _, chunk := range chunkUints(ids, entityRefChunkSize) {
			var rows []row
			if err := a.ctx.db.Model(&models.Note{}).
				Select("id", "note_type_id").
				Where("id IN ?", chunk).
				Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, r := range rows {
				data := map[string]any{}
				if r.NoteTypeId != nil {
					data["note_type_id"] = *r.NoteTypeId
				}
				out[r.ID] = data
			}
		}
	case "group":
		type row struct {
			ID         uint
			CategoryId *uint
		}
		for _, chunk := range chunkUints(ids, entityRefChunkSize) {
			var rows []row
			if err := a.ctx.db.Model(&models.Group{}).
				Select("id", "category_id").
				Where("id IN ?", chunk).
				Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, r := range rows {
				data := map[string]any{}
				if r.CategoryId != nil {
					data["category_id"] = *r.CategoryId
				}
				out[r.ID] = data
			}
		}
	default:
		return nil, fmt.Errorf("unknown entity type %q", entity)
	}

	return out, nil
}
