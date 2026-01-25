package application_context

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime/multipart"
	"path"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mahresources/models"
	"mahresources/models/query_models"
)

// CountHashReferences counts how many resources and versions reference a given hash
func (ctx *MahresourcesContext) CountHashReferences(hash string) (int64, error) {
	var versionCount int64
	var resourceCount int64

	if err := ctx.db.Model(&models.ResourceVersion{}).Where("hash = ?", hash).Count(&versionCount).Error; err != nil {
		return 0, err
	}

	if err := ctx.db.Model(&models.Resource{}).Where("hash = ?", hash).Count(&resourceCount).Error; err != nil {
		return 0, err
	}

	return versionCount + resourceCount, nil
}

// GetVersions returns all versions for a resource, ordered by version number descending
// If no versions exist (resource not yet migrated), returns a virtual v1 based on current resource state
func (ctx *MahresourcesContext) GetVersions(resourceID uint) ([]models.ResourceVersion, error) {
	var versions []models.ResourceVersion
	err := ctx.db.Where("resource_id = ?", resourceID).Order("version_number DESC").Find(&versions).Error
	if err != nil {
		return nil, err
	}

	// If no versions exist, create a virtual v1 from resource's current state
	if len(versions) == 0 {
		resource, err := ctx.GetResource(resourceID)
		if err != nil {
			return nil, err
		}
		virtualV1 := models.ResourceVersion{
			ResourceID:      resourceID,
			VersionNumber:   1,
			Hash:            resource.Hash,
			HashType:        resource.HashType,
			FileSize:        resource.FileSize,
			ContentType:     resource.ContentType,
			Width:           resource.Width,
			Height:          resource.Height,
			Location:        resource.Location,
			StorageLocation: resource.StorageLocation,
			Comment:         "Current version",
			CreatedAt:       resource.CreatedAt,
		}
		// Note: ID is 0 (not persisted) - UI should handle this
		versions = []models.ResourceVersion{virtualV1}
	}

	return versions, nil
}

// GetVersion returns a specific version by ID
func (ctx *MahresourcesContext) GetVersion(versionID uint) (*models.ResourceVersion, error) {
	var version models.ResourceVersion
	err := ctx.db.First(&version, versionID).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetVersionByNumber returns a specific version by resource ID and version number
func (ctx *MahresourcesContext) GetVersionByNumber(resourceID uint, versionNumber int) (*models.ResourceVersion, error) {
	var version models.ResourceVersion
	err := ctx.db.Where("resource_id = ? AND version_number = ?", resourceID, versionNumber).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// UploadNewVersion uploads a new version of an existing resource
func (ctx *MahresourcesContext) UploadNewVersion(resourceID uint, file multipart.File, header *multipart.FileHeader, comment string) (*models.ResourceVersion, error) {
	// Verify resource exists
	resource, err := ctx.GetResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	// Check if resource has any versions - if not, create v1 from current state first (lazy migration)
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&versionCount)
	if versionCount == 0 {
		v1 := models.ResourceVersion{
			ResourceID:      resourceID,
			VersionNumber:   1,
			Hash:            resource.Hash,
			HashType:        resource.HashType,
			FileSize:        resource.FileSize,
			ContentType:     resource.ContentType,
			Width:           resource.Width,
			Height:          resource.Height,
			Location:        resource.Location,
			StorageLocation: resource.StorageLocation,
			Comment:         "Initial version",
		}
		if err := ctx.db.Create(&v1).Error; err != nil {
			return nil, fmt.Errorf("failed to create initial version: %w", err)
		}
	}

	// Get the next version number
	var maxVersion int
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Select("COALESCE(MAX(version_number), 0)").Scan(&maxVersion)
	nextVersion := maxVersion + 1

	// Process the file
	hash, location, fileSize, contentType, width, height, storageLocation, err := ctx.processFileForVersion(file, header)
	if err != nil {
		return nil, fmt.Errorf("failed to process file: %w", err)
	}

	// Create the version record
	version := models.ResourceVersion{
		ResourceID:      resourceID,
		VersionNumber:   nextVersion,
		Hash:            hash,
		HashType:        "SHA1",
		FileSize:        fileSize,
		ContentType:     contentType,
		Width:           width,
		Height:          height,
		Location:        location,
		StorageLocation: storageLocation,
		Comment:         comment,
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&version).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create version record: %w", err)
	}

	// Update resource's current version AND sync main fields from the new version
	// This ensures thumbnails, previews, and file serving use the current version's content
	resourceUpdates := map[string]interface{}{
		"current_version_id": version.ID,
		"hash":               version.Hash,
		"location":           version.Location,
		"storage_location":   version.StorageLocation,
		"content_type":       version.ContentType,
		"width":              version.Width,
		"height":             version.Height,
		"file_size":          version.FileSize,
	}
	if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(resourceUpdates).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update resource fields: %w", err)
	}

	// Clear cached previews/thumbnails so they regenerate with new content
	if err := tx.Where("resource_id = ?", resourceID).Delete(&models.Preview{}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to clear cached previews: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger().Info(models.LogActionCreate, "resource_version", &version.ID, fmt.Sprintf("v%d for resource %d", nextVersion, resourceID), comment, nil)

	return &version, nil
}

