package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// GetSeries retrieves a series by ID with preloaded resources.
func (ctx *MahresourcesContext) GetSeries(id uint) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Preload("Resources", pageLimit).First(&series, id).Error
}

// GetSeriesBySlug retrieves a series by its unique slug.
func (ctx *MahresourcesContext) GetSeriesBySlug(slug string) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Where("slug = ?", slug).Preload("Resources", pageLimit).First(&series).Error
}

// UpdateSeries updates a series name and/or meta.
// When meta changes, recomputes effective Meta for all resources in the series.
func (ctx *MahresourcesContext) UpdateSeries(editor *query_models.SeriesEditor) (*models.Series, error) {
	var series models.Series

	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		if err := tx.Preload("Resources").First(&series, editor.ID).Error; err != nil {
			return err
		}

		oldMeta := series.Meta
		if editor.Name != "" {
			series.Name = editor.Name
		}

		metaChanged := false
		if editor.Meta != "" {
			series.Meta = types.JSON(editor.Meta)
			metaChanged = string(oldMeta) != editor.Meta
		}

		if err := tx.Save(&series).Error; err != nil {
			return err
		}

		// Recompute effective Meta for all resources if meta changed
		if metaChanged {
			for _, resource := range series.Resources {
				effectiveMeta, err := mergeMeta(series.Meta, resource.OwnMeta)
				if err != nil {
					return err
				}
				if err := tx.Model(resource).Update("meta", effectiveMeta).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "series", &series.ID, series.Name, "Updated series", nil)
	return &series, nil
}

// DeleteSeries merges meta back into all resources, then deletes the series.
func (ctx *MahresourcesContext) DeleteSeries(id uint) error {
	return ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		var series models.Series
		if err := tx.Preload("Resources").First(&series, id).Error; err != nil {
			return err
		}

		// Merge meta back into each resource (resource wins)
		for _, resource := range series.Resources {
			effectiveMeta, err := mergeMeta(series.Meta, resource.OwnMeta)
			if err != nil {
				return err
			}
			if err := tx.Model(resource).Updates(map[string]interface{}{
				"meta":      effectiveMeta,
				"own_meta":  types.JSON("{}"),
				"series_id": nil,
			}).Error; err != nil {
				return err
			}
		}

		if err := tx.Delete(&series).Error; err != nil {
			return err
		}

		txCtx.Logger().Info(models.LogActionDelete, "series", &id, series.Name, "Deleted series", nil)
		return nil
	})
}

// RemoveResourceFromSeries detaches a resource from its series,
// merging series meta back (resource wins). Auto-deletes empty series.
func (ctx *MahresourcesContext) RemoveResourceFromSeries(resourceID uint) error {
	return ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		var resource models.Resource
		if err := tx.First(&resource, resourceID).Error; err != nil {
			return err
		}

		if resource.SeriesID == nil {
			return errors.New("resource is not in a series")
		}

		seriesID := *resource.SeriesID

		var series models.Series
		if err := tx.First(&series, seriesID).Error; err != nil {
			return err
		}

		// Merge meta back (resource wins): series meta as base, OwnMeta on top
		effectiveMeta, err := mergeMeta(series.Meta, resource.OwnMeta)
		if err != nil {
			return err
		}

		if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(map[string]interface{}{
			"meta":      effectiveMeta,
			"own_meta":  types.JSON("{}"),
			"series_id": nil,
		}).Error; err != nil {
			return err
		}

		// Auto-delete series if now empty
		var count int64
		tx.Model(&models.Resource{}).Where("series_id = ?", seriesID).Count(&count)
		if count == 0 {
			if err := tx.Delete(&models.Series{}, seriesID).Error; err != nil {
				return err
			}
			txCtx.Logger().Info(models.LogActionDelete, "series", &seriesID, "", "Auto-deleted empty series", nil)
		}

		txCtx.Logger().Info(models.LogActionUpdate, "resource", &resourceID, resource.Name, "Removed from series", nil)
		return nil
	})
}

