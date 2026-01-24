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
	"mime/multipart"
	"path"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
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
func (ctx *MahresourcesContext) GetVersions(resourceID uint) ([]models.ResourceVersion, error) {
	var versions []models.ResourceVersion
	err := ctx.db.Where("resource_id = ?", resourceID).Order("version_number DESC").Find(&versions).Error
	return versions, err
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
	// Get the resource
	resource, err := ctx.GetResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
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

	// Update resource's current version
	if err := tx.Model(resource).Update("current_version_id", version.ID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update current version: %w", err)
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

	if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Update("current_version_id", version.ID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update current version: %w", err)
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

// MigrateResourceVersions creates initial version records for existing resources
func (ctx *MahresourcesContext) MigrateResourceVersions() error {
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Count(&versionCount)
	if versionCount > 0 {
		return nil
	}

	var resources []models.Resource
	if err := ctx.db.Where("current_version_id IS NULL").Find(&resources).Error; err != nil {
		return err
	}

	if len(resources) == 0 {
		return nil
	}

	ctx.Logger().Info(models.LogActionCreate, "system", nil, fmt.Sprintf("Migrating %d resources to versioning system", len(resources)), "", nil)

	for _, resource := range resources {
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

		if err := ctx.db.Create(&version).Error; err != nil {
			ctx.Logger().Warning(models.LogActionCreate, "migration", &resource.ID, "Failed to create version for resource", err.Error(), nil)
			continue
		}

		if err := ctx.db.Model(&resource).Update("current_version_id", version.ID).Error; err != nil {
			ctx.Logger().Warning(models.LogActionCreate, "migration", &resource.ID, "Failed to update current version", err.Error(), nil)
		}
	}

	return nil
}
