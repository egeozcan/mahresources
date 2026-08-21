package application_context

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// relationInScope reports whether both endpoints of a relation edge are visible
// under the current principal's scope.
//
// group_relations is not in scopeColumn (scoping.go), and it cannot be: subtree
// containment for an edge is a property of two columns, not one, so the ORM
// callbacks that confine groups, resources and notes never see it. GetRelation
// below has compensated for that on the read path since before this change; the
// write paths did not, which left every relation edge in the database renameable
// and deletable by any principal that could reach a write at all. A
// group-confined user reaches one whenever its own ordinary CRUD wakes a plugin
// hook, because mah.db exposes UpdateGroupRelation and DeleteGroupRelation.
//
// Both endpoints must be visible, not either: an edge whose far end is outside
// the subtree still describes a group the principal may not see, and renaming or
// deleting it is a write about that group. AddRelation already enforces this
// implicitly, by reading both groups through the scoped handle before inserting.
func (ctx *MahresourcesContext) relationInScope(relation *models.GroupRelation) bool {
	if !ctx.isScopedPrincipal() {
		return true
	}
	if relation == nil || relation.FromGroupId == nil || relation.ToGroupId == nil {
		// A dangling edge belongs to no subtree, so no scoped principal owns it.
		return false
	}
	// visibleGroupIDs, not GroupVisible: it reads the allow-list applyPrincipalScope
	// already materialised onto the db context and issues no query at all, and it
	// is the helper written for tables scopeColumn does not map. GroupVisible would
	// run two COUNTs through the scoped handle, each carrying the whole subtree as
	// an IN list — past SQLite's variable limit that query *errors*, and
	// entityVisible reads an error as "not visible", so a principal with a large
	// enough subtree would be denied its own in-subtree edges.
	visible := ctx.visibleGroupIDs([]uint{*relation.FromGroupId, *relation.ToGroupId})
	return visible[*relation.FromGroupId] && visible[*relation.ToGroupId]
}

func (ctx *MahresourcesContext) EditRelation(query query_models.GroupRelationshipQuery) (*models.GroupRelation, error) {
	if err := ctx.requireEditorRole("editing a relation"); err != nil {
		return nil, err
	}

	var relation = &models.GroupRelation{ID: query.Id}

	if err := ctx.db.First(relation).Error; err != nil {
		return nil, err
	}

	// gorm.ErrRecordNotFound rather than a distinct "forbidden", matching what
	// GetRelation already answers: an out-of-scope group reads as absent, so a
	// different answer here would be an oracle for which relation ids exist.
	if !ctx.relationInScope(relation) {
		return nil, gorm.ErrRecordNotFound
	}

	if query.Name != "" {
		relation.Name = query.Name
	}
	relation.Description = query.Description

	if err := ctx.db.Save(relation).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "relation", &relation.ID, relation.Name, "Updated relation", nil)

	return relation, nil
}

