package application_context

import (
	"fmt"
	"net/http"

	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func (ctx *MahresourcesContext) GetResource(id uint) (*models.Resource, error) {
	var resource models.Resource

	return &resource, ctx.db.Preload(clause.Associations).First(&resource, id).Error
}

// GetResourceByID returns a resource without preloading associations.
// Use this for internal operations that only need the resource entity itself.
func (ctx *MahresourcesContext) GetResourceByID(id uint) (*models.Resource, error) {
	var resource models.Resource
	return &resource, ctx.db.First(&resource, id).Error
}

func (ctx *MahresourcesContext) GetSimilarResources(id uint) ([]*models.Resource, error) {
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

		return resources, ctx.db.
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

	return result, nil
}

func (ctx *MahresourcesContext) GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error) {
	var resource models.Resource
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).Model(&resource).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) ([]models.Resource, error) {
	var resources []models.Resource
	resLimit := maxResults

	if query.MaxResults > 0 {
		resLimit = int(query.MaxResults)
	}

	return resources, ctx.db.Scopes(database_scopes.ResourceQuery(query, false, ctx.db)).
		Limit(resLimit).
		Offset(offset).
		Preload("Tags").
		Preload("Owner").
		Preload("ResourceCategory").
		Preload("Series").
		Find(&resources).
		Error
}

func (ctx *MahresourcesContext) GetResourcesWithIds(ids *[]uint) ([]*models.Resource, error) {
	var resources []*models.Resource

	if len(*ids) == 0 {
		return resources, nil
	}

	return resources, ctx.db.Preload("Tags").Find(&resources, ids).Error
}

