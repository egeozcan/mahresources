package application_context

import (
	"fmt"

	"mahresources/models"
)

// idChunk bounds an IN-list. SQLite's and Postgres's parameter ceilings are
// reachable here in a way they are not for most queries: an explicit Extent is
// whatever the reviewer selected, and a Group's subtree can be arbitrarily wide.
// Everything below chunks rather than assuming the list is small — the same
// reasoning visibleGroupIDs' own comment gives for not appending an IN-list of
// subtree ids at all.
const idChunk = 500

// ReductionExtent is a Reduction's Extent, resolved.
//
// Group ids are expanded through their descendants here rather than at rest,
// because a Group's descendants and its contents both change and D23 makes
// re-scanning an explicit act. This runs once per compute.
type ReductionExtent struct {
	// ResourceIDs are the Resources named explicitly.
	ResourceIDs map[uint]bool
	// GroupIDs are the selected Groups and every descendant of each.
	GroupIDs map[uint]bool
	// Size is how many distinct Resources the Extent holds.
	Size int
}

// Empty reports an Extent that names nothing.
func (e *ReductionExtent) Empty() bool {
	return len(e.ResourceIDs) == 0 && len(e.GroupIDs) == 0
}

// resolveReductionExtent expands the stored Extent against the current database
// and the current principal.
//
// Scoping is the db handle's, not this function's: ctx.db carries the principal's
// subtree filter through the GORM callback, so a confined reviewer's Extent
// silently loses the Resources and Groups outside their subtree rather than this
// code having to remember to filter. That is also why a Reduction is re-resolved
// on every compute and every render instead of being expanded once and stored: a
// scoped user's subtree can change after a Reduction is created.
func (ctx *MahresourcesContext) resolveReductionExtent(stored models.ResourceReductionExtent) (*ReductionExtent, error) {
	extent := &ReductionExtent{
		ResourceIDs: map[uint]bool{},
		GroupIDs:    map[uint]bool{},
	}

	// Explicit Resource ids, filtered to the ones that still exist and that this
	// principal may see. A deleted Resource simply leaves the Extent.
	for _, chunk := range chunkUints(stored.ResourceIDs, idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", chunk).
			Pluck("resources.id", &found).Error; err != nil {
			return nil, fmt.Errorf("resolving the Extent's Resources: %w", err)
		}
		for _, id := range found {
			extent.ResourceIDs[id] = true
		}
	}

	// Groups, each expanded through its own subtree. A Reduction over a parent
	// Group covers the subtree the reviewer actually means.
	for _, root := range stored.GroupIDs {
		if !ctx.GroupVisible(root) {
			continue
		}
		ids, err := ctx.collectSubtreeGroupIDs(root)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			extent.GroupIDs[id] = true
		}
	}

	size, err := ctx.countExtentResources(extent)
	if err != nil {
		return nil, err
	}
	extent.Size = size
	return extent, nil
}

// extentResourceIDs streams every Resource id in the Extent through fn, in
// chunks, so a wide Extent never becomes one enormous slice.
//
// A Resource is in a Group's Extent if the Group owns it or is related to it.
// Both, because both are how the app files a Resource under a Group and the
// resources list shows both — an Extent that covered only one would silently miss
// half of what "everything filed under Holidays" means.
func (ctx *MahresourcesContext) extentResourceIDs(extent *ReductionExtent, fn func(ids []uint) error) error {
	seen := map[uint]bool{}
	emit := func(ids []uint) error {
		fresh := make([]uint, 0, len(ids))
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				fresh = append(fresh, id)
			}
		}
		if len(fresh) == 0 {
			return nil
		}
		return fn(fresh)
	}

	for _, chunk := range chunkUints(mapKeys(extent.ResourceIDs), idChunk) {
		if err := emit(chunk); err != nil {
			return err
		}
	}

	if len(extent.GroupIDs) == 0 {
		return nil
	}
	groupIDs := mapKeys(extent.GroupIDs)
	for _, groupChunk := range chunkUints(groupIDs, idChunk) {
		var owned []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.owner_id IN ?", groupChunk).
			Pluck("resources.id", &owned).Error; err != nil {
			return fmt.Errorf("resolving the Extent's owned Resources: %w", err)
		}
		for _, c := range chunkUints(owned, idChunk) {
			if err := emit(c); err != nil {
				return err
			}
		}

		var related []uint
		if err := ctx.db.Model(&models.Resource{}).
			Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
			Where("grr.group_id IN ?", groupChunk).
			Pluck("resources.id", &related).Error; err != nil {
			return fmt.Errorf("resolving the Extent's related Resources: %w", err)
		}
		for _, c := range chunkUints(related, idChunk) {
			if err := emit(c); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ctx *MahresourcesContext) countExtentResources(extent *ReductionExtent) (int, error) {
	total := 0
	err := ctx.extentResourceIDs(extent, func(ids []uint) error {
		total += len(ids)
		return nil
	})
	return total, err
}

// containsResources reports which of the given Resource ids are inside the
// Extent. Asked of the Cluster candidates rather than of the whole library, so
// the answer costs one query per chunk of candidates however wide the Extent is.
func (ctx *MahresourcesContext) containsResources(extent *ReductionExtent, candidates []uint) (map[uint]bool, error) {
	inside := map[uint]bool{}
	remaining := make([]uint, 0, len(candidates))
	for _, id := range candidates {
		if extent.ResourceIDs[id] {
			inside[id] = true
			continue
		}
		remaining = append(remaining, id)
	}
	if len(remaining) == 0 || len(extent.GroupIDs) == 0 {
		return inside, nil
	}

	groupIDs := mapKeys(extent.GroupIDs)
	for _, candidateChunk := range chunkUints(remaining, idChunk) {
		for _, groupChunk := range chunkUints(groupIDs, idChunk) {
			var owned []uint
			if err := ctx.db.Model(&models.Resource{}).
				Where("resources.id IN ?", candidateChunk).
				Where("resources.owner_id IN ?", groupChunk).
				Pluck("resources.id", &owned).Error; err != nil {
				return nil, err
			}
			for _, id := range owned {
				inside[id] = true
			}

			var related []uint
			if err := ctx.db.Model(&models.Resource{}).
				Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
				Where("resources.id IN ?", candidateChunk).
				Where("grr.group_id IN ?", groupChunk).
				Pluck("resources.id", &related).Error; err != nil {
				return nil, err
			}
			for _, id := range related {
				inside[id] = true
			}
		}
	}
	return inside, nil
}

func mapKeys(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
