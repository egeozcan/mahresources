package application_context

import (
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

const entityRefChunkSize = 500

type actionEntityRefReader struct {
	ctx *MahresourcesContext
}

// NewActionEntityRefReader returns an EntityRefReader backed by the given
// application context. Used by GetActionRunHandler to validate entity_ref
// params before dispatching the action.
func NewActionEntityRefReader(ctx *MahresourcesContext) plugin_system.EntityRefReader {
	return &actionEntityRefReader{ctx: ctx}
}

func chunkUints(ids []uint, size int) [][]uint {
	if len(ids) <= size {
		return [][]uint{ids}
	}
	var out [][]uint
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[i:end])
	}
	return out
}

func (a *actionEntityRefReader) ResourcesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	var matched []uint
	for _, chunk := range chunkUints(ids, entityRefChunkSize) {
		q := &query_models.ResourceSearchQuery{
			Ids:          chunk,
			ContentTypes: filter.ContentTypes,
		}
		rows, err := a.ctx.GetResources(0, len(chunk), q)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			matched = append(matched, r.ID)
		}
	}
	return matched, nil
}

func (a *actionEntityRefReader) NotesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	var matched []uint
	for _, chunk := range chunkUints(ids, entityRefChunkSize) {
		q := &query_models.NoteQuery{
			Ids:         chunk,
			NoteTypeIds: filter.NoteTypeIDs,
		}
		rows, err := a.ctx.GetNotes(0, len(chunk), q)
		if err != nil {
			return nil, err
		}
		for _, n := range rows {
			matched = append(matched, n.ID)
		}
	}
	return matched, nil
}

func (a *actionEntityRefReader) GroupsMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	var matched []uint
	for _, chunk := range chunkUints(ids, entityRefChunkSize) {
		q := &query_models.GroupQuery{
			Ids:        chunk,
			Categories: filter.CategoryIDs,
		}
		rows, err := a.ctx.GetGroups(0, len(chunk), q)
		if err != nil {
			return nil, err
		}
		for _, g := range rows {
			matched = append(matched, g.ID)
		}
	}
	return matched, nil
}