func (ctx *MahresourcesContext) AddRelation(fromGroupId, toGroupId, relationTypeId uint, name, description string) (*models.GroupRelation, error) {
	if err := ctx.requireEditorRole("creating a relation"); err != nil {
		return nil, err
	}

	var relationType models.GroupRelationType
	var fromGroup models.Group
	var toGroup models.Group
	var relation models.GroupRelation

	if fromGroupId == 0 || toGroupId == 0 {
		return nil, errors.New("fromGroupId and toGroupId are required")
	}

	if fromGroupId == toGroupId {
		return nil, errors.New("cannot relate to self")
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&relationType, relationTypeId).Error; err != nil {
			return err
		}

		if err := tx.First(&fromGroup, fromGroupId).Error; err != nil {
			return err
		}

		if err := tx.First(&toGroup, toGroupId).Error; err != nil {
			return err
		}

		if fromGroup.CategoryId == nil || toGroup.CategoryId == nil ||
			relationType.FromCategoryId == nil || relationType.ToCategoryId == nil {
			return errors.New("both groups and the relation type must have categories assigned")
		}

		// Findings 60 and 65 both ask for this message to say what is wrong.
		// "category mismatch" names neither side, neither requirement, and not
		// even which of the two groups is the problem — a reader who picked one
		// wrong group out of two has to guess. Naming both sides is also what
		// makes the message actionable without opening the relation type.
		//
		// docs/todo.md recorded this as fixed in Batch 12 and it was not: the
		// line below was untouched since the auth merge, and a live POST still
		// answered {"error":"category mismatch"}. Found by the Phase 3 coverage
		// audit, which asks whether every finding marked FIXED is named by a test.
		if *toGroup.CategoryId != *relationType.ToCategoryId || *fromGroup.CategoryId != *relationType.FromCategoryId {
			var fromCategory, toCategory models.Category
			fromName := ctx.categoryNameFor(tx, relationType.FromCategoryId, &fromCategory)
			toName := ctx.categoryNameFor(tx, relationType.ToCategoryId, &toCategory)

			var problems []string
			if *fromGroup.CategoryId != *relationType.FromCategoryId {
				problems = append(problems, fmt.Sprintf(
					"%q is the From group and relation type %q requires its From group to be in category %s",
					fromGroup.Name, relationType.Name, fromName))
			}
			if *toGroup.CategoryId != *relationType.ToCategoryId {
				problems = append(problems, fmt.Sprintf(
					"%q is the To group and relation type %q requires its To group to be in category %s",
					toGroup.Name, relationType.Name, toName))
			}
			return errors.New("category mismatch: " + strings.Join(problems, "; "))
		}

		relation = models.GroupRelation{
			FromGroupId:    &fromGroup.ID,
			ToGroupId:      &toGroup.ID,
			RelationTypeId: &relationType.ID,
			Name:           name,
			Description:    description,
		}

		if err := tx.Save(&relation).Error; err != nil {
			return err
		}

		if relationType.BackRelationId != nil {
			backRelation := &models.GroupRelation{
				FromGroupId:    &toGroup.ID,
				ToGroupId:      &fromGroup.ID,
				RelationTypeId: relationType.BackRelationId,
			}

			return tx.Save(backRelation).Error
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionCreate, "relation", &relation.ID, relation.Name, "Created relation", nil)
	}

	return &relation, err
}

func (ctx *MahresourcesContext) GetRelation(id uint) (*models.GroupRelation, error) {
	var relation models.GroupRelation

	if err := ctx.db.Preload(clause.Associations, pageLimit).First(&relation, id).Error; err != nil {
		return nil, err
	}
	// group_relations is not owner-scoped, so a scoped principal could otherwise
	// read a relation (and its endpoint group IDs) outside its subtree. Require
	// both endpoints visible; fail-closed to not-found otherwise.
	if ctx.isScopedPrincipal() {
		fromVisible := relation.FromGroupId != nil && ctx.GroupVisible(*relation.FromGroupId)
		toVisible := relation.ToGroupId != nil && ctx.GroupVisible(*relation.ToGroupId)
		if !fromVisible || !toVisible {
			return nil, gorm.ErrRecordNotFound
		}
	}
	return &relation, nil
}

func (ctx *MahresourcesContext) GetRelationType(id uint) (*models.GroupRelationType, error) {
	var relationType models.GroupRelationType
	ctx.db.Preload(clause.Associations, pageLimit).First(&relationType, id)

	return &relationType, ctx.db.Preload(clause.Associations, pageLimit).First(&relationType, id).Error
}

