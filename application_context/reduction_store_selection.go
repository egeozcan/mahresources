package application_context

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/models"
	"mahresources/mrql"
)

// A Reduction's selections as rows (design D5): explicit Resource ids in
// reduction_extent_resources, Group ids in reduction_extent_groups, and the
// whole-library flag on the Reduction row. They replace the Extent JSON.

// reductionSelectionCap is the most ids one create or widen request may add, and
// the most Resources "from a Group's Resources" may take.
const reductionSelectionCap = 10000

// reductionSelectionPreview is how many names the review's selection preview lists.
const reductionSelectionPreview = 20

// ErrReductionSelectionTooLarge refuses a selection above the per-request cap.
var ErrReductionSelectionTooLarge = fmt.Errorf("a selection may name at most %d Resources or Groups at a time", reductionSelectionCap)

// ErrReductionGroupNotVisible refuses a selection naming a Group outside the
// principal's subtree, the way an invisible owner Group is refused today.
var ErrReductionGroupNotVisible = errors.New("no such Group")

// selectionAfterPreflight runs inside the "from a Group's Resources" transaction
// between its preflight count and its inserts. It is nil outside a test, which
// uses it to land a concurrent insert there.
var selectionAfterPreflight func(tx *gorm.DB)

// addExtentResources adds explicit Resource ids, ignoring ones already selected.
// A confined principal selects only Resources whose owner is in its subtree: the
// others are skipped, as the scoped Resource query skips them on today's create.
func (s *reductionStore) addExtentResources(r *models.ResourceReduction, ids []uint) error {
	if len(ids) > reductionSelectionCap {
		return ErrReductionSelectionTooLarge
	}
	ids = dedupeUints(ids)
	switch s.scope.mode {
	case scopeDeny:
		return nil
	case scopeSubtree:
		prefix, args := s.with()
		return s.db.Transaction(func(tx *gorm.DB) error {
			for from := 0; from < len(ids); from += reductionRowChunk {
				chunk := ids[from:min(from+reductionRowChunk, len(ids))]
				if err := tx.Exec(prefix+`INSERT INTO reduction_extent_resources (reduction_id, resource_id)
					SELECT ?, id FROM resources WHERE id IN ? AND owner_id IN `+subtreeIDs+` ON CONFLICT DO NOTHING`,
					append(append([]any{}, args...), r.ID, chunk)...).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}
	rows := make([]models.ReductionExtentResource, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, models.ReductionExtentResource{ReductionID: r.ID, ResourceID: id})
	}
	return s.insertIgnore(rows)
}

// addExtentGroups adds Group ids, ignoring ones already selected. Every Group a
// confined principal names must be in its subtree, or the request is refused
// whole (ErrReductionGroupNotVisible).
func (s *reductionStore) addExtentGroups(r *models.ResourceReduction, ids []uint) error {
	if len(ids) > reductionSelectionCap {
		return ErrReductionSelectionTooLarge
	}
	ids = dedupeUints(ids)
	rows := make([]models.ReductionExtentGroup, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, models.ReductionExtentGroup{ReductionID: r.ID, GroupID: id})
	}
	switch s.scope.mode {
	case scopeDeny:
		if len(ids) > 0 {
			return ErrReductionGroupNotVisible
		}
		return nil
	case scopeSubtree:
		prefix, args := s.with()
		return s.db.Transaction(func(tx *gorm.DB) error {
			var visible int64
			for from := 0; from < len(ids); from += reductionRowChunk {
				chunk := ids[from:min(from+reductionRowChunk, len(ids))]
				var n int64
				if err := tx.Raw(prefix+`SELECT COUNT(*) FROM groups WHERE id IN ? AND id IN `+subtreeIDs,
					append(append([]any{}, args...), chunk)...).Scan(&n).Error; err != nil {
					return err
				}
				visible += n
			}
			if visible != int64(len(ids)) {
				return ErrReductionGroupNotVisible
			}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, reductionRowChunk).Error
		})
	}
	return s.insertIgnore(rows)
}

func (s *reductionStore) insertIgnore(rows any) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, reductionRowChunk).Error
	})
}

// addExtentFromGroupResources selects every Resource the Group owns that the
// principal may see. See addExtentFromOwners.
func (s *reductionStore) addExtentFromGroupResources(r *models.ResourceReduction, groupID uint) (int, error) {
	return s.addExtentFromOwners(r, groupID, false)
}