// processFileForVersion handles file storage and returns metadata
func (ctx *MahresourcesContext) processFileForVersion(file multipart.File, header *multipart.FileHeader) (string, string, int64, string, uint, uint, *string, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return "", "", 0, "", 0, 0, nil, err
	}

	hash := computeSHA1(content)
	fileSize := int64(len(content))
	contentType := detectContentType(content)
	width, height := getDimensionsFromContent(content, contentType)
	ext := getExtensionFromFilename(header.Filename, contentType)
	location := buildVersionResourcePath(hash, ext)

	// Deduplication: only store if file doesn't exist
	if exists, _ := afero.Exists(ctx.fs, location); !exists {
		if err := ctx.storeVersionFile(location, content); err != nil {
			return "", "", 0, "", 0, 0, nil, err
		}
	}

	return hash, location, fileSize, contentType, width, height, nil, nil
}

func (ctx *MahresourcesContext) storeVersionFile(location string, content []byte) error {
	dir := path.Dir(location)
	if err := ctx.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := ctx.fs.Create(location)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(content)
	return err
}

func computeSHA1(content []byte) string {
	h := sha1.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func detectContentType(content []byte) string {
	mime := mimetype.Detect(content)
	return mime.String()
}

func getDimensionsFromContent(content []byte, contentType string) (uint, uint) {
	if !strings.HasPrefix(contentType, "image/") {
		return 0, 0
	}
	reader := bytes.NewReader(content)
	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0
	}
	return uint(config.Width), uint(config.Height)
}

func getExtensionFromFilename(filename, contentType string) string {
	ext := path.Ext(filename)
	if ext != "" {
		return ext
	}
	mime := mimetype.Lookup(contentType)
	if mime != nil {
		return mime.Extension()
	}
	return ""
}

func buildVersionResourcePath(hash, ext string) string {
	return fmt.Sprintf("/resources/%s/%s/%s/%s%s", hash[0:2], hash[2:4], hash[4:6], hash, ext)
}

