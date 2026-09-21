package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errResourceSeriesChanged  = errors.New("resource series changed while waiting for canonical locks")
	errResourceContentChanged = errors.New("resource content changed after deletion backup")
)

func sameOptionalUint(a, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (ctx *MahresourcesContext) withResourceSeriesRetry(operation string, fn func(*MahresourcesContext) error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = ctx.WithTransaction(fn)
		if !errors.Is(err, errResourceSeriesChanged) {
			return err
		}
	}
	return fmt.Errorf("resource series kept changing during %s: %w", operation, err)
}

// GetSeries retrieves a series by ID with preloaded resources.
func (ctx *MahresourcesContext) GetSeries(id uint) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Preload("Resources", pageLimit).First(&series, id).Error
}

// GetSeriesCount returns the total count of series.
func (ctx *MahresourcesContext) GetSeriesCount() (int64, error) {
	var count int64
	return count, ctx.db.Model(&models.Series{}).Count(&count).Error
}

// GetSeriesBySlug retrieves a series by its unique slug.
func (ctx *MahresourcesContext) GetSeriesBySlug(slug string) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Where("slug = ?", slug).Preload("Resources", pageLimit).First(&series).Error
}

// CreateSeries creates an explicit series. Unlike the resource-upload
// find-or-create path, an explicit series owns its metadata before any resource
// joins it.
func (ctx *MahresourcesContext) CreateSeries(creator *query_models.SeriesCreator) (*models.Series, error) {
	if err := ctx.requireEditorRole("create a series"); err != nil {
		return nil, err
	}
	_, writer := ctx.SeriesCRUD()
	return writer.Create(creator)
}