func (ctx *MahresourcesContext) AddRelationType(query *query_models.RelationshipTypeEditorQuery) (*models.GroupRelationType, error) {
	if err := ctx.requireEditorRole("creating a relation type"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("relation type name is required")
	}

	if query.FromCategory == 0 || query.ToCategory == 0 {
		return nil, errors.New("fromCategory and toCategory are required")
	}

	var relationType = models.GroupRelationType{
		Name:           query.Name,
		FromCategoryId: &query.FromCategory,
		ToCategoryId:   &query.ToCategory,
		Description:    query.Description,
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&relationType).Error; err != nil {
			if isForeignKeyError(err) {
				return errors.New("fromCategory and toCategory must reference existing categories")
			}
			return err
		}

		if query.ReverseName != "" {
			if query.ReverseName == query.Name {
				if query.FromCategory != query.ToCategory {
					return errors.New("self-referential relation types require FromCategory == ToCategory")
				}

				relationType.BackRelationId = &relationType.ID

				return tx.Save(&relationType).Error
			}

			backRelationType := models.GroupRelationType{
				Name:           query.ReverseName,
				FromCategoryId: &query.ToCategory,
				ToCategoryId:   &query.FromCategory,
			}

			// Adopting an existing reverse type is deliberate: it is how a pair
			// created separately gets linked, and inserting a duplicate instead
			// would be worse. But only "not found" may fall through to the insert.
			// Treating every error as "not found" turned a transient read failure
			// into a second, unlinked reverse type — and left the caller with no
			// sign that the lookup had failed at all.
			lookupErr := tx.Where(&backRelationType).First(&backRelationType).Error
			switch {
			case lookupErr == nil:
				// Claim the existing row conditionally rather than Save()-ing over
				// it. The read below and the write after it are two statements, so
				// two concurrent creates both saw back_relation_id NULL and both
				// wrote: the second silently won, leaving the first's forward type
				// pointing at a reverse type that points at somebody else. Making
				// the claim the predicate is the shape ClaimDownloadHistoryRetry
				// already uses, and RowsAffected is the answer.
				claim := tx.Model(&models.GroupRelationType{}).
					Where("id = ? AND back_relation_id IS NULL", backRelationType.ID).
					Update("back_relation_id", relationType.ID)
				if claim.Error != nil {
					return claim.Error
				}
				if claim.RowsAffected == 0 {
					return errors.New("back relation is already associated with something else")
				}
				backRelationType.BackRelationId = &relationType.ID
			case errors.Is(lookupErr, gorm.ErrRecordNotFound):
				backRelationType.BackRelationId = &relationType.ID
				if err := tx.Save(&backRelationType).Error; err != nil {
					return err
				}
			default:
				return lookupErr
			}

			relationType.BackRelationId = &backRelationType.ID

			return tx.Save(&relationType).Error
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionCreate, "relationType", &relationType.ID, relationType.Name, "Created relation type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeRelationType)
	}
	return &relationType, err
}

func (ctx *MahresourcesContext) EditRelationType(query *query_models.RelationshipTypeEditorQuery) (*models.GroupRelationType, error) {
	if err := ctx.requireEditorRole("editing a relation type"); err != nil {
		return nil, err
	}
	// Re-pointing a relation type's categories deletes every edge that no longer
	// matches, database-wide. See refuseGlobalCascadeWhenScoped.
	if err := ctx.refuseGlobalCascadeWhenScoped("editing a relation type"); err != nil {
		return nil, err
	}

	var relationType = models.GroupRelationType{}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&relationType, query.Id).Error; err != nil {
			return err
		}

		if query.Name != "" {
			relationType.Name = query.Name
		}
		relationType.Description = query.Description
		if query.FromCategory != 0 {
			relationType.FromCategoryId = &query.FromCategory
		}
		if query.ToCategory != 0 {
			relationType.ToCategoryId = &query.ToCategory
		}

		// Enforce self-referential invariant: FromCategory must equal ToCategory
		if relationType.BackRelationId != nil && *relationType.BackRelationId == relationType.ID {
			if relationType.FromCategoryId != nil && relationType.ToCategoryId != nil &&
				*relationType.FromCategoryId != *relationType.ToCategoryId {
				return errors.New("self-referential relation types require FromCategory == ToCategory")
			}
		}

		// Delete GroupRelation records whose groups no longer match the updated category constraints
		if query.FromCategory != 0 {
			if err := tx.Where("relation_type_id = ? AND from_group_id IN (SELECT id FROM groups WHERE category_id IS NULL OR category_id != ?)",
				relationType.ID, query.FromCategory).Delete(&models.GroupRelation{}).Error; err != nil {
				return err
			}
		}
		if query.ToCategory != 0 {
			if err := tx.Where("relation_type_id = ? AND to_group_id IN (SELECT id FROM groups WHERE category_id IS NULL OR category_id != ?)",
				relationType.ID, query.ToCategory).Delete(&models.GroupRelation{}).Error; err != nil {
				return err
			}
		}

		// Also clean up back-relation records if this type has a back-relation
		if relationType.BackRelationId != nil {
			if query.FromCategory != 0 {
				// Forward relations with wrong from_group category were deleted.
				// Their back-relations have those same groups as to_group.
				if err := tx.Where("relation_type_id = ? AND to_group_id IN (SELECT id FROM groups WHERE category_id IS NULL OR category_id != ?)",
					*relationType.BackRelationId, query.FromCategory).Delete(&models.GroupRelation{}).Error; err != nil {
					return err
				}
			}
			if query.ToCategory != 0 {
				// Forward relations with wrong to_group category were deleted.
				// Their back-relations have those same groups as from_group.
				if err := tx.Where("relation_type_id = ? AND from_group_id IN (SELECT id FROM groups WHERE category_id IS NULL OR category_id != ?)",
					*relationType.BackRelationId, query.ToCategory).Delete(&models.GroupRelation{}).Error; err != nil {
					return err
				}
			}
		}

		return tx.Save(&relationType).Error
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "relationType", &relationType.ID, relationType.Name, "Updated relation type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeRelationType)
	}
	return &relationType, err
}

