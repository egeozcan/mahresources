package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
)

func (ctx *MahresourcesContext) MergeGroups(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 {
		return errors.New("one or more losers required")
	}

	if winnerId == 0 {
		return errors.New("invalid winner ID")
	}

	for _, id := range loserIds {
		if id == 0 {
			return errors.New("invalid group ID")
		}
		if id == winnerId {
			return errors.New("winner cannot also be the loser")
		}
	}

	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		// Load losers WITHOUT associations — we only need their basic fields for backup
		var losers []*models.Group
		if loadErr := altCtx.db.Find(&losers, &loserIds).Error; loadErr != nil {
			return loadErr
		}

		// Verify all loser IDs were found
		if len(losers) != len(loserIds) {
			return fmt.Errorf("one or more loser groups not found")
		}

		// Load winner WITHOUT associations
		var winner models.Group
		if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Raw SQL below bypasses the GORM scope callbacks, so for a group-limited
		// principal every transferred association's far endpoint must be confined to
		// the subtree: a loser's association to an out-of-subtree group/note/resource
		// is left untouched rather than re-pointed at the (in-scope) winner. The
		// added filters are no-ops for an unscoped principal (admin/system/editor),
		// and fail-closed when the subtree could not be resolved (subtreeIDs empty →
		// IN () matches nothing). subtreeIDs are group IDs; notes/resources are
		// confined via their owner_id (the same column the read scope uses).
		subtreeIDs, scopedMerge, _ := altCtx.subtreeScopeIDs()

		// Batch SQL transfers — tags (tags are global, never owner-scoped)
		if err := altCtx.db.Exec("INSERT INTO group_tags (group_id, tag_id) SELECT ?, tag_id FROM group_tags WHERE group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related groups (both directions, exclude self-references).
		// Far endpoint is the other group: out-direction → related_group_id, in-direction → group_id.
		relGroupsOutFilter, relGroupsInFilter := "", ""
		relGroupsOutArgs := []any{winnerId, loserIds, winnerId}
		relGroupsInArgs := []any{winnerId, loserIds, winnerId}
		if scopedMerge {
			relGroupsOutFilter = " AND related_group_id IN ?"
			relGroupsOutArgs = append(relGroupsOutArgs, subtreeIDs)
			relGroupsInFilter = " AND group_id IN ?"
			relGroupsInArgs = append(relGroupsInArgs, subtreeIDs)
		}
		if err := altCtx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) SELECT ?, related_group_id FROM group_related_groups WHERE group_id IN ? AND related_group_id != ?"+relGroupsOutFilter+" ON CONFLICT DO NOTHING", relGroupsOutArgs...).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) SELECT group_id, ? FROM group_related_groups WHERE related_group_id IN ? AND group_id != ?"+relGroupsInFilter+" ON CONFLICT DO NOTHING", relGroupsInArgs...).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related notes (far endpoint is a note; scope by note owner).
		relNotesFilter := ""
		relNotesArgs := []any{winnerId, loserIds}
		if scopedMerge {
			relNotesFilter = " AND note_id IN (SELECT id FROM notes WHERE owner_id IN ?)"
			relNotesArgs = append(relNotesArgs, subtreeIDs)
		}
		if err := altCtx.db.Exec("INSERT INTO groups_related_notes (group_id, note_id) SELECT ?, note_id FROM groups_related_notes WHERE group_id IN ?"+relNotesFilter+" ON CONFLICT DO NOTHING", relNotesArgs...).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related resources (far endpoint is a resource; scope by resource owner).
		relResFilter := ""
		relResArgs := []any{winnerId, loserIds}
		if scopedMerge {
			relResFilter = " AND resource_id IN (SELECT id FROM resources WHERE owner_id IN ?)"
			relResArgs = append(relResArgs, subtreeIDs)
		}
		if err := altCtx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) SELECT ?, resource_id FROM groups_related_resources WHERE group_id IN ?"+relResFilter+" ON CONFLICT DO NOTHING", relResArgs...).Error; err != nil {
			return err
		}

		// Batch SQL transfers — group_relations (both directions)
		// group_relations is a full entity with relation_type_id, name, description — transfer all columns.
		// Far endpoint: out-direction → to_group_id, in-direction → from_group_id.
		// Rows created by the merge are attributed to the operator running it
		// (created_by_user_id). The actor bind sits in the SELECT projection, so it
		// is the 2nd placeholder — before the WHERE binds. Nullable *uint → SQL NULL
		// when there is no actor; root under no-auth.
		mergeActor := altCtx.actingUserIDPtr()
		outFilter, inFilter := "", ""
		outArgs := []any{winnerId, mergeActor, loserIds, winnerId}
		inArgs := []any{winnerId, mergeActor, loserIds, winnerId}
		if scopedMerge {
			outFilter = " AND to_group_id IN ?"
			outArgs = append(outArgs, subtreeIDs)
			inFilter = " AND from_group_id IN ?"
			inArgs = append(inArgs, subtreeIDs)
		}
		if err := altCtx.db.Exec("INSERT INTO group_relations (from_group_id, created_by_user_id, to_group_id, relation_type_id, name, description, created_at, updated_at) SELECT ?, ?, to_group_id, relation_type_id, name, description, created_at, updated_at FROM group_relations WHERE from_group_id IN ? AND to_group_id != ?"+outFilter+" ON CONFLICT DO NOTHING", outArgs...).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("INSERT INTO group_relations (from_group_id, to_group_id, created_by_user_id, relation_type_id, name, description, created_at, updated_at) SELECT from_group_id, ?, ?, relation_type_id, name, description, created_at, updated_at FROM group_relations WHERE to_group_id IN ? AND from_group_id != ?"+inFilter+" ON CONFLICT DO NOTHING", inArgs...).Error; err != nil {
			return err
		}

		// Batch SQL transfers — ownership updates
		if err := altCtx.db.Exec("UPDATE groups SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("UPDATE notes SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("UPDATE resources SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Re-read the winner — its owner_id may have changed if it was owned by a loser
		if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Walk up the winner's ancestry chain to detect indirect ownership cycles
		// introduced by the batch ownership transfer above.
		visited := map[uint]bool{winnerId: true}
		current := winner.OwnerId
		for i := 0; i < 100 && current != nil; i++ {
			if visited[*current] {
				// Found a cycle — break it by NULLing this group's owner_id
				if err := altCtx.db.Exec("UPDATE groups SET owner_id = NULL WHERE id = ?", *current).Error; err != nil {
					return err
				}
				break
			}
			visited[*current] = true
			var g models.Group
			if err := altCtx.db.Select("id", "owner_id").First(&g, *current).Error; err != nil {
				break
			}
			current = g.OwnerId
		}

		backups := make(map[string]types.JSON)
		for _, loser := range losers {
			backupData, err := json.Marshal(loser)
			if err != nil {
				return err
			}
			backups[fmt.Sprintf("group_%v", loser.ID)] = backupData
		}

		// Merge all losers' meta into the winner in a single batched operation,
		// avoiding the previous N+1 (one UPDATE per loser). Winner keys always
		// win on conflict; among losers, the lowest-id loser wins, which is
		// deterministic and consistent across Postgres and SQLite.
		var err error
		switch altCtx.Config.DbType {
		case constants.DbTypePosgres:
			err = altCtx.db.Exec(`
				UPDATE groups SET meta = (
					SELECT coalesce(jsonb_object_agg(key, value), '{}'::jsonb)
					FROM (
						SELECT DISTINCT ON (key) key, value
						FROM (
							SELECT key, value, 1 AS priority, 0 AS ord
								FROM jsonb_each(coalesce(nullif(meta, 'null'::jsonb), '{}'::jsonb))
							UNION ALL
							SELECT key, value, 2 AS priority, g.id AS ord
								FROM groups g, jsonb_each(coalesce(nullif(g.meta, 'null'::jsonb), '{}'::jsonb))
								WHERE g.id IN ?
						) s
						ORDER BY key, priority ASC, ord ASC
					) t
				) WHERE id = ?`, loserIds, winnerId).Error
		case constants.DbTypeSqlite:
			// Aggregate loser meta in Go (lowest-id loser wins on conflict) and
			// apply with a single json_patch so the winner's keys take precedence.
			sortedLosers := make([]*models.Group, len(losers))
			copy(sortedLosers, losers)
			sort.Slice(sortedLosers, func(i, j int) bool { return sortedLosers[i].ID < sortedLosers[j].ID })

			mergedLosersMeta := make(map[string]any)
			for _, loser := range sortedLosers {
				var m map[string]any
				if uErr := json.Unmarshal(loser.Meta, &m); uErr == nil {
					for k, v := range m {
						if _, exists := mergedLosersMeta[k]; !exists {
							mergedLosersMeta[k] = v
						}
					}
				}
			}
			if len(mergedLosersMeta) > 0 {
				mergedMetaBytes, mErr := json.Marshal(mergedLosersMeta)
				if mErr != nil {
					return mErr
				}
				err = altCtx.db.Exec(`UPDATE groups SET meta = json_patch(?, coalesce(nullif(meta, 'null'), '{}')) WHERE id = ?`, string(mergedMetaBytes), winnerId).Error
			}
		default:
			err = errors.New("db doesn't support merging meta")
		}
		if err != nil {
			return err
		}

		for _, loser := range losers {
			if err := altCtx.DeleteGroup(loser.ID); err != nil {
				return err
			}
		}

		// Save backups to winner's meta
		backupObj := make(map[string]any)
		backupObj["backups"] = backups
		backupsBytes, err := json.Marshal(&backupObj)
		if err != nil {
			return err
		}

		if ctx.Config.DbType == constants.DbTypePosgres {
			if err := altCtx.db.Exec("update groups set meta = COALESCE(nullif(meta, 'null'::jsonb), '{}'::jsonb) || ? where id = ?", backupsBytes, winner.ID).Error; err != nil {
				return err
			}
		} else if ctx.Config.DbType == constants.DbTypeSqlite {
			if err := altCtx.db.Exec("update groups set meta = json_patch(COALESCE(nullif(meta, 'null'), '{}'), ?) where id = ?", string(backupsBytes), winner.ID).Error; err != nil {
				return err
			}
		}

		// Clean up any self-referential group relations created during the merge
		if err := altCtx.db.Exec(`DELETE FROM group_relations WHERE to_group_id = from_group_id`).Error; err != nil {
			return err
		}

		return nil
	})
}

