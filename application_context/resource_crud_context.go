package application_context

import (
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"

	"gorm.io/gorm/clause"
)

func (ctx *MahresourcesContext) GetResource(id uint) (*models.Resource, error) {
	var resource models.Resource

	return &resource, ctx.db.Preload(clause.Associations, pageLimit).First(&resource, id).Error
}

func (ctx *MahresourcesContext) GetSimilarResources(id uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	// Find all resource IDs similar to this one from pre-computed similarities
	var similarIDs []uint

	// Query both directions using UNION ALL for better index utilization.
	// We store with ResourceID1 < ResourceID2, so we need to check both columns.
	rows, err := ctx.db.Raw(`
		SELECT resource_id2 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id1 = ?
		UNION ALL
		SELECT resource_id1 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id2 = ?
		ORDER BY hamming_distance ASC
	`, id, id).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var similarID uint
		var hammingDistance int
		if err := rows.Scan(&similarID, &hammingDistance); err != nil {
			return nil, err
		}
		similarIDs = append(similarIDs, similarID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(similarIDs) == 0 {
		// Fall back to exact hash match for resources not yet processed by worker
		hashQuery := ctx.db.Table("image_hashes rootHash").
			Select("d_hash").
			Where("rootHash.resource_id = ?", id).
			Limit(1)

		sameHashIdsQuery := ctx.db.Table("image_hashes").
			Select("resource_id").
			Group("resource_id").
			Where("d_hash = (?)", hashQuery)

		return &resources, ctx.db.
			Preload("Tags").
			Joins("Owner").
			Where("resources.id IN (?)", sameHashIdsQuery).
			Where("resources.id <> ?", id).
			Find(&resources).Error
	}

	// Fetch resources
	if err := ctx.db.
		Preload("Tags").
		Joins("Owner").
		Where("resources.id IN ?", similarIDs).
		Find(&resources).Error; err != nil {
		return nil, err
	}

	// Preserve order from similarity query (sorted by hamming_distance ASC)
	idToIndex := make(map[uint]int, len(similarIDs))
	for i, id := range similarIDs {
		idToIndex[id] = i
	}

	sortedResources := make([]*models.Resource, len(similarIDs))
	for i := range resources {
		sortedResources[idToIndex[resources[i].ID]] = resources[i]
	}

	// Filter out any nil entries (in case of missing resources)
	result := make([]*models.Resource, 0, len(sortedResources))
	for _, r := range sortedResources {
		if r != nil {
			result = append(result, r)
		}
	}

	return &result, nil
}

func (ctx *MahresourcesContext) GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error) {
	var resource models.Resource
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).Model(&resource).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) (*[]models.Resource, error) {
	var resources []models.Resource
	resLimit := maxResults

	if query.MaxResults > 0 {
		resLimit = int(query.MaxResults)
	}

	return &resources, ctx.db.Scopes(database_scopes.ResourceQuery(query, false, ctx.db)).
		Limit(resLimit).
		Offset(offset).
		Preload("Tags").
		Preload("Owner").
		Find(&resources).
		Error
}

func (ctx *MahresourcesContext) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	if len(*ids) == 0 {
		return &resources, nil
	}

	return &resources, ctx.db.Find(&resources, ids).Preload("Tags").Error
}

func (ctx *MahresourcesContext) EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var resource models.Resource

	if err := tx.Preload(clause.Associations, pageLimit).First(&resource, resourceQuery.ID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Groups").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Tags").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Notes").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)
	if err := tx.Model(&resource).Association("Groups").Append(&groups); err != nil {
		tx.Rollback()
		return nil, err
	}

	notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)
	if err := tx.Model(&resource).Association("Notes").Append(&notes); err != nil {
		tx.Rollback()
		return nil, err
	}

	tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)
	if err := tx.Model(&resource).Association("Tags").Append(&tags); err != nil {
		tx.Rollback()
		return nil, err
	}

	resource.Name = resourceQuery.Name
	if resourceQuery.Meta != "" {
		resource.Meta = []byte(resourceQuery.Meta)
	}
	resource.Description = resourceQuery.Description
	resource.OriginalName = resourceQuery.OriginalName
	resource.OriginalLocation = resourceQuery.OriginalLocation
	resource.Category = resourceQuery.Category
	resource.ContentCategory = resourceQuery.ContentCategory
	resource.OwnerId = &resourceQuery.OwnerId
	resource.Owner = &models.Group{ID: resourceQuery.OwnerId}

	if err := tx.Save(resource).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "resource", &resource.ID, resource.Name, "Updated resource", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return &resource, nil
}