func (ctx *MahresourcesContext) GetRelationTypes(offset int, maxResults int, query *query_models.RelationshipTypeQuery) ([]*models.GroupRelationType, error) {
	var groupRelationTypes []*models.GroupRelationType

	return groupRelationTypes, ctx.db.Scopes(database_scopes.RelationTypeQuery(query)).Limit(maxResults).Offset(offset).Find(&groupRelationTypes).Error
}

func (ctx *MahresourcesContext) GetRelationTypesCount(query *query_models.RelationshipTypeQuery) (int64, error) {
	var relationType models.GroupRelationType
	var count int64

	return count, ctx.db.Scopes(database_scopes.RelationTypeQuery(query)).Model(&relationType).Count(&count).Error
}

func (ctx *MahresourcesContext) GetRelations(offset int, maxResults int, query *query_models.GroupRelationshipQuery) ([]*models.GroupRelation, error) {
	var groupRelations []*models.GroupRelation

	db, deny := ctx.scopeRelationQuery(ctx.db.Scopes(database_scopes.RelationQuery(query)))
	if deny {
		return []*models.GroupRelation{}, nil
	}
	return groupRelations, db.Preload(clause.Associations, pageLimit).Limit(maxResults).Offset(offset).Find(&groupRelations).Error
}

func (ctx *MahresourcesContext) GetRelationsCount(query *query_models.GroupRelationshipQuery) (int64, error) {
	var count int64
	db, deny := ctx.scopeRelationQuery(ctx.db.Scopes(database_scopes.RelationQuery(query)))
	if deny {
		return 0, nil
	}
	return count, db.Model(&models.GroupRelation{}).Count(&count).Error
}

// scopeRelationQuery confines a group-relation query to relations whose BOTH
// endpoints are inside the caller's subtree. group_relations is not an
// owner-bearing table, so the GORM scope callback never touches it; without this
// a group-limited principal would learn the IDs and existence of out-of-subtree
// groups via the raw from_group_id/to_group_id columns (the preloaded group
// objects are nil-filtered, but the integer FKs are not). Returns deny=true
// (fail-closed) when the subtree could not be resolved. No-op for unscoped/admin.
func (ctx *MahresourcesContext) scopeRelationQuery(db *gorm.DB) (*gorm.DB, bool) {
	ids, isScoped, mustDeny := ctx.subtreeScopeIDs()
	if mustDeny {
		return db, true
	}
	if isScoped {
		return db.Where("from_group_id IN ? AND to_group_id IN ?", ids, ids), false
	}
	return db, false
}

func (ctx *MahresourcesContext) GetRelationTypesWithIds(ids *[]uint) ([]*models.GroupRelationType, error) {
	var relationTypes []*models.GroupRelationType

	if len(*ids) == 0 {
		return relationTypes, nil
	}

	return relationTypes, ctx.db.Find(&relationTypes, ids).Error
}