func (ctx *MahresourcesContext) GroupMetaKeys() ([]interfaces.MetaKey, error) {
	return metaKeys(ctx, "groups")
}

func (ctx *MahresourcesContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one group ID is required")
	}
	if len(query.EditedId) == 0 {
		return fmt.Errorf("at least one tag ID is required")
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)
	uniqueGroupIds := deduplicateUints(query.ID)

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		// Verify all group IDs exist
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", uniqueGroupIds).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(uniqueGroupIds) {
			return fmt.Errorf("one or more groups not found")
		}

		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", uniqueEditedIds).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more tags not found")
		}

		for _, tagID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO group_tags (group_id, tag_id) SELECT id, ? FROM groups WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one group ID is required")
	}
	if len(query.EditedId) == 0 {
		return fmt.Errorf("at least one tag ID is required")
	}

	uniqueGroupIds := deduplicateUints(query.ID)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		// RBAC: verify all group IDs are visible (scope callback filters this Count).
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", uniqueGroupIds).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(uniqueGroupIds) {
			return fmt.Errorf("one or more groups not found")
		}
		return tx.Exec(
			"DELETE FROM group_tags WHERE group_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one group ID is required")
	}

	if strings.TrimSpace(query.Meta) == "" {
		return nil
	}

	if err := ValidateMeta(query.Meta); err != nil {
		return err
	}

	// Verify all group IDs exist
	var count int64
	if err := ctx.db.Model(&models.Group{}).Where("id IN ?", query.ID).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(deduplicateUints(query.ID)) {
		return fmt.Errorf("one or more groups not found")
	}

	var group models.Group
	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	return ctx.db.
		Model(&group).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error
}

