package application_context

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) EditRelation(query query_models.GroupRelationshipQuery) (*models.GroupRelation, error) {
	var relation = &models.GroupRelation{ID: query.Id}

	if err := ctx.db.First(relation).Error; err != nil {
		return nil, err
	}

	relation.Name = query.Name
	relation.Description = query.Description

	if err := ctx.db.Save(relation).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "relation", &relation.ID, relation.Name, "Updated relation", nil)

	return relation, nil
}

func (ctx *MahresourcesContext) AddRelation(fromGroupId, toGroupId, relationTypeId uint) (*models.GroupRelation, error) {
	var relationType models.GroupRelationType
	var fromGroup models.Group
	var toGroup models.Group
	var relation models.GroupRelation

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

		if *toGroup.CategoryId != *relationType.ToCategoryId || *fromGroup.CategoryId != *relationType.FromCategoryId {
			return errors.New("category mismatch")
		}

		relation = models.GroupRelation{
			FromGroupId:    &fromGroup.ID,
			ToGroupId:      &toGroup.ID,
			RelationTypeId: &relationType.ID,
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

	return &relation, ctx.db.Preload(clause.Associations, pageLimit).First(&relation, id).Error
}

func (ctx *MahresourcesContext) GetRelationType(id uint) (*models.GroupRelationType, error) {
	var relationType models.GroupRelationType
	ctx.db.Preload(clause.Associations, pageLimit).First(&relationType, id)

	return &relationType, ctx.db.Preload(clause.Associations, pageLimit).First(&relationType, id).Error
}

func (ctx *MahresourcesContext) AddRelationType(query *query_models.RelationshipTypeEditorQuery) (*models.GroupRelationType, error) {
	var relationType = models.GroupRelationType{
		Name:           query.Name,
		FromCategoryId: &query.FromCategory,
		ToCategoryId:   &query.ToCategory,
		Description:    query.Description,
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&relationType).Error; err != nil {
			return err
		}

		if query.ReverseName != "" {
			if query.ReverseName == query.Name {
				relationType.BackRelationId = &relationType.ID

				return tx.Save(&relationType).Error
			}

			backRelationType := models.GroupRelationType{
				Name:           query.ReverseName,
				FromCategoryId: &query.ToCategory,
				ToCategoryId:   &query.FromCategory,
			}

			if err := tx.Where(&backRelationType).First(&backRelationType).Error; err == nil {
				if backRelationType.BackRelationId != nil {
					return errors.New("back relation is already associated with something else")
				}
			}

			backRelationType.BackRelationId = &relationType.ID

			if err := tx.Save(&backRelationType).Error; err != nil {
				return err
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
	var relationType = models.GroupRelationType{}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.db.First(&relationType, query.Id).Error; err != nil {
			return err
		}

		relationType.Name = query.Name
		relationType.Description = query.Description

		return ctx.db.Save(&relationType).Error
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "relationType", &relationType.ID, relationType.Name, "Updated relation type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeRelationType)
	}
	return &relationType, err
}

func (ctx *MahresourcesContext) GetRelationTypes(offset int, maxResults int, query *query_models.RelationshipTypeQuery) (*[]*models.GroupRelationType, error) {
	var groupRelationTypes []*models.GroupRelationType

	return &groupRelationTypes, ctx.db.Scopes(database_scopes.RelationTypeQuery(query)).Limit(maxResults).Offset(offset).Find(&groupRelationTypes).Error
}

func (ctx *MahresourcesContext) GetRelationTypesCount(query *query_models.RelationshipTypeQuery) (int64, error) {
	var relationType models.GroupRelationType
	var count int64

	return count, ctx.db.Scopes(database_scopes.RelationTypeQuery(query)).Model(&relationType).Count(&count).Error
}

func (ctx *MahresourcesContext) GetRelations(offset int, maxResults int, query *query_models.GroupRelationshipQuery) (*[]*models.GroupRelation, error) {
	var groupRelations []*models.GroupRelation

	return &groupRelations, ctx.db.Scopes(database_scopes.RelationQuery(query)).Preload(clause.Associations, pageLimit).Limit(maxResults).Offset(offset).Find(&groupRelations).Error
}

func (ctx *MahresourcesContext) GetRelationsCount(query *query_models.GroupRelationshipQuery) (int64, error) {
	var groupRelation models.GroupRelation
	var count int64
	err := ctx.db.Scopes(database_scopes.RelationQuery(query)).Model(&groupRelation).Count(&count).Error

	return count, err
}

func (ctx *MahresourcesContext) GetRelationTypesWithIds(ids *[]uint) (*[]*models.GroupRelationType, error) {
	var relationTypes []*models.GroupRelationType

	if len(*ids) == 0 {
		return &relationTypes, nil
	}

	return &relationTypes, ctx.db.Find(&relationTypes, ids).Error
}

func (ctx *MahresourcesContext) DeleteRelationship(relationshipId uint) error {
	relation := models.GroupRelation{ID: relationshipId}

	err := ctx.db.Select(clause.Associations).Delete(&relation).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "relation", &relationshipId, "", "Deleted relation", nil)
	}
	return err
}

func (ctx *MahresourcesContext) DeleteRelationshipType(relationshipTypeId uint) error {
	relationType := models.GroupRelationType{ID: relationshipTypeId}

	err := ctx.db.Select(clause.Associations).Delete(&relationType).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "relationType", &relationshipTypeId, "", "Deleted relation type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeRelationType)
	}
	return err
}