// RestoreVersion creates a new version by copying metadata from an old version
func (ctx *MahresourcesContext) RestoreVersion(resourceID, versionID uint, comment string) (*models.ResourceVersion, error) {
	sourceVersion, err := ctx.GetVersion(versionID)
	if err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}

	if sourceVersion.ResourceID != resourceID {
		return nil, errors.New("version does not belong to this resource")
	}

	var maxVersion int
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Select("COALESCE(MAX(version_number), 0)").Scan(&maxVersion)
	nextVersion := maxVersion + 1

	if comment == "" {
		comment = fmt.Sprintf("Restored from version %d", sourceVersion.VersionNumber)
	}

	version := models.ResourceVersion{
		ResourceID:      resourceID,
		VersionNumber:   nextVersion,
		Hash:            sourceVersion.Hash,
		HashType:        sourceVersion.HashType,
		FileSize:        sourceVersion.FileSize,
		ContentType:     sourceVersion.ContentType,
		Width:           sourceVersion.Width,
		Height:          sourceVersion.Height,
		Location:        sourceVersion.Location,
		StorageLocation: sourceVersion.StorageLocation,
		Comment:         comment,
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&version).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create version record: %w", err)
	}

	// Update resource's current version AND sync main fields from the restored version
	// This ensures thumbnails, previews, and file serving use the current version's content
	resourceUpdates := map[string]interface{}{
		"current_version_id": version.ID,
		"hash":               version.Hash,
		"location":           version.Location,
		"storage_location":   version.StorageLocation,
		"content_type":       version.ContentType,
		"width":              version.Width,
		"height":             version.Height,
		"file_size":          version.FileSize,
	}
	if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(resourceUpdates).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update resource fields: %w", err)
	}

	// Clear cached previews/thumbnails so they regenerate with new content
	if err := tx.Where("resource_id = ?", resourceID).Delete(&models.Preview{}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to clear cached previews: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger().Info(models.LogActionCreate, "resource_version", &version.ID, fmt.Sprintf("Restored v%d from v%d", nextVersion, sourceVersion.VersionNumber), comment, nil)

	return &version, nil
}

// DeleteVersion deletes a version, checking reference count before removing file
func (ctx *MahresourcesContext) DeleteVersion(resourceID, versionID uint) error {
	version, err := ctx.GetVersion(versionID)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
	}

	if version.ResourceID != resourceID {
		return errors.New("version does not belong to this resource")
	}

	var resource models.Resource
	if err := ctx.db.First(&resource, resourceID).Error; err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	if resource.CurrentVersionID != nil && *resource.CurrentVersionID == versionID {
		return errors.New("cannot delete current version")
	}

	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&versionCount)
	if versionCount <= 1 {
		return errors.New("cannot delete last version - delete the resource instead")
	}

	hash := version.Hash
	location := version.Location
	storageLocation := version.StorageLocation

	if err := ctx.db.Delete(version).Error; err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}

	refCount, err := ctx.CountHashReferences(hash)
	if err != nil {
		ctx.Logger().Warning(models.LogActionDelete, "resource_version", &versionID, "Failed to count hash references", err.Error(), nil)
	} else if refCount == 0 {
		fs, _ := ctx.GetFsForStorageLocation(storageLocation)
		if fs != nil {
			_ = fs.Remove(location)
		}
	}

	ctx.Logger().Info(models.LogActionDelete, "resource_version", &versionID, fmt.Sprintf("v%d of resource %d", version.VersionNumber, resourceID), "", nil)

	return nil
}

// CleanupVersions removes old versions based on criteria, returns deleted version IDs
func (ctx *MahresourcesContext) CleanupVersions(query *query_models.VersionCleanupQuery) ([]uint, error) {
	var deletedIDs []uint

	var resource models.Resource
	if err := ctx.db.First(&resource, query.ResourceID).Error; err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	q := ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", query.ResourceID)

	if resource.CurrentVersionID != nil {
		q = q.Where("id != ?", *resource.CurrentVersionID)
	}

	if query.KeepLast > 0 {
		var keepIDs []uint
		ctx.db.Model(&models.ResourceVersion{}).
			Where("resource_id = ?", query.ResourceID).
			Order("version_number DESC").
			Limit(query.KeepLast).
			Pluck("id", &keepIDs)
		if len(keepIDs) > 0 {
			q = q.Where("id NOT IN ?", keepIDs)
		}
	}

	if query.OlderThanDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -query.OlderThanDays)
		q = q.Where("created_at < ?", cutoff)
	}

	var versions []models.ResourceVersion
	if err := q.Find(&versions).Error; err != nil {
		return nil, err
	}

	if query.DryRun {
		for _, v := range versions {
			deletedIDs = append(deletedIDs, v.ID)
		}
		return deletedIDs, nil
	}

	for _, v := range versions {
		if err := ctx.DeleteVersion(query.ResourceID, v.ID); err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "version_cleanup", &v.ID, "Failed to delete version", err.Error(), nil)
			continue
		}
		deletedIDs = append(deletedIDs, v.ID)
	}

	return deletedIDs, nil
}