func (ctx *MahresourcesContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteGroup(id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) FindParentsOfGroup(id uint) ([]models.Group, error) {
	var results []models.Group
	var ids []uint

	findIdErr := ctx.db.Raw(`
		WITH RECURSIVE cte AS (
			SELECT id, owner_id, 1 AS level FROM groups WHERE id = ?
			UNION ALL
			SELECT g.id, g.owner_id, cte.level + 1 AS level FROM groups g
			INNER JOIN cte ON cte.owner_id = g.id
			WHERE cte.level < 20
		)
		SELECT id
		FROM cte
		ORDER BY level;
	`, id).Scan(&ids).Error

	if findIdErr != nil {
		return nil, findIdErr
	}

	if len(ids) == 0 {
		return results, nil
	}

	findIdErr = ctx.db.Find(&results, ids).Error

	if findIdErr != nil {
		return nil, findIdErr
	}

	sort.Slice(results, func(i, j int) bool {
		return lib.IndexOf(ids, results[i].ID) > lib.IndexOf(ids, results[j].ID)
	})

	return results, nil
}

func (ctx *MahresourcesContext) DuplicateGroup(id uint) (*models.Group, error) {
	var result *models.Group
	var original models.Group

	if err := ctx.db.Preload(clause.Associations).First(&original, id).Error; err != nil {
		return nil, err
	}

	// Copy slices to avoid shared references with the original
	relatedResources := make([]*models.Resource, len(original.RelatedResources))
	copy(relatedResources, original.RelatedResources)

	relatedNotes := make([]*models.Note, len(original.RelatedNotes))
	copy(relatedNotes, original.RelatedNotes)

	relatedGroups := make([]*models.Group, len(original.RelatedGroups))
	copy(relatedGroups, original.RelatedGroups)

	tags := make([]*models.Tag, len(original.Tags))
	copy(tags, original.Tags)

	result = &models.Group{
		Name:             original.Name,
		Description:      original.Description,
		URL:              original.URL,
		Meta:             original.Meta,
		OwnerId:          original.OwnerId,
		RelatedResources: relatedResources,
		RelatedNotes:     relatedNotes,
		RelatedGroups:    relatedGroups,
		Tags:             tags,
		CategoryId:       original.CategoryId,
	}

	if err := ctx.db.Save(result).Error; err != nil {
		return nil, err
	}

	// For a group-limited principal, only copy relations whose far endpoint is
	// inside the subtree, so cloning an in-scope group (which an admin may have
	// linked to an external group) does not mint new relations referencing groups
	// outside the caller's scope. inSubtree is true-for-all when unscoped.
	subtreeIDs, scopedClone, denyClone := ctx.subtreeScopeIDs()
	allowed := make(map[uint]struct{}, len(subtreeIDs))
	for _, gid := range subtreeIDs {
		allowed[gid] = struct{}{}
	}
	inSubtree := func(gid *uint) bool {
		if !scopedClone {
			return true
		}
		if denyClone || gid == nil {
			return false
		}
		_, ok := allowed[*gid]
		return ok
	}

	// Copy outgoing relationships (original is FromGroup)
	for _, rel := range original.Relationships {
		if !inSubtree(rel.ToGroupId) {
			continue
		}
		newRel := models.GroupRelation{
			FromGroupId:    &result.ID,
			ToGroupId:      rel.ToGroupId,
			RelationTypeId: rel.RelationTypeId,
			Name:           rel.Name,
			Description:    rel.Description,
		}
		// Ignore conflicts (unique index on from_group_id, to_group_id, relation_type_id)
		ctx.db.Create(&newRel)
	}

	// Copy incoming relationships (original is ToGroup)
	for _, rel := range original.BackRelations {
		if !inSubtree(rel.FromGroupId) {
			continue
		}
		newRel := models.GroupRelation{
			FromGroupId:    rel.FromGroupId,
			ToGroupId:      &result.ID,
			RelationTypeId: rel.RelationTypeId,
			Name:           rel.Name,
			Description:    rel.Description,
		}
		ctx.db.Create(&newRel)
	}

	return result, nil
}