// UpdateSeries updates a series name and/or meta.
// When meta changes, recomputes effective Meta for all resources in the series.
func (ctx *MahresourcesContext) UpdateSeries(editor *query_models.SeriesEditor) (*models.Series, error) {
	if err := ctx.requireEditorRole("update a series"); err != nil {
		return nil, err
	}
	var series models.Series

	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		query := seriesWriteQuery(tx).Preload("Resources")
		if err := query.First(&series, editor.ID).Error; err != nil {
			return err
		}

		oldMeta := series.Meta
		if editor.Name != "" {
			trimmed := strings.TrimSpace(editor.Name)
			if trimmed == "" {
				return errors.New("series name must be non-empty")
			}
			series.Name = trimmed
		}

		metaChanged := false
		if editor.Meta != "" {
			normalizedMeta := strings.TrimSpace(editor.Meta)
			if normalizedMeta == "" {
				normalizedMeta = "{}"
			}
			if err := ValidateMeta(normalizedMeta); err != nil {
				return err
			}
			series.Meta = types.JSON(normalizedMeta)
			metaChanged = string(oldMeta) != normalizedMeta
		}

		// Resources were preloaded only to recompute their effective metadata.
		// Never save the association with the Series row: a concurrent membership
		// move is governed by the shared row lock and its own Resource write.
		if err := tx.Omit("Resources").Save(&series).Error; err != nil {
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
		if err := seriesWriteQuery(tx).Preload("Resources").First(&series, id).Error; err != nil {
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
	return ctx.withResourceSeriesRetry("series removal", func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		var resource models.Resource
		if err := tx.First(&resource, resourceID).Error; err != nil {
			return err
		}
		if resource.SeriesID == nil {
			return errors.New("resource is not in a series")
		}
		seriesID := *resource.SeriesID

		lockedSeries, err := lockSeriesRowsForWrite(tx, seriesID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var current models.Resource
				if readErr := tx.First(&current, resourceID).Error; readErr != nil {
					return readErr
				}
				if !sameOptionalUint(resource.SeriesID, current.SeriesID) {
					return errResourceSeriesChanged
				}
			}
			return err
		}
		series := lockedSeries[seriesID]

		resourceQuery := tx
		if tx.Dialector.Name() == "postgres" {
			resourceQuery = resourceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var lockedResource models.Resource
		if err := resourceQuery.First(&lockedResource, resourceID).Error; err != nil {
			return err
		}
		if !sameOptionalUint(resource.SeriesID, lockedResource.SeriesID) {
			return errResourceSeriesChanged
		}
		resource = lockedResource

		// Merge meta back (resource wins): series meta as base, OwnMeta on top.
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

// seriesWriteQuery serializes every operation that derives state from a Series
// row and then writes either that row or one of its Resources. Without the shared
// lock, a metadata patch can preload its old members while a concurrent upload
// derives effective metadata from the old Series value; whichever commits last
// leaves the newly assigned Resource permanently inconsistent.
//
// SQLite rejects FOR UPDATE and already serializes writers, so the clause is
// PostgreSQL-only.
func seriesWriteQuery(tx *gorm.DB) *gorm.DB {
	if tx.Dialector.Name() == "postgres" {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}

// lockSeriesRowsForWrite takes every named Series row in ascending id order and
// returns fresh snapshots from inside the lock. Multi-Series moves must never
// acquire destination then source: reciprocal moves would take opposite orders
// and deadlock.
func lockSeriesRowsForWrite(tx *gorm.DB, ids ...uint) (map[uint]*models.Series, error) {
	unique := make(map[uint]struct{}, len(ids))
	ordered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 0 {
		return map[uint]*models.Series{}, nil
	}

	var rows []models.Series
	if err := seriesWriteQuery(tx).Where("id IN ?", ordered).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(ordered) {
		return nil, gorm.ErrRecordNotFound
	}
	locked := make(map[uint]*models.Series, len(rows))
	for i := range rows {
		locked[rows[i].ID] = &rows[i]
	}
	return locked, nil
}

// ensureSeriesForResource resolves the id and creator result without retaining
// a row lock. Callers that already belong to another Series need the id first so
// they can lock source and destination together in canonical order.
func (ctx *MahresourcesContext) ensureSeriesForResource(tx *gorm.DB, slug string) (*models.Series, bool, error) {
	actor := ctx.actingUserIDPtr()
	var insertResult *gorm.DB
	switch ctx.Config.DbType {
	case constants.DbTypePosgres:
		insertResult = tx.Exec("INSERT INTO series (name, slug, meta, created_by_user_id, created_at, updated_at) VALUES (?, ?, '{}', ?, NOW(), NOW()) ON CONFLICT (slug) DO NOTHING", slug, slug, actor)
	default:
		insertResult = tx.Exec("INSERT OR IGNORE INTO series (name, slug, meta, created_by_user_id, created_at, updated_at) VALUES (?, ?, '{}', ?, datetime('now'), datetime('now'))", slug, slug, actor)
	}
	if insertResult.Error != nil {
		return nil, false, fmt.Errorf("failed to insert series: %w", insertResult.Error)
	}
	var series models.Series
	if err := tx.Where("slug = ?", slug).First(&series).Error; err != nil {
		return nil, false, fmt.Errorf("failed to fetch series with slug %q: %w", slug, err)
	}
	return &series, insertResult.RowsAffected > 0, nil
}

// GetOrCreateSeriesForResource handles the concurrent-safe series assignment
// during resource creation. Returns the series and whether this resource is
// the series creator (should donate all meta to series).
func (ctx *MahresourcesContext) GetOrCreateSeriesForResource(tx *gorm.DB, slug string) (*models.Series, bool, error) {
	series, isCreator, err := ctx.ensureSeriesForResource(tx, slug)
	if err != nil {
		return nil, false, err
	}
	locked, err := lockSeriesRowsForWrite(tx, series.ID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to lock series with slug %q: %w", slug, err)
	}
	return locked[series.ID], isCreator, nil
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
			// Another request already claimed creator; refetch under the same lock
			// used by metadata patches and act as joiner.
			if err := seriesWriteQuery(tx).First(series, series.ID).Error; err != nil {
				return err
			}
			isCreator = false
		}
	}

	if !isCreator {
		// Compute OwnMeta from what the resource submitted, then persist the
		// effective value immediately. A resource joining a series with {} must
		// inherit the series metadata now, not only after a later series edit.
		ownMeta, err := computeOwnMeta(resource.Meta, series.Meta)
		if err != nil {
			return err
		}
		resource.OwnMeta = ownMeta
		effectiveMeta, err := mergeMeta(series.Meta, ownMeta)
		if err != nil {
			return err
		}
		resource.Meta = effectiveMeta
	}

	seriesID := series.ID
	result := tx.Model(&models.Resource{}).
		Where("id = ?", resource.ID).
		Updates(map[string]interface{}{
			"series_id": seriesID,
			"own_meta":  resource.OwnMeta,
			"meta":      resource.Meta,
		})
	if result.Error != nil {
		return result.Error
	}
	// Update through a detached model: callers editing an existing Resource may
	// still carry its preloaded old Series, and GORM's association callbacks can
	// otherwise replay that stale foreign key over the requested membership.
	resource.SeriesID = &seriesID
	resource.Series = series
	return nil
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

	// Merge: start with base, overlay wins.
	// A nil value in overlay means "remove this key" (explicit null override).
	for k, v := range overlayMap {
		if v == nil {
			delete(baseMap, k)
		} else {
			baseMap[k] = v
		}
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged meta: %w", err)
	}

	return types.JSON(result), nil
}

// computeOwnMeta computes the resource's own meta: keys where the resource
// value differs from the series, plus keys not present in the series.
// When recordRemovals is true, series keys absent from the resource are
// recorded as explicit nil (null) overrides so that mergeMeta will delete
// them. This should only be set when the caller knows the resource meta
// was explicitly authored by the user (e.g. an edit), not when computing
// a delta during a series assignment or move.
func computeOwnMeta(resourceMeta, seriesMeta types.JSON, recordRemovals ...bool) (types.JSON, error) {
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

	// Record explicit null overrides for series keys that were
	// intentionally removed from the resource (present in series but
	// absent from resource). Only when the caller signals that the
	// resource meta was explicitly authored by the user.
	if len(recordRemovals) > 0 && recordRemovals[0] {
		for k := range seriesMap {
			if _, inResource := resourceMap[k]; !inResource {
				ownMap[k] = nil
			}
		}
	}

	result, err := json.Marshal(ownMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal own meta: %w", err)
	}

	return types.JSON(result), nil
}
