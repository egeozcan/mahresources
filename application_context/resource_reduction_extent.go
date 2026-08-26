package application_context

import (
	"fmt"
	"time"

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
	// ResourceIDs are the Resources named explicitly — filtered, like everything
	// here, to what the current principal may see.
	ResourceIDs map[uint]bool
	// GroupIDs are the selected Groups and every descendant of each.
	GroupIDs map[uint]bool
	// SelectedGroups is how many of the *named* Groups the principal may see, as
	// opposed to how many Groups the expansion reached. The page reports what was
	// selected, and after somebody's subtree shrinks that number has to shrink
	// with it: an exact count of Groups they can no longer open is a fact about
	// those Groups.
	SelectedGroups int
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
		extent.SelectedGroups++
	}

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

// countExtentResources is how many distinct Resources the Extent holds.
//
// This walks the whole Extent, so it is deliberately called once per compute and
// never on a page render. A Reduction over a top-level Group in a library of
// millions reaches millions of Resources, and paying that per page view is the
// difference between a review surface and an outage. What the page shows instead
// is the figure the last compute recorded, which is also the figure its coverage
// line is about.
func (ctx *MahresourcesContext) countExtentResources(extent *ReductionExtent) (int, error) {
	total := 0
	err := ctx.extentResourceIDs(extent, func(ids []uint) error {
		total += len(ids)
		return nil
	})
	return total, err
}

// extentArrivalsSince counts the Resources that entered the Extent after the
// given instant — the drift figure the page reports so a Group-scoped Reduction
// can be seen going out of date rather than surprising the reviewer.
//
// Filtered on created_at first and deduplicated over ids rather than summed over
// counts. The filter is what keeps this cheap: however wide the Extent, only the
// Resources created since the last compute are ever materialised, and a Resource
// that is both named explicitly and owned by a named Group must be counted once.
func (ctx *MahresourcesContext) extentArrivalsSince(extent *ReductionExtent, since time.Time) (int, error) {
	arrivals := map[uint]bool{}
	collect := func(ids []uint) {
		for _, id := range ids {
			arrivals[id] = true
		}
	}

	for _, chunk := range chunkUints(mapKeys(extent.ResourceIDs), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", chunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &found).Error; err != nil {
			return 0, err
		}
		collect(found)
	}

	for _, groupChunk := range chunkUints(mapKeys(extent.GroupIDs), idChunk) {
		var owned []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.owner_id IN ?", groupChunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &owned).Error; err != nil {
			return 0, err
		}
		collect(owned)

		var related []uint
		if err := ctx.db.Model(&models.Resource{}).
			Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
			Where("grr.group_id IN ?", groupChunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &related).Error; err != nil {
			return 0, err
		}
		collect(related)
	}

	return len(arrivals), nil
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
