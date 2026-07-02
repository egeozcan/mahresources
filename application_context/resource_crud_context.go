package application_context

import (
	"errors"
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
	return ctx.getSimilarResourcesLimited(id, 0)
}

// getSimilarResourcesLimited is GetSimilarResources with an optional cap on how many
// similar resources are actually loaded. A limit <= 0 means no cap (the public
// GetSimilarResources behaviour). The cap is applied to the Hamming-sorted similarity
// id list BEFORE the resource+tag preload, so a resource in a large near-duplicate
// cluster does not force loading — and tag-preloading — every similar row. Callers that
// only aggregate a bounded window of the nearest matches (suggested tags) pass a limit
// so the cap bounds the database work, not just the downstream in-memory scoring.
func (ctx *MahresourcesContext) getSimilarResourcesLimited(id uint, limit int) ([]*models.Resource, error) {
	var resources []*models.Resource

	// Find all resource IDs similar to this one from pre-computed similarities
	var similarIDs []uint
	distanceByID := make(map[uint]int)

	// Read-time thresholds (image similarity v2): the write path stores v2 pairs up
	// to MaxStoredPDistance (11) unconditionally, so filtering happens here and a
	// threshold change applies instantly with no recompute. The effective distance
	// is COALESCE(p_distance, hamming_distance): the v2 pHash distance when present,
	// else the legacy dHash distance for old pairs.
	pThreshold, aThreshold := ctx.similarityThresholds()

	// Query both directions using UNION ALL for better index utilization.
	// We store with ResourceID1 < ResourceID2, so we need to check both columns.
	// When bounded, push LIMIT into the query so the similarity scan itself is capped
	// (not just the resource+tag preload below) — this matters for a resource in a very
	// large near-duplicate cluster. ORDER BY + LIMIT apply to the whole compound query in
	// both SQLite and Postgres.
	//
	// The aHash secondary filter only applies when its threshold is nonzero, and it
	// never excludes legacy pairs (a_distance IS NULL passes) so v1 matches survive.
	aClause := ""
	if aThreshold > 0 {
		aClause = fmt.Sprintf(" AND (a_distance IS NULL OR a_distance <= %d)", aThreshold)
	}
	filter := fmt.Sprintf("COALESCE(p_distance, hamming_distance) <= %d%s", pThreshold, aClause)
	query := fmt.Sprintf(`
		SELECT resource_id2 as similar_id, COALESCE(p_distance, hamming_distance) as dist FROM resource_similarities WHERE resource_id1 = ? AND %s
		UNION ALL
		SELECT resource_id1 as similar_id, COALESCE(p_distance, hamming_distance) as dist FROM resource_similarities WHERE resource_id2 = ? AND %s
		ORDER BY dist ASC`, filter, filter)
	queryArgs := []interface{}{id, id}
	if limit > 0 {
		query += "\n\t\tLIMIT ?"
		queryArgs = append(queryArgs, limit)
	}
	rows, err := ctx.db.Raw(query, queryArgs...).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var similarID uint
		var distance int
		if err := rows.Scan(&similarID, &distance); err != nil {
			return nil, err
		}
		similarIDs = append(similarIDs, similarID)
		if _, seen := distanceByID[similarID]; !seen {
			distanceByID[similarID] = distance
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The query already applies LIMIT when bounded; this is a defensive secondary cap so the
	// resource + tag preload below can never exceed `limit` regardless of driver quirks.
	if limit > 0 && len(similarIDs) > limit {
		similarIDs = similarIDs[:limit]
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

		fallback := ctx.db.
			Preload("Tags").
			Joins("Owner").
			Where("resources.id IN (?)", sameHashIdsQuery).
			Where("resources.id <> ?", id)
		if limit > 0 {
			fallback = fallback.Limit(limit)
		}

		return resources, fallback.Find(&resources).Error
	}

	// Fetch resources
	if err := ctx.db.
		Preload("Tags").
		Joins("Owner").
		Where("resources.id IN ?", similarIDs).
		Find(&resources).Error; err != nil {
		return nil, err
	}

	// Preserve order from similarity query (sorted by distance ASC) and attach the
	// perceptual distance so the UI can surface a confidence tier.
	idToIndex := make(map[uint]int, len(similarIDs))
	for i, id := range similarIDs {
		idToIndex[id] = i
	}

	sortedResources := make([]*models.Resource, len(similarIDs))
	for i := range resources {
		r := resources[i]
		if d, ok := distanceByID[r.ID]; ok {
			dist := d
			r.SimilarityDistance = &dist
		}
		sortedResources[idToIndex[r.ID]] = r
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

// similarityThresholds returns the read-time perceptual-similarity thresholds:
// the max effective distance (COALESCE p_distance/hamming_distance) and the max
// aHash distance (0 disables the aHash filter). Falls back to sane defaults when
// runtime settings are not wired (e.g. in isolated unit tests).
func (ctx *MahresourcesContext) similarityThresholds() (int, uint64) {
	if ctx.settings != nil {
		return ctx.settings.HashSimilarityThreshold(), ctx.settings.HashAHashThreshold()
	}
	return 10, 0
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
	if err := ValidateEntityName(resourceQuery.Name, "resource"); err != nil {
		return nil, err
	}

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

		if len(resourceQuery.Groups) > 0 {
			if err := ValidateAssociationIDs[models.Group](tx, resourceQuery.Groups, "groups"); err != nil {
				return err
			}
		}
		if len(resourceQuery.Notes) > 0 {
			if err := ValidateAssociationIDs[models.Note](tx, resourceQuery.Notes, "notes"); err != nil {
				return err
			}
		}
		if len(resourceQuery.Tags) > 0 {
			if err := ValidateAssociationIDs[models.Tag](tx, resourceQuery.Tags, "tags"); err != nil {
				return err
			}
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
			if err := ValidateMeta(resourceQuery.Meta); err != nil {
				return err
			}
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
		resource.ResourceCategoryId = ctx.resourceCategoryIdOrDefault(resourceQuery.ResourceCategoryId)
		if resourceQuery.Width != 0 {
			resource.Width = resourceQuery.Width
		}
		if resourceQuery.Height != 0 {
			resource.Height = resourceQuery.Height
		}
		// Validate the (possibly changed) owner against the caller's scope. The
		// scope UPDATE callback only matches the row by its *current* owner, so
		// without this a group-limited principal could relocate a resource it owns
		// into another subtree (or orphan it). Mirror CreateOrUpdateNote: the new
		// owner must resolve through the (scoped) transaction db, and a scoped
		// principal may not clear the owner. No-op for admins / unscoped users /
		// the auth-off system. Use the transaction's db (tx), never the outer
		// ctx.db — a non-transaction query here would need a second connection and
		// deadlock under a single-connection pool.
		if resourceQuery.OwnerId != 0 {
			var ownerCheck models.Group
			if err := tx.Select("id").First(&ownerCheck, resourceQuery.OwnerId).Error; err != nil {
				return errors.New("owner group not found")
			}
		} else if altCtx.isScopedPrincipal() {
			return errors.New("owner group not found")
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

		// Omit the Owner belongs-to so Save does not upsert the owner group: the
		// owner_id FK column is written regardless, and upserting the stub group
		// {ID} would fire the scope create-callback (the stub has a nil owner),
		// which rejects the edit for a group-limited principal even when the new
		// owner is inside its subtree.
		if err := tx.Omit("Owner").Save(&resource).Error; err != nil {
			return err
		}

		// Explicitly persist OwnMeta to ensure it's saved even if GORM's
		// Save doesn't detect the change on the JSON field
		if resource.SeriesID != nil || seriesChanged {
			if err := tx.Model(&resource).Update("own_meta", resource.OwnMeta).Error; err != nil {
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

func (ctx *MahresourcesContext) resourceCategoryIdOrDefault(v uint) uint {
	if v == 0 {
		return ctx.DefaultResourceCategoryID
	}
	return v
}

// resolveResourceCategory determines the category for a resource.
// If v == 0 (not specified), runs auto-detection. Otherwise uses v as-is.
func (ctx *MahresourcesContext) resolveResourceCategory(v uint, contentType string, width, height uint, fileSize int64) uint {
	if v != 0 {
		return v
	}
	return ctx.detectResourceCategory(contentType, width, height, fileSize)
}
