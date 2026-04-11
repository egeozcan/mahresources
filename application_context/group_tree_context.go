package application_context

import (
	"fmt"

	"mahresources/models/query_models"
)

// GetGroupTreeRoots returns top-level groups (no owner) with child counts.
// Always returns a non-nil slice so JSON marshaling produces [] instead of null.
func (ctx *MahresourcesContext) GetGroupTreeRoots(limit int) ([]query_models.GroupTreeNode, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	results := make([]query_models.GroupTreeNode, 0)

	err := ctx.db.Raw(`
		SELECT g.id, g.name, g.owner_id, COALESCE(c.name, '') AS category_name,
		       (SELECT COUNT(*) FROM groups ch WHERE ch.owner_id = g.id) AS child_count
		FROM groups g
		LEFT JOIN categories c ON c.id = g.category_id
		WHERE g.owner_id IS NULL
		ORDER BY g.name
		LIMIT ?
	`, limit).Scan(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetGroupTreeChildren returns the direct children of a group with child counts.
// Always returns a non-nil slice so JSON marshaling produces [] instead of null.
func (ctx *MahresourcesContext) GetGroupTreeChildren(parentID uint, limit int) ([]query_models.GroupTreeNode, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	results := make([]query_models.GroupTreeNode, 0)

	err := ctx.db.Raw(`
		SELECT g.id, g.name, g.owner_id, COALESCE(c.name, '') AS category_name,
		       (SELECT COUNT(*) FROM groups ch WHERE ch.owner_id = g.id) AS child_count
		FROM groups g
		LEFT JOIN categories c ON c.id = g.category_id
		WHERE g.owner_id = ?
		ORDER BY g.name
		LIMIT ?
	`, parentID, limit).Scan(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}

// collectSubtreeGroupIDs returns the flat list of all group IDs in the subtree
// rooted at rootID (including the root itself). It uses a recursive CTE with
// no artificial per-parent cap so it works correctly for large subtrees.
// A defensive ceiling of 1_000_000 rows is applied.
func (ctx *MahresourcesContext) collectSubtreeGroupIDs(rootID uint) ([]uint, error) {
	var ids []uint
	err := ctx.db.Raw(`
		WITH RECURSIVE tree AS (
			SELECT id FROM groups WHERE id = ?
			UNION ALL
			SELECT g.id FROM groups g JOIN tree ON g.owner_id = tree.id
		)
		SELECT id FROM tree
		LIMIT 1000000
	`, rootID).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("collectSubtreeGroupIDs(%d): %w", rootID, err)
	}
	return ids, nil
}

// GetGroupTreeDown returns a tree of groups starting from rootID, going maxLevels deep.
// Each parent's children are limited to childLimit to prevent explosions.
func (ctx *MahresourcesContext) GetGroupTreeDown(rootID uint, maxLevels int, childLimit int) ([]query_models.GroupTreeRow, error) {
	if maxLevels <= 0 {
		maxLevels = 3
	}
	if childLimit <= 0 || childLimit > 100 {
		childLimit = 50
	}

	var results []query_models.GroupTreeRow

	err := ctx.db.Raw(`
		WITH RECURSIVE tree AS (
			SELECT id, name, owner_id, category_id, 0 AS level
			FROM groups WHERE id = ?
			UNION ALL
			SELECT g.id, g.name, g.owner_id, g.category_id, tree.level + 1
			FROM groups g
			INNER JOIN tree ON g.owner_id = tree.id
			WHERE tree.level < ?
		)
		SELECT t.id, t.name, t.owner_id, COALESCE(c.name, '') AS category_name,
		       (SELECT COUNT(*) FROM groups ch WHERE ch.owner_id = t.id) AS child_count,
		       t.level
		FROM tree t
		LEFT JOIN categories c ON c.id = t.category_id
		ORDER BY t.level, t.name
		LIMIT 5000
	`, rootID, maxLevels).Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Enforce per-parent child limit in Go
	parentChildCount := make(map[uint]int)
	filtered := make([]query_models.GroupTreeRow, 0, len(results))

	for _, row := range results {
		if row.Level == 0 {
			// Always include root
			filtered = append(filtered, row)
			continue
		}

		parentID := uint(0)
		if row.OwnerID != nil {
			parentID = *row.OwnerID
		}

		if parentChildCount[parentID] < childLimit {
			filtered = append(filtered, row)
			parentChildCount[parentID]++
		}
	}

	results = filtered

	return results, nil
}