// GetOrCreateSeriesForResource handles the concurrent-safe series assignment
// during resource creation. Returns the series and whether this resource is
// the series creator (should donate all meta to series).
func (ctx *MahresourcesContext) GetOrCreateSeriesForResource(tx *gorm.DB, slug string) (*models.Series, bool, error) {
	// Step 1: Insert or ignore (concurrent-safe)
	var insertResult *gorm.DB
	switch ctx.Config.DbType {
	case constants.DbTypePosgres:
		insertResult = tx.Exec("INSERT INTO series (name, slug, meta, created_at, updated_at) VALUES (?, ?, '{}', NOW(), NOW()) ON CONFLICT (slug) DO NOTHING", slug, slug)
	default: // SQLite
		insertResult = tx.Exec("INSERT OR IGNORE INTO series (name, slug, meta, created_at, updated_at) VALUES (?, ?, '{}', datetime('now'), datetime('now'))", slug, slug)
	}
	if insertResult.Error != nil {
		return nil, false, fmt.Errorf("failed to insert series: %w", insertResult.Error)
	}

	// Step 2: Fetch the series
	var series models.Series
	if err := tx.Where("slug = ?", slug).First(&series).Error; err != nil {
		return nil, false, fmt.Errorf("failed to fetch series with slug %q: %w", slug, err)
	}

	// Step 3: Check if this is a fresh series (meta is empty = we're the creator)
	isCreator := len(series.Meta) == 0 || string(series.Meta) == "{}" || string(series.Meta) == "null"

	return &series, isCreator, nil
}

// AssignResourceToSeries assigns a resource to a series during creation.
// If isCreator is true, the resource donates all its meta to the series.
// If false, it computes OwnMeta as the diff from series meta.
func (ctx *MahresourcesContext) AssignResourceToSeries(tx *gorm.DB, resource *models.Resource, series *models.Series, isCreator bool) error {
	if isCreator {
		// Optimistic update: only claim creator if meta is still empty.
		// This prevents two concurrent requests from both becoming "creator"
		// and overwriting each other's meta.
		result := tx.Model(&models.Series{}).
			Where("id = ? AND (meta = '{}' OR meta IS NULL OR meta = 'null')", series.ID).
			Update("meta", resource.Meta)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected > 0 {
			// Successfully claimed creator role
			series.Meta = resource.Meta
			resource.OwnMeta = types.JSON("{}")
		} else {
			// Another request already claimed creator; refetch and act as joiner
			if err := tx.First(series, series.ID).Error; err != nil {
				return err
			}
			isCreator = false
		}
	}

	if !isCreator {
		// Compute OwnMeta: keys that differ from series or don't exist in series
		ownMeta, err := computeOwnMeta(resource.Meta, series.Meta)
		if err != nil {
			return err
		}
		resource.OwnMeta = ownMeta
		// Resource Meta stays unchanged (already the effective value)
	}

	resource.SeriesID = &series.ID
	return tx.Model(resource).Updates(map[string]interface{}{
		"series_id": series.ID,
		"own_meta":  resource.OwnMeta,
	}).Error
}

// mergeMeta merges base (series) meta with overlay (resource own) meta.
// Overlay values win on conflict. Returns the merged JSON.
func mergeMeta(base, overlay types.JSON) (types.JSON, error) {
	baseMap := make(map[string]interface{})
	overlayMap := make(map[string]interface{})

	if len(base) > 0 && string(base) != "null" {
		if err := json.Unmarshal(base, &baseMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base meta: %w", err)
		}
	}

	if len(overlay) > 0 && string(overlay) != "null" {
		if err := json.Unmarshal(overlay, &overlayMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overlay meta: %w", err)
		}
	}

	// Merge: start with base, overlay wins
	for k, v := range overlayMap {
		baseMap[k] = v
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged meta: %w", err)
	}

	return types.JSON(result), nil
}

// computeOwnMeta computes the resource's own meta: keys where the resource
// value differs from the series, plus keys not present in the series.
func computeOwnMeta(resourceMeta, seriesMeta types.JSON) (types.JSON, error) {
	resourceMap := make(map[string]interface{})
	seriesMap := make(map[string]interface{})

	if len(resourceMeta) > 0 && string(resourceMeta) != "null" {
		if err := json.Unmarshal(resourceMeta, &resourceMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource meta: %w", err)
		}
	}

	if len(seriesMeta) > 0 && string(seriesMeta) != "null" {
		if err := json.Unmarshal(seriesMeta, &seriesMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal series meta: %w", err)
		}
	}

	ownMap := make(map[string]interface{})
	for k, v := range resourceMap {
		seriesVal, exists := seriesMap[k]
		if !exists {
			ownMap[k] = v
			continue
		}
		// Compare via JSON marshaling for deep equality
		vJSON, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource meta value for key %q: %w", k, err)
		}
		sJSON, err := json.Marshal(seriesVal)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal series meta value for key %q: %w", k, err)
		}
		if string(vJSON) != string(sJSON) {
			ownMap[k] = v
		}
	}

	result, err := json.Marshal(ownMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal own meta: %w", err)
	}

	return types.JSON(result), nil
}