func (ctx *MahresourcesContext) EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error) {
	hookData := map[string]any{
		"id":          float64(resourceQuery.ID),
		"name":        resourceQuery.Name,
		"description": resourceQuery.Description,
		"meta":        resourceQuery.Meta,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_resource_update", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		resourceQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		resourceQuery.Description = desc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		resourceQuery.Meta = hMeta
	}

	var resource models.Resource

	err := ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		tx := altCtx.db

		if err := tx.Preload(clause.Associations, pageLimit).First(&resource, resourceQuery.ID).Error; err != nil {
			return err
		}

		if err := tx.Model(&resource).Association("Groups").Clear(); err != nil {
			return err
		}

		if err := tx.Model(&resource).Association("Tags").Clear(); err != nil {
			return err
		}

		if err := tx.Model(&resource).Association("Notes").Clear(); err != nil {
			return err
		}

		groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)
		if err := tx.Model(&resource).Association("Groups").Append(&groups); err != nil {
			return err
		}

		notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)
		if err := tx.Model(&resource).Association("Notes").Append(&notes); err != nil {
			return err
		}

		tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)
		if err := tx.Model(&resource).Association("Tags").Append(&tags); err != nil {
			return err
		}

		// Ensure Series is loaded if SeriesID is set (clause.Associations with pageLimit may not load it)
		if resource.SeriesID != nil && resource.Series == nil {
			resource.Series = &models.Series{}
			if err := tx.First(resource.Series, *resource.SeriesID).Error; err != nil {
				return err
			}
		}

		resource.Name = resourceQuery.Name
		if resourceQuery.Meta != "" {
			resource.Meta = []byte(resourceQuery.Meta)
			// Recompute OwnMeta if the resource is in a series
			if resource.SeriesID != nil && resource.Series != nil {
				ownMeta, err := computeOwnMeta(resource.Meta, resource.Series.Meta, true)
				if err != nil {
					return err
				}
				resource.OwnMeta = ownMeta
			}
		}
		resource.Description = resourceQuery.Description
		resource.OriginalName = resourceQuery.OriginalName
		resource.OriginalLocation = resourceQuery.OriginalLocation
		resource.Category = resourceQuery.Category
		resource.ContentCategory = resourceQuery.ContentCategory
		resource.ResourceCategoryId = uintPtrOrNil(resourceQuery.ResourceCategoryId)
		if resourceQuery.Width != 0 {
			resource.Width = resourceQuery.Width
		}
		if resourceQuery.Height != 0 {
			resource.Height = resourceQuery.Height
		}
		resource.OwnerId = uintPtrOrNil(resourceQuery.OwnerId)
		if resourceQuery.OwnerId != 0 {
			resource.Owner = &models.Group{ID: resourceQuery.OwnerId}
		} else {
			resource.Owner = nil
		}

		// Handle series assignment changes
		// Capture old series ID before any mutations so auto-delete logic can
		// clean up the previous series when the resource moves to a new one.
		oldSeriesID := resource.SeriesID

		// Resolve SeriesSlug to SeriesId if provided (matches create-path behavior)
		newSeriesID := resourceQuery.SeriesId
		if newSeriesID == 0 && resourceQuery.SeriesSlug != "" {
			series, isCreator, err := ctx.GetOrCreateSeriesForResource(tx, resourceQuery.SeriesSlug)
			if err != nil {
				return fmt.Errorf("series slug %q: %w", resourceQuery.SeriesSlug, err)
			}
			newSeriesID = series.ID
			// If this resource is the series creator, donate meta to series
			if isCreator {
				if err := ctx.AssignResourceToSeries(tx, &resource, series, true); err != nil {
					return err
				}
			}
		}
		seriesChanged := false

		if newSeriesID > 0 {
			if oldSeriesID == nil || *oldSeriesID != newSeriesID {
				seriesChanged = true
			}
		} else if oldSeriesID != nil && resourceQuery.SeriesSlug == "" {
			// Only remove from series if SeriesSlug wasn't provided
			// (SeriesSlug="" + SeriesId=0 means "not provided" for partial updates)
			seriesChanged = true
		}

		if seriesChanged {
			if newSeriesID > 0 {
				// Assigning to a (new) series
				var newSeries models.Series
				if err := tx.First(&newSeries, newSeriesID).Error; err != nil {
					return fmt.Errorf("series %d not found: %w", newSeriesID, err)
				}
				ownMeta, err := computeOwnMeta(resource.Meta, newSeries.Meta)
				if err != nil {
					return err
				}
				resource.OwnMeta = ownMeta
				effectiveMeta, err := mergeMeta(newSeries.Meta, ownMeta)
				if err != nil {
					return err
				}
				resource.Meta = effectiveMeta
				resource.SeriesID = &newSeries.ID
				resource.Series = &newSeries
			} else {
				// Removing from series - Meta already has effective value
				resource.OwnMeta = types.JSON("{}")
				resource.SeriesID = nil
				resource.Series = nil
			}
		} else if resource.SeriesID != nil && resource.Series != nil &&
			resource.Series.ID != *resource.SeriesID {
			// AssignResourceToSeries may have changed SeriesID without updating
			// the loaded Series association — clear it so Save doesn't revert the FK.
			resource.Series = nil
		}

		if err := tx.Save(resource).Error; err != nil {
			return err
		}

		// Explicitly persist OwnMeta to ensure it's saved even if GORM's
		// Save doesn't detect the change on the JSON field
		if resource.SeriesID != nil || seriesChanged {
			if err := tx.Model(resource).Update("own_meta", resource.OwnMeta).Error; err != nil {
				return err
			}
		}

		// Auto-delete old series if it became empty
		if seriesChanged && oldSeriesID != nil {
			var count int64
			tx.Model(&models.Resource{}).Where("series_id = ?", *oldSeriesID).Count(&count)
			if count == 0 {
				if err := tx.Delete(&models.Series{}, *oldSeriesID).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	ctx.syncMentionsForResource(&resource)

	ctx.Logger().Info(models.LogActionUpdate, "resource", &resource.ID, resource.Name, "Updated resource", nil)

	ctx.RunAfterPluginHooks("after_resource_update", map[string]any{
		"id":          float64(resource.ID),
		"name":        resource.Name,
		"description": resource.Description,
		"meta":        string(resource.Meta),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return &resource, nil
}

// GetSeriesSiblings returns other resources in the same series, ordered by created_at.
func (ctx *MahresourcesContext) GetSeriesSiblings(resourceID uint, seriesID uint) ([]*models.Resource, error) {
	var resources []*models.Resource
	return resources, ctx.db.
		Where("series_id = ? AND id != ?", seriesID, resourceID).
		Order("created_at ASC").
		Find(&resources).Error
}

// GetResourceByHash retrieves a resource by its content hash.
// This is useful for serving resources in contexts where only the hash is known,
// such as shared note resource serving.
func (ctx *MahresourcesContext) GetResourceByHash(hash string) (*models.Resource, error) {
	var resource models.Resource
	if err := ctx.db.Where("hash = ?", hash).First(&resource).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

// ServeResourceByHash serves a resource file by its content hash.
// This is used by the share server to serve resources for shared notes.
func (ctx *MahresourcesContext) ServeResourceByHash(w http.ResponseWriter, r *http.Request, hash string) {
	resource, err := ctx.GetResourceByHash(hash)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Get the appropriate filesystem for this resource
	fs, err := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		http.Error(w, "Storage not found", http.StatusNotFound)
		return
	}

	file, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", resource.ContentType)
	http.ServeContent(w, r, resource.Name, resource.UpdatedAt, file)
}

func uintPtrOrNil(v uint) *uint {
	if v == 0 {
		return nil
	}
	return &v
}