// ownerTreeCTE names the Groups whose Resources are selected: the root alone, or
// the root and every Group below it (UNION, so a cycle in the tree ends).
const (
	ownerTreeOne     = `owner_tree(id) AS (SELECT CAST(? AS bigint))`
	ownerTreeSubtree = `owner_tree(id) AS (SELECT id FROM groups WHERE id = ? UNION SELECT g.id FROM groups g INNER JOIN owner_tree ot ON g.owner_id = ot.id)`
)

// addExtentFromOwners selects every Resource a Group owns, or that it and every
// Group below it own, that the principal may see, by scoped INSERT ... SELECT in
// id-keyset chunks, so no id list is held in Go and the number of statements does
// not grow with the number of Groups. More than the cap refuses the whole request.
// It returns how many Resources were selected (new or already there).
func (s *reductionStore) addExtentFromOwners(r *models.ResourceReduction, rootID uint, descendants bool) (int, error) {
	if s.scope.mode == scopeDeny {
		return 0, nil
	}
	tree := ownerTreeOne
	if descendants {
		tree = ownerTreeSubtree
	}
	// The scope's own subtree (scope_tree) comes first when the principal is
	// confined; the owner tree is a second CTE of the same WITH.
	prefix, args := "WITH RECURSIVE "+tree+" ", []any{rootID}
	if s.scope.mode == scopeSubtree {
		prefix, args = mrql.SubtreeCTEDefinition+", "+tree+" ", []any{s.scope.root, rootID}
	}
	owned := " owner_id IN (SELECT id FROM owner_tree)"
	if s.scope.mode == scopeSubtree {
		owned += " AND owner_id IN " + subtreeIDs
	}
	with := func(extra ...any) []any { return append(append([]any{}, args...), extra...) }
	total := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Raw(prefix+`SELECT COUNT(*) FROM (SELECT 1 FROM resources WHERE`+owned+` LIMIT ?) capped`,
			with(reductionSelectionCap+1)...).Scan(&count).Error; err != nil {
			return err
		}
		if count > reductionSelectionCap {
			return ErrReductionSelectionTooLarge
		}
		if selectionAfterPreflight != nil {
			selectionAfterPreflight(tx)
		}
		after := uint(0)
		for {
			var hi uint
			if err := tx.Raw(prefix+`SELECT COALESCE(MAX(id), 0) FROM (SELECT id FROM resources WHERE`+owned+` AND id > ? ORDER BY id LIMIT ?) chunk`,
				with(after, reductionRowChunk)...).Scan(&hi).Error; err != nil {
				return err
			}
			if hi == 0 {
				return nil
			}
			res := tx.Exec(prefix+`INSERT INTO reduction_extent_resources (reduction_id, resource_id)
				SELECT ?, id FROM resources WHERE`+owned+` AND id > ? AND id <= ? ORDER BY id
				ON CONFLICT DO NOTHING`,
				with(r.ID, after, hi)...)
			if res.Error != nil {
				return res.Error
			}
			var chunk int64
			if err := tx.Raw(prefix+`SELECT COUNT(*) FROM resources WHERE`+owned+` AND id > ? AND id <= ?`,
				with(after, hi)...).Scan(&chunk).Error; err != nil {
				return err
			}
			total += int(chunk)
			// The preflight can be stale by now; the cap is judged on what this
			// transaction has inserted, and refusing rolls all of it back.
			if total > reductionSelectionCap {
				return ErrReductionSelectionTooLarge
			}
			after = hi
		}
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// extentPreview is the review's selection preview: how many Resources are
// selected explicitly, and the first names among those the principal may see.
func (s *reductionStore) extentPreview(r *models.ResourceReduction) (int64, []string, error) {
	prefix, args := s.with()
	visible := ""
	var n int64
	switch s.scope.mode {
	case scopeDeny:
		return 0, nil, nil
	case scopeSubtree:
		// A Resource moved out of the subtree is neither named nor counted.
		visible = " AND r.owner_id IN " + subtreeIDs
		if err := s.db.Raw(prefix+`SELECT COUNT(*) FROM reduction_extent_resources e JOIN resources r ON r.id = e.resource_id
			WHERE e.reduction_id = ?`+visible, append(append([]any{}, args...), r.ID)...).Scan(&n).Error; err != nil {
			return 0, nil, err
		}
	default:
		if err := s.db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ?", r.ID).Count(&n).Error; err != nil {
			return 0, nil, err
		}
	}
	var names []string
	err := s.db.Raw(prefix+`SELECT r.name FROM reduction_extent_resources e JOIN resources r ON r.id = e.resource_id
		WHERE e.reduction_id = ?`+visible+` ORDER BY e.resource_id LIMIT ?`,
		append(append([]any{}, args...), r.ID, reductionSelectionPreview)...).Scan(&names).Error
	return n, names, err
}