func (ctx *MahresourcesContext) DeleteRelationship(relationshipId uint) error {
	if err := ctx.requireEditorRole("deleting a relation"); err != nil {
		return err
	}

	var relation models.GroupRelation
	if err := ctx.db.First(&relation, relationshipId).Error; err != nil {
		return err
	}
	// Before the back-relation sweep below, not after: that sweep deletes a
	// second row selected by group ids, so a principal that may not touch this
	// edge has to be stopped ahead of it. See relationInScope.
	if !ctx.relationInScope(&relation) {
		return gorm.ErrRecordNotFound
	}
	relationName := relation.Name

	// Check if the relation type has a back-relation type
	var relationType models.GroupRelationType
	if relation.RelationTypeId != nil {
		if err := ctx.db.First(&relationType, *relation.RelationTypeId).Error; err == nil {
			if relationType.BackRelationId != nil && relation.FromGroupId != nil && relation.ToGroupId != nil {
				// Delete the auto-created back-relation (swapped groups, back-relation type)
				ctx.db.Where("from_group_id = ? AND to_group_id = ? AND relation_type_id = ?",
					*relation.ToGroupId, *relation.FromGroupId, *relationType.BackRelationId).
					Delete(&models.GroupRelation{})
			}
		}
	}

	err := ctx.db.Select(clause.Associations).Delete(&relation).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "relation", &relationshipId, relationName, "Deleted relation", nil)
	}
	return err
}

func (ctx *MahresourcesContext) DeleteRelationshipType(relationshipTypeId uint) error {
	if err := ctx.requireEditorRole("deleting a relation type"); err != nil {
		return err
	}
	// Deleting a relation type deletes every edge of that type, everywhere.
	if err := ctx.refuseGlobalCascadeWhenScoped("deleting a relation type"); err != nil {
		return err
	}

	// One transaction, unlike the sequence this used to be. The edge cascades
	// below were standalone statements, so a failure between them left the
	// database holding a relation type whose edges were already gone, or
	// back-relation pointers cleared for a type that then survived. It also
	// means globalCascadeDeleteCallback refusing the final delete rolls the
	// cascades back rather than leaving them committed.
	var relationType models.GroupRelationType
	var relationTypeName string
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&relationType, relationshipTypeId).Error; err != nil {
			return err
		}
		relationTypeName = relationType.Name

		// Delete GroupRelation records of the back-relation type (orphaned mirrors
		// whose forward relations will be deleted). Must happen before clearing
		// BackRelationId so we still know which type to clean up.
		if relationType.BackRelationId != nil && *relationType.BackRelationId != relationshipTypeId {
			if err := tx.Where("relation_type_id = ?", *relationType.BackRelationId).
				Delete(&models.GroupRelation{}).Error; err != nil {
				return err
			}
		}

		// Clear BackRelationId on any relation type that references this one,
		// since SQLite FK constraints (OnDelete:SET NULL) don't fire reliably.
		if err := tx.Model(&models.GroupRelationType{}).
			Where("back_relation_id = ?", relationshipTypeId).
			Update("back_relation_id", nil).Error; err != nil {
			return err
		}

		// Clear our own BackRelationId to avoid circular FK issues during deletion
		if relationType.BackRelationId != nil {
			if err := tx.Model(&relationType).Update("back_relation_id", nil).Error; err != nil {
				return err
			}
		}

		// Explicitly delete GroupRelation records that reference this type,
		// since SQLite FK cascades (OnDelete:CASCADE) don't fire reliably.
		if err := tx.Where("relation_type_id = ?", relationshipTypeId).
			Delete(&models.GroupRelation{}).Error; err != nil {
			return err
		}

		return tx.Select(clause.Associations).Delete(&relationType).Error
	})
	if err != nil {
		return err
	}

	// Outside the transaction, deliberately. Logger.log writes through the
	// PARENT handle, not tx, so logging from inside would ask the pool for a
	// second connection while this transaction holds one — a deadlock at
	// -max-db-connections=1 and lock contention above it. Invalidating the cache
	// after commit rather than before is the same argument: neither should
	// happen for a transaction that then rolls back.
	ctx.Logger().Info(models.LogActionDelete, "relationType", &relationshipTypeId, relationTypeName, "Deleted relation type", nil)
	ctx.InvalidateSearchCacheByType(EntityTypeRelationType)
	return nil
}

// categoryNameFor resolves a category id to a quoted name for an error message,
// falling back to the id when the row cannot be read. An error message must not
// be the thing that fails.
func (ctx *MahresourcesContext) categoryNameFor(tx *gorm.DB, id *uint, into *models.Category) string {
	if id == nil {
		return "(none)"
	}
	if err := tx.First(into, *id).Error; err != nil || into.Name == "" {
		return fmt.Sprintf("#%d", *id)
	}
	return fmt.Sprintf("%q", into.Name)
}