// BulkCleanupVersions cleans up versions across multiple resources
func (ctx *MahresourcesContext) BulkCleanupVersions(query *query_models.BulkVersionCleanupQuery) (map[uint][]uint, error) {
	result := make(map[uint][]uint)

	q := ctx.db.Model(&models.Resource{})
	if query.OwnerID > 0 {
		q = q.Where("owner_id = ?", query.OwnerID)
	}

	var resourceIDs []uint
	if err := q.Pluck("id", &resourceIDs).Error; err != nil {
		return nil, err
	}

	for _, resourceID := range resourceIDs {
		cleanupQuery := &query_models.VersionCleanupQuery{
			ResourceID:    resourceID,
			KeepLast:      query.KeepLast,
			OlderThanDays: query.OlderThanDays,
			DryRun:        query.DryRun,
		}

		deletedIDs, err := ctx.CleanupVersions(cleanupQuery)
		if err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "bulk_version_cleanup", &resourceID, "Failed to cleanup versions", err.Error(), nil)
			continue
		}

		if len(deletedIDs) > 0 {
			result[resourceID] = deletedIDs
		}
	}

	return result, nil
}

// VersionComparison holds comparison data between two versions
type VersionComparison struct {
	Version1       *models.ResourceVersion `json:"version1"`
	Version2       *models.ResourceVersion `json:"version2"`
	SizeDelta      int64                   `json:"sizeDelta"`
	SameHash       bool                    `json:"sameHash"`
	SameType       bool                    `json:"sameType"`
	DimensionsDiff bool                    `json:"dimensionsDiff"`
}

// CompareVersions compares two versions and returns comparison data
func (ctx *MahresourcesContext) CompareVersions(resourceID, v1ID, v2ID uint) (*VersionComparison, error) {
	version1, err := ctx.GetVersion(v1ID)
	if err != nil {
		return nil, fmt.Errorf("version 1 not found: %w", err)
	}

	version2, err := ctx.GetVersion(v2ID)
	if err != nil {
		return nil, fmt.Errorf("version 2 not found: %w", err)
	}

	if version1.ResourceID != resourceID || version2.ResourceID != resourceID {
		return nil, errors.New("versions do not belong to this resource")
	}

	comparison := &VersionComparison{
		Version1:       version1,
		Version2:       version2,
		SizeDelta:      version2.FileSize - version1.FileSize,
		SameHash:       version1.Hash == version2.Hash,
		SameType:       version1.ContentType == version2.ContentType,
		DimensionsDiff: version1.Width != version2.Width || version1.Height != version2.Height,
	}

	return comparison, nil
}

// SyncResourcesFromCurrentVersion updates resource fields to match their current version
// This fixes resources where version uploads occurred before the sync fix was deployed
func (ctx *MahresourcesContext) SyncResourcesFromCurrentVersion() error {
	// Use a single query to find resources that are out of sync with their current version
	// This is much faster than loading all resources and checking each one
	type outOfSyncResource struct {
		ResourceID       uint
		VersionHash      string
		VersionLocation  string
		StorageLocation  *string
		VersionType      string
		VersionWidth     uint
		VersionHeight    uint
		VersionFileSize  int64
	}

	// Use silent DB session to suppress GORM logging during migration
	silentDB := ctx.db.Session(&gorm.Session{Logger: logger.Discard})

	var outOfSync []outOfSyncResource
	err := silentDB.Raw(`
		SELECT r.id as resource_id,
		       v.hash as version_hash,
		       v.location as version_location,
		       v.storage_location as storage_location,
		       v.content_type as version_type,
		       v.width as version_width,
		       v.height as version_height,
		       v.file_size as version_file_size
		FROM resources r
		JOIN resource_versions v ON r.current_version_id = v.id
		WHERE r.hash != v.hash
		   OR r.location != v.location
		   OR r.content_type != v.content_type
		   OR r.file_size != v.file_size
	`).Scan(&outOfSync).Error
	if err != nil {
		return err
	}

	if len(outOfSync) == 0 {
		return nil
	}

	log.Printf("Syncing %d resources to their current versions...", len(outOfSync))

	for i, item := range outOfSync {
		if (i+1)%10000 == 0 {
			log.Printf("  Version sync progress: %d/%d (%.1f%%)", i+1, len(outOfSync), float64(i+1)/float64(len(outOfSync))*100)
		}

		updates := map[string]interface{}{
			"hash":             item.VersionHash,
			"location":         item.VersionLocation,
			"storage_location": item.StorageLocation,
			"content_type":     item.VersionType,
			"width":            item.VersionWidth,
			"height":           item.VersionHeight,
			"file_size":        item.VersionFileSize,
		}
		if err := silentDB.Model(&models.Resource{}).Where("id = ?", item.ResourceID).Updates(updates).Error; err != nil {
			ctx.Logger().Warning(models.LogActionCreate, "sync_version", &item.ResourceID, "Failed to sync resource from version", err.Error(), nil)
			continue
		}

		// Clear cached previews
		silentDB.Where("resource_id = ?", item.ResourceID).Delete(&models.Preview{})

		// Yield CPU time every 100 items (lower priority background task)
		if (i+1)%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	log.Printf("Version sync complete: %d resources synced", len(outOfSync))

	return nil
}

// MigrateResourceVersions creates initial version records for existing resources that don't have versions
func (ctx *MahresourcesContext) MigrateResourceVersions() error {
	// Use silent DB session to suppress GORM logging during migration
	silentDB := ctx.db.Session(&gorm.Session{Logger: logger.Discard})

	// Count resources that need migration
	var count int64
	if err := silentDB.Model(&models.Resource{}).Where("current_version_id IS NULL").Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return nil
	}

	log.Printf("Migrating %d resources to versioning system (background)...", count)

	// Process in batches to avoid loading all resources into memory
	batchSize := 500
	migrated := 0

	for {
		var resources []models.Resource
		if err := silentDB.Where("current_version_id IS NULL").
			Order("id").
			Limit(batchSize).
			Find(&resources).Error; err != nil {
			return err
		}

		if len(resources) == 0 {
			break
		}

		for _, resource := range resources {
			// Check if this resource already has versions (shouldn't happen, but be safe)
			var existingVersionCount int64
			silentDB.Model(&models.ResourceVersion{}).Where("resource_id = ?", resource.ID).Count(&existingVersionCount)
			if existingVersionCount > 0 {
				// Resource has versions but no current_version_id set - fix it
				var latestVersion models.ResourceVersion
				if err := silentDB.Where("resource_id = ?", resource.ID).Order("version_number DESC").First(&latestVersion).Error; err == nil {
					silentDB.Model(&resource).Update("current_version_id", latestVersion.ID)
				}
				migrated++
				continue
			}

			version := models.ResourceVersion{
				ResourceID:      resource.ID,
				VersionNumber:   1,
				Hash:            resource.Hash,
				HashType:        resource.HashType,
				FileSize:        resource.FileSize,
				ContentType:     resource.ContentType,
				Width:           resource.Width,
				Height:          resource.Height,
				Location:        resource.Location,
				StorageLocation: resource.StorageLocation,
				Comment:         "Initial version (migrated)",
			}

			if err := silentDB.Create(&version).Error; err != nil {
				log.Printf("Warning: failed to create version for resource %d: %v", resource.ID, err)
				continue
			}

			if err := silentDB.Model(&resource).Update("current_version_id", version.ID).Error; err != nil {
				log.Printf("Warning: failed to update current version for resource %d: %v", resource.ID, err)
			}
			migrated++

			// Log progress every 10,000 resources
			if migrated%10000 == 0 {
				log.Printf("  Version migration progress: %d/%d resources (%.1f%%)", migrated, count, float64(migrated)/float64(count)*100)
			}
		}

		// Don't increment offset - we're filtering by current_version_id IS NULL,
		// and we just updated those records, so next query gets fresh batch
		if len(resources) < batchSize {
			break
		}

		// Yield CPU time to other goroutines (lower priority background task)
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("Version migration complete: %d resources migrated", migrated)
	return nil
}
