# Resource Versioning Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add version history tracking for resource files with upload, restore, compare, and cleanup capabilities.

**Architecture:** New `ResourceVersion` model with has-many relationship to `Resource`. Versions store file metadata (hash, size, dimensions) while sharing content-addressed storage. Reference counting ensures files aren't deleted while still referenced.

**Tech Stack:** Go, GORM, Gorilla Mux, Pongo2 templates, Alpine.js, sergi/go-diff for text comparison.

---

## Task 1: Create ResourceVersion Model

**Files:**
- Create: `models/resource_version_model.go`

**Step 1: Write the model file**

```go
package models

import (
	"time"
)

type ResourceVersion struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"createdAt"`
	ResourceID      uint      `gorm:"index;not null" json:"resourceId"`
	VersionNumber   int       `gorm:"not null" json:"versionNumber"`
	Hash            string    `gorm:"index;not null" json:"hash"`
	HashType        string    `gorm:"not null;default:'SHA1'" json:"hashType"`
	FileSize        int64     `gorm:"not null" json:"fileSize"`
	ContentType     string    `json:"contentType"`
	Width           uint      `json:"width"`
	Height          uint      `json:"height"`
	Location        string    `gorm:"not null" json:"location"`
	StorageLocation *string   `json:"storageLocation"`
	Comment         string    `json:"comment"`
}

func (v ResourceVersion) GetId() uint {
	return v.ID
}
```

**Step 2: Run tests to verify no syntax errors**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add models/resource_version_model.go
git commit -m "feat: add ResourceVersion model"
```

---

## Task 2: Update Resource Model with Version Relationship

**Files:**
- Modify: `models/resource_model.go`

**Step 1: Add version fields to Resource struct**

After line 34 (after `Previews` field), add:

```go
	CurrentVersionID *uint              `json:"currentVersionId"`
	CurrentVersion   *ResourceVersion   `gorm:"foreignKey:CurrentVersionID" json:"currentVersion,omitempty"`
	Versions         []ResourceVersion  `gorm:"foreignKey:ResourceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"versions,omitempty"`
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add models/resource_model.go
git commit -m "feat: add version relationship to Resource model"
```

---

## Task 3: Register ResourceVersion in GORM AutoMigrate

**Files:**
- Modify: `application_context/context.go`

**Step 1: Find the AutoMigrate call and add ResourceVersion**

Search for `AutoMigrate` in context.go. Add `&models.ResourceVersion{}` to the list of models.

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/context.go
git commit -m "feat: register ResourceVersion for auto-migration"
```

---

## Task 4: Create Query Models for Versioning

**Files:**
- Create: `models/query_models/version_query.go`

**Step 1: Write the query models**

```go
package query_models

type VersionUploadQuery struct {
	ResourceID uint   `json:"resourceId"`
	Comment    string `json:"comment"`
}

type VersionRestoreQuery struct {
	ResourceID uint   `json:"resourceId"`
	VersionID  uint   `json:"versionId"`
	Comment    string `json:"comment"`
}

type VersionCleanupQuery struct {
	ResourceID    uint `json:"resourceId"`
	KeepLast      int  `json:"keepLast"`
	OlderThanDays int  `json:"olderThanDays"`
	DryRun        bool `json:"dryRun"`
}

type BulkVersionCleanupQuery struct {
	KeepLast      int  `json:"keepLast"`
	OlderThanDays int  `json:"olderThanDays"`
	OwnerID       uint `json:"ownerId"`
	DryRun        bool `json:"dryRun"`
}

type VersionCompareQuery struct {
	ResourceID uint `json:"resourceId"`
	V1         uint `json:"v1"`
	V2         uint `json:"v2"`
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add models/query_models/version_query.go
git commit -m "feat: add query models for versioning"
```

---

## Task 5: Add Hash Reference Counting Function

**Files:**
- Create: `application_context/resource_version_context.go`

**Step 1: Write the base version context with reference counting**

```go
package application_context

import (
	"mahresources/models"
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
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add hash reference counting for versioning"
```

---

## Task 6: Add Version CRUD Operations

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add GetVersions and GetVersion functions**

Append to resource_version_context.go:

```go
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
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version read operations"
```

---

## Task 7: Add Upload New Version Function

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add imports at top of file**

```go
import (
	"errors"
	"fmt"
	"io"
	"mahresources/models"
	"mahresources/models/query_models"
	"mime/multipart"
)
```

**Step 2: Add UploadNewVersion function**

```go
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

	// Process the file using existing upload infrastructure
	// This reuses the hash computation and deduplication logic
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
```

**Step 3: Run build to verify**

Run: `go build ./...`
Expected: Build fails (processFileForVersion not defined yet)

**Step 4: Commit work in progress**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add upload new version function (WIP)"
```

---

## Task 8: Add File Processing Helper for Versions

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add processFileForVersion helper**

This extracts file processing logic that can be reused:

```go
// processFileForVersion handles file storage and returns metadata
// Returns: hash, location, fileSize, contentType, width, height, storageLocation, error
func (ctx *MahresourcesContext) processFileForVersion(file multipart.File, header *multipart.FileHeader) (string, string, int64, string, uint, uint, *string, error) {
	// Read file content for hashing
	content, err := io.ReadAll(file)
	if err != nil {
		return "", "", 0, "", 0, 0, nil, err
	}

	// Reset file reader
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// Compute hash
	hash := computeSHA1(content)
	fileSize := int64(len(content))

	// Detect content type
	contentType := detectContentType(content, header.Filename)

	// Get dimensions if image
	width, height := getDimensions(content, contentType)

	// Build storage path
	ext := getExtension(header.Filename, contentType)
	location := buildResourcePath(hash, ext)

	// Check if file already exists (deduplication)
	if exists, _ := afero.Exists(ctx.fs, location); !exists {
		// Store the file
		if err := ctx.storeFile(location, content); err != nil {
			return "", "", 0, "", 0, 0, nil, err
		}
	}

	return hash, location, fileSize, contentType, width, height, nil, nil
}

func (ctx *MahresourcesContext) storeFile(location string, content []byte) error {
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
```

**Step 2: Add required imports**

Add to imports:
```go
	"crypto/sha1"
	"encoding/hex"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
```

**Step 3: Add helper functions**

```go
func computeSHA1(content []byte) string {
	h := sha1.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func detectContentType(content []byte, filename string) string {
	mime := mimetype.Detect(content)
	return mime.String()
}

func getDimensions(content []byte, contentType string) (uint, uint) {
	if !strings.HasPrefix(contentType, "image/") {
		return 0, 0
	}
	// Use image.DecodeConfig for dimensions
	reader := bytes.NewReader(content)
	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0
	}
	return uint(config.Width), uint(config.Height)
}

func getExtension(filename, contentType string) string {
	ext := path.Ext(filename)
	if ext != "" {
		return ext
	}
	// Fallback to content type
	mime := mimetype.Lookup(contentType)
	if mime != nil {
		return mime.Extension()
	}
	return ""
}

func buildResourcePath(hash, ext string) string {
	return fmt.Sprintf("/resources/%s/%s/%s/%s%s", hash[0:2], hash[2:4], hash[4:6], hash, ext)
}
```

**Step 4: Add bytes and image imports**

```go
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
```

**Step 5: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add file processing helper for versions"
```

---

## Task 9: Add Version Restore Function

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add RestoreVersion function**

```go
// RestoreVersion creates a new version by copying metadata from an old version
func (ctx *MahresourcesContext) RestoreVersion(resourceID, versionID uint, comment string) (*models.ResourceVersion, error) {
	// Get the source version
	sourceVersion, err := ctx.GetVersion(versionID)
	if err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}

	if sourceVersion.ResourceID != resourceID {
		return nil, errors.New("version does not belong to this resource")
	}

	// Get the next version number
	var maxVersion int
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Select("COALESCE(MAX(version_number), 0)").Scan(&maxVersion)
	nextVersion := maxVersion + 1

	// Default comment
	if comment == "" {
		comment = fmt.Sprintf("Restored from version %d", sourceVersion.VersionNumber)
	}

	// Create new version with same file reference (deduplication)
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

	// Update resource's current version
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
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version restore function"
```

---

## Task 10: Add Version Delete Function with Reference Counting

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add DeleteVersion function**

```go
// DeleteVersion deletes a version, checking reference count before removing file
func (ctx *MahresourcesContext) DeleteVersion(resourceID, versionID uint) error {
	version, err := ctx.GetVersion(versionID)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
	}

	if version.ResourceID != resourceID {
		return errors.New("version does not belong to this resource")
	}

	// Check if this is the current version
	var resource models.Resource
	if err := ctx.db.First(&resource, resourceID).Error; err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	if resource.CurrentVersionID != nil && *resource.CurrentVersionID == versionID {
		return errors.New("cannot delete current version")
	}

	// Check if this is the last version
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&versionCount)
	if versionCount <= 1 {
		return errors.New("cannot delete last version - delete the resource instead")
	}

	hash := version.Hash
	location := version.Location
	storageLocation := version.StorageLocation

	// Delete the version record
	if err := ctx.db.Delete(version).Error; err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}

	// Check reference count and delete file if no longer referenced
	refCount, err := ctx.CountHashReferences(hash)
	if err != nil {
		ctx.Logger().Warn("version", &versionID, "Failed to count hash references", err.Error())
	} else if refCount == 0 {
		// Move file to deleted folder
		fs, _ := ctx.GetFsForStorageLocation(storageLocation)
		if fs != nil {
			_ = fs.Remove(location)
		}
	}

	ctx.Logger().Info(models.LogActionDelete, "resource_version", &versionID, fmt.Sprintf("v%d of resource %d", version.VersionNumber, resourceID), "", nil)

	return nil
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version delete with reference counting"
```

---

## Task 11: Add Version Cleanup Functions

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add CleanupVersions function**

```go
// CleanupVersions removes old versions based on criteria, returns deleted version IDs
func (ctx *MahresourcesContext) CleanupVersions(query *query_models.VersionCleanupQuery) ([]uint, error) {
	var deletedIDs []uint

	// Get current version to exclude
	var resource models.Resource
	if err := ctx.db.First(&resource, query.ResourceID).Error; err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	// Build query for versions to delete
	q := ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", query.ResourceID)

	// Exclude current version
	if resource.CurrentVersionID != nil {
		q = q.Where("id != ?", *resource.CurrentVersionID)
	}

	// Apply KeepLast filter - keep N most recent versions
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

	// Apply OlderThanDays filter
	if query.OlderThanDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -query.OlderThanDays)
		q = q.Where("created_at < ?", cutoff)
	}

	// Get versions to delete
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

	// Delete each version
	for _, v := range versions {
		if err := ctx.DeleteVersion(query.ResourceID, v.ID); err != nil {
			ctx.Logger().Warn("version_cleanup", &v.ID, "Failed to delete version", err.Error())
			continue
		}
		deletedIDs = append(deletedIDs, v.ID)
	}

	return deletedIDs, nil
}
```

**Step 2: Add required import**

Add `"time"` to imports.

**Step 3: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version cleanup function"
```

---

## Task 12: Add Bulk Version Cleanup Function

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add BulkCleanupVersions function**

```go
// BulkCleanupVersions cleans up versions across multiple resources
func (ctx *MahresourcesContext) BulkCleanupVersions(query *query_models.BulkVersionCleanupQuery) (map[uint][]uint, error) {
	result := make(map[uint][]uint)

	// Build resource query
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
			ctx.Logger().Warn("bulk_version_cleanup", &resourceID, "Failed to cleanup versions", err.Error())
			continue
		}

		if len(deletedIDs) > 0 {
			result[resourceID] = deletedIDs
		}
	}

	return result, nil
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add bulk version cleanup function"
```

---

## Task 13: Add Migration Function for Existing Resources

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add MigrateResourceVersions function**

```go
// MigrateResourceVersions creates initial version records for existing resources
// This should be called once on startup when the version table is empty
func (ctx *MahresourcesContext) MigrateResourceVersions() error {
	// Check if migration is needed
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Count(&versionCount)
	if versionCount > 0 {
		return nil // Already migrated
	}

	// Get all resources without a current version
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
			ctx.Logger().Warn("migration", &resource.ID, "Failed to create version for resource", err.Error())
			continue
		}

		if err := ctx.db.Model(&resource).Update("current_version_id", version.ID).Error; err != nil {
			ctx.Logger().Warn("migration", &resource.ID, "Failed to update current version", err.Error())
		}
	}

	return nil
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version migration for existing resources"
```

---

## Task 14: Call Migration on Context Initialization

**Files:**
- Modify: `application_context/context.go`

**Step 1: Find the initialization code after AutoMigrate and add migration call**

After the AutoMigrate call completes, add:

```go
	// Migrate existing resources to versioning system
	if err := mahContext.MigrateResourceVersions(); err != nil {
		log.Printf("Warning: failed to migrate resource versions: %v", err)
	}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/context.go
git commit -m "feat: call version migration on startup"
```

---

## Task 15: Update DeleteResource to Check Version References

**Files:**
- Modify: `application_context/resource_bulk_context.go`

**Step 1: Update DeleteResource function to check hash references before deleting file**

In the DeleteResource function, after `_ = file.Close()` (around line 70) and before deleting the file, add reference check:

Replace the line `_ = fs.Remove(resource.GetCleanLocation())` (around line 77) with:

```go
	// Check if any other resources or versions reference this hash
	refCount, countErr := ctx.CountHashReferences(resource.Hash)
	if countErr != nil {
		ctx.Logger().Warn("resource", &resourceId, "Failed to count hash references", countErr.Error())
		refCount = 1 // Assume referenced to be safe
	}

	// Only delete file if no other references exist
	if refCount == 0 {
		_ = fs.Remove(resource.GetCleanLocation())
	}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_bulk_context.go
git commit -m "feat: check hash references before deleting resource file"
```

---

## Task 16: Create Version API Handlers

**Files:**
- Create: `server/api_handlers/version_api_handlers.go`

**Step 1: Write the API handlers**

```go
package api_handlers

import (
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"strconv"
)

// VersionReader interface for reading versions
type VersionReader interface {
	GetVersions(resourceID uint) ([]models.ResourceVersion, error)
	GetVersion(versionID uint) (*models.ResourceVersion, error)
}

// VersionWriter interface for writing versions
type VersionWriter interface {
	UploadNewVersion(resourceID uint, file interfaces.MultipartFile, header interfaces.MultipartFileHeader, comment string) (*models.ResourceVersion, error)
	RestoreVersion(resourceID, versionID uint, comment string) (*models.ResourceVersion, error)
	DeleteVersion(resourceID, versionID uint) error
	CleanupVersions(query *query_models.VersionCleanupQuery) ([]uint, error)
	BulkCleanupVersions(query *query_models.BulkVersionCleanupQuery) (map[uint][]uint, error)
}

// GetListVersionsHandler returns handler for listing versions
func GetListVersionsHandler(ctx VersionReader) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		versions, err := ctx.GetVersions(uint(resourceID))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(versions, w, r)
	}
}

// GetVersionHandler returns handler for getting a single version
func GetVersionHandler(ctx VersionReader) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid version id"), w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.GetVersion(uint(versionID))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		http_utils.WriteJSONResponse(version, w, r)
	}
}

// GetUploadVersionHandler returns handler for uploading a new version
func GetUploadVersionHandler(ctx VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http_utils.HandleError(fmt.Errorf("file required"), w, r, http.StatusBadRequest)
			return
		}
		defer file.Close()

		comment := r.FormValue("comment")

		version, err := ctx.UploadNewVersion(uint(resourceID), file, header, comment)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(version, w, r)
	}
}

// GetRestoreVersionHandler returns handler for restoring a version
func GetRestoreVersionHandler(ctx VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.VersionRestoreQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.RestoreVersion(query.ResourceID, query.VersionID, query.Comment)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(version, w, r)
	}
}

// GetDeleteVersionHandler returns handler for deleting a version
func GetDeleteVersionHandler(ctx VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		versionID, err := strconv.ParseUint(r.URL.Query().Get("versionId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid versionId"), w, r, http.StatusBadRequest)
			return
		}

		if err := ctx.DeleteVersion(uint(resourceID), uint(versionID)); err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(map[string]string{"status": "deleted"}, w, r)
	}
}

// GetCleanupVersionsHandler returns handler for cleaning up versions
func GetCleanupVersionsHandler(ctx VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.VersionCleanupQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		deletedIDs, err := ctx.CleanupVersions(&query)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(map[string]interface{}{
			"deletedVersionIds": deletedIDs,
			"count":             len(deletedIDs),
		}, w, r)
	}
}

// GetBulkCleanupVersionsHandler returns handler for bulk cleanup
func GetBulkCleanupVersionsHandler(ctx VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.BulkVersionCleanupQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		result, err := ctx.BulkCleanupVersions(&query)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		totalDeleted := 0
		for _, ids := range result {
			totalDeleted += len(ids)
		}

		http_utils.WriteJSONResponse(map[string]interface{}{
			"deletedByResource": result,
			"totalDeleted":      totalDeleted,
		}, w, r)
	}
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build may fail due to interface issues - will fix in next task

**Step 3: Commit**

```bash
git add server/api_handlers/version_api_handlers.go
git commit -m "feat: add version API handlers"
```

---

## Task 17: Add Version File Download Handler

**Files:**
- Modify: `server/api_handlers/version_api_handlers.go`

**Step 1: Add interfaces import and file serving**

Add this to the version_api_handlers.go:

```go
// VersionFileServer interface for serving version files
type VersionFileServer interface {
	GetVersion(versionID uint) (*models.ResourceVersion, error)
	GetFsForStorageLocation(storageLocation *string) (interfaces.Fs, error)
}

// GetVersionFileHandler returns handler for downloading version file
func GetVersionFileHandler(ctx VersionFileServer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, err := strconv.ParseUint(r.URL.Query().Get("versionId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid versionId"), w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.GetVersion(uint(versionID))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		fs, err := ctx.GetFsForStorageLocation(version.StorageLocation)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		file, err := fs.Open(version.Location)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", version.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"v%d_%s\"", version.VersionNumber, version.Hash[:8]))

		http.ServeContent(w, r, "", version.CreatedAt, file)
	}
}
```

**Step 2: Add io import**

Add `"io"` to imports if not present.

**Step 3: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add server/api_handlers/version_api_handlers.go
git commit -m "feat: add version file download handler"
```

---

## Task 18: Register Version API Routes

**Files:**
- Modify: `server/routes.go`

**Step 1: Find the route registration section and add version routes**

After the resource routes section (around line 150), add:

```go
	// Version routes
	router.Methods(http.MethodGet).Path("/v1/resource/versions").
		HandlerFunc(api_handlers.GetListVersionsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/version").
		HandlerFunc(api_handlers.GetVersionHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/versions").
		HandlerFunc(api_handlers.GetUploadVersionHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/version/restore").
		HandlerFunc(api_handlers.GetRestoreVersionHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/resource/version").
		HandlerFunc(api_handlers.GetDeleteVersionHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/version/file").
		HandlerFunc(api_handlers.GetVersionFileHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/versions/cleanup").
		HandlerFunc(api_handlers.GetCleanupVersionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/versions/cleanup").
		HandlerFunc(api_handlers.GetBulkCleanupVersionsHandler(appContext))
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/routes.go
git commit -m "feat: register version API routes"
```

---

## Task 19: Add Version Comparison Function

**Files:**
- Modify: `application_context/resource_version_context.go`

**Step 1: Add CompareVersions function**

```go
// VersionComparison holds comparison data between two versions
type VersionComparison struct {
	Version1       *models.ResourceVersion `json:"version1"`
	Version2       *models.ResourceVersion `json:"version2"`
	SizeDelta      int64                   `json:"sizeDelta"`
	SameHash       bool                    `json:"sameHash"`
	SameType       bool                    `json:"sameType"`
	DimensionsDiff bool                    `json:"dimensionsDiff"`
	TextDiff       *string                 `json:"textDiff,omitempty"`
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

	// TODO: Add text diff for text-based content types in a future task
	// This requires reading file contents and using a diff library

	return comparison, nil
}
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add application_context/resource_version_context.go
git commit -m "feat: add version comparison function"
```

---

## Task 20: Add Comparison API Handler

**Files:**
- Modify: `server/api_handlers/version_api_handlers.go`

**Step 1: Add VersionComparer interface and handler**

```go
// VersionComparer interface for comparing versions
type VersionComparer interface {
	CompareVersions(resourceID, v1ID, v2ID uint) (*application_context.VersionComparison, error)
}

// GetCompareVersionsHandler returns handler for comparing versions
func GetCompareVersionsHandler(ctx VersionComparer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		v1, err := strconv.ParseUint(r.URL.Query().Get("v1"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid v1"), w, r, http.StatusBadRequest)
			return
		}

		v2, err := strconv.ParseUint(r.URL.Query().Get("v2"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid v2"), w, r, http.StatusBadRequest)
			return
		}

		comparison, err := ctx.CompareVersions(uint(resourceID), uint(v1), uint(v2))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		http_utils.WriteJSONResponse(comparison, w, r)
	}
}
```

**Step 2: Add import for application_context**

Add `"mahresources/application_context"` to imports.

**Step 3: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add server/api_handlers/version_api_handlers.go
git commit -m "feat: add version comparison API handler"
```

---

## Task 21: Register Comparison Route

**Files:**
- Modify: `server/routes.go`

**Step 1: Add comparison route**

After the other version routes, add:

```go
	router.Methods(http.MethodGet).Path("/v1/resource/versions/compare").
		HandlerFunc(api_handlers.GetCompareVersionsHandler(appContext))
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/routes.go
git commit -m "feat: register version comparison route"
```

---

## Task 22: Add Version Panel Template Partial

**Files:**
- Create: `templates/partials/versionPanel.tpl`

**Step 1: Write the version panel template**

```django
{% if versions %}
<div class="mt-6" x-data="{ expanded: {{ versions|length }} > 1, compareMode: false, selected: [] }">
    <button
        @click="expanded = !expanded"
        class="flex items-center justify-between w-full px-4 py-2 text-left bg-gray-100 hover:bg-gray-200 rounded-lg"
    >
        <span class="font-medium">Versions ({{ versions|length }})</span>
        <svg class="w-5 h-5 transition-transform" :class="{ 'rotate-180': expanded }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
        </svg>
    </button>

    <div x-show="expanded" x-collapse class="mt-2 border rounded-lg divide-y">
        {% for version in versions %}
        <div class="p-4 {% if version.ID == currentVersionId %}bg-blue-50{% endif %}">
            <div class="flex items-center justify-between">
                <div class="flex items-center space-x-3">
                    <template x-if="compareMode">
                        <input type="checkbox"
                            :value="{{ version.ID }}"
                            @change="selected.includes({{ version.ID }}) ? selected = selected.filter(x => x !== {{ version.ID }}) : selected.push({{ version.ID }})"
                            :disabled="selected.length >= 2 && !selected.includes({{ version.ID }})"
                            class="rounded">
                    </template>
                    <span class="font-medium">
                        v{{ version.VersionNumber }}
                        {% if version.ID == currentVersionId %}
                        <span class="ml-1 px-2 py-0.5 text-xs bg-blue-100 text-blue-800 rounded">current</span>
                        {% endif %}
                    </span>
                    <span class="text-gray-500 text-sm">{{ version.CreatedAt|date:"Jan 02, 2006" }}</span>
                    <span class="text-gray-500 text-sm">{{ version.FileSize|humanReadableSize }}</span>
                </div>
                <div class="flex items-center space-x-2">
                    <a href="/v1/resource/version/file?versionId={{ version.ID }}"
                       class="px-3 py-1 text-sm text-indigo-600 hover:text-indigo-800">
                        Download
                    </a>
                    {% if version.ID != currentVersionId %}
                    <form action="/v1/resource/version/restore" method="post" class="inline">
                        <input type="hidden" name="resourceId" value="{{ resourceId }}">
                        <input type="hidden" name="versionId" value="{{ version.ID }}">
                        <button type="submit" class="px-3 py-1 text-sm text-green-600 hover:text-green-800">
                            Restore
                        </button>
                    </form>
                    <form action="/v1/resource/version?resourceId={{ resourceId }}&versionId={{ version.ID }}" method="post" class="inline"
                          x-data="confirmAction({ message: 'Delete this version?' })" x-bind="events">
                        <input type="hidden" name="_method" value="DELETE">
                        <button type="submit" class="px-3 py-1 text-sm text-red-600 hover:text-red-800">
                            Delete
                        </button>
                    </form>
                    {% endif %}
                </div>
            </div>
            {% if version.Comment %}
            <p class="mt-1 text-sm text-gray-600 italic">"{{ version.Comment }}"</p>
            {% endif %}
        </div>
        {% endfor %}

        <div class="p-4 bg-gray-50">
            <div class="flex items-center justify-between">
                <button @click="compareMode = !compareMode; selected = []"
                        class="px-3 py-1 text-sm border rounded hover:bg-gray-100"
                        :class="{ 'bg-indigo-100 border-indigo-300': compareMode }">
                    <span x-text="compareMode ? 'Cancel Compare' : 'Compare'"></span>
                </button>

                <template x-if="compareMode && selected.length === 2">
                    <a :href="'/v1/resource/versions/compare?resourceId={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
                       class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
                        Compare Selected
                    </a>
                </template>

                <form action="/v1/resource/versions" method="post" enctype="multipart/form-data"
                      class="flex items-center space-x-2">
                    <input type="hidden" name="resourceId" value="{{ resourceId }}">
                    <input type="file" name="file" required class="text-sm">
                    <input type="text" name="comment" placeholder="Comment (optional)"
                           class="px-2 py-1 text-sm border rounded">
                    <button type="submit" class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
                        Upload New Version
                    </button>
                </form>
            </div>
        </div>
    </div>
</div>
{% endif %}
```

**Step 2: Commit**

```bash
git add templates/partials/versionPanel.tpl
git commit -m "feat: add version panel template partial"
```

---

## Task 23: Include Version Panel in Resource Display

**Files:**
- Modify: `templates/displayResource.tpl`

**Step 1: Add version panel include**

Before `{% endblock %}` for the body block (around line 28), add:

```django
    {% include "/partials/versionPanel.tpl" with versions=versions currentVersionId=resource.CurrentVersionID resourceId=resource.ID %}
```

**Step 2: Commit**

```bash
git add templates/displayResource.tpl
git commit -m "feat: include version panel in resource display"
```

---

## Task 24: Update Resource Template Context Provider

**Files:**
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`

**Step 1: Find ResourceContextProvider and add versions to context**

Add versions loading after getting the resource:

```go
	// Get versions for the resource
	versions, _ := context.GetVersions(resource.ID)
```

And add to the return context:

```go
	"versions": versions,
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/resource_template_context.go
git commit -m "feat: add versions to resource template context"
```

---

## Task 25: Add Unit Tests for Version Context

**Files:**
- Create: `application_context/resource_version_context_test.go`

**Step 1: Write basic tests**

```go
package application_context

import (
	"testing"

	"mahresources/models"
)

func TestCountHashReferences(t *testing.T) {
	ctx := setupTestContext(t)
	defer teardownTestContext(ctx)

	// Create a resource with known hash
	resource := &models.Resource{
		Name:     "test",
		Hash:     "abc123",
		HashType: "SHA1",
		Location: "/test/path",
		FileSize: 100,
	}
	ctx.db.Create(resource)

	// Create a version with same hash
	version := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "abc123",
		HashType:      "SHA1",
		Location:      "/test/path",
		FileSize:      100,
	}
	ctx.db.Create(version)

	count, err := ctx.CountHashReferences("abc123")
	if err != nil {
		t.Fatalf("CountHashReferences failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 references, got %d", count)
	}
}

func TestGetVersions(t *testing.T) {
	ctx := setupTestContext(t)
	defer teardownTestContext(ctx)

	// Create a resource
	resource := &models.Resource{
		Name:     "test",
		Hash:     "abc123",
		HashType: "SHA1",
		Location: "/test/path",
		FileSize: 100,
	}
	ctx.db.Create(resource)

	// Create versions
	for i := 1; i <= 3; i++ {
		version := &models.ResourceVersion{
			ResourceID:    resource.ID,
			VersionNumber: i,
			Hash:          "abc123",
			HashType:      "SHA1",
			Location:      "/test/path",
			FileSize:      100,
		}
		ctx.db.Create(version)
	}

	versions, err := ctx.GetVersions(resource.ID)
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got %d", len(versions))
	}

	// Verify order (DESC)
	if versions[0].VersionNumber != 3 {
		t.Errorf("Expected first version to be 3, got %d", versions[0].VersionNumber)
	}
}
```

**Step 2: Run tests**

Run: `go test ./application_context/... -run TestCountHashReferences -v`
Expected: Test passes (may need to adjust based on test setup)

**Step 3: Commit**

```bash
git add application_context/resource_version_context_test.go
git commit -m "test: add unit tests for version context"
```

---

## Task 26: Run Full Test Suite

**Step 1: Run all Go tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Build the application**

Run: `npm run build`
Expected: Build succeeds

**Step 3: Commit any fixes if needed**

---

## Task 27: Manual Integration Testing

**Step 1: Start the server in ephemeral mode**

Run: `./mahresources -ephemeral -bind-address=:8181`

**Step 2: Test version API endpoints**

1. Create a resource via UI
2. Upload a new version via `/v1/resource/versions` POST
3. List versions via `/v1/resource/versions?resourceId=X` GET
4. Restore a version via `/v1/resource/version/restore` POST
5. Delete a version via `/v1/resource/version?resourceId=X&versionId=Y` DELETE
6. Compare versions via `/v1/resource/versions/compare?resourceId=X&v1=Y&v2=Z` GET

**Step 3: Verify UI**

1. Navigate to a resource detail page
2. Verify version panel shows
3. Test upload new version
4. Test restore
5. Test compare mode

---

## Task 28: Final Commit and Summary

**Step 1: Review all changes**

Run: `git log --oneline HEAD~25..HEAD`

**Step 2: Create summary commit if needed**

If all tests pass and everything works, the implementation is complete.

---

## Summary of Files Changed/Created

**New Files:**
- `models/resource_version_model.go` - ResourceVersion model
- `models/query_models/version_query.go` - Query models
- `application_context/resource_version_context.go` - Business logic
- `application_context/resource_version_context_test.go` - Tests
- `server/api_handlers/version_api_handlers.go` - API handlers
- `templates/partials/versionPanel.tpl` - UI template

**Modified Files:**
- `models/resource_model.go` - Add version relationship
- `application_context/context.go` - AutoMigrate + migration call
- `application_context/resource_bulk_context.go` - Reference counting
- `server/routes.go` - Register version routes
- `server/template_handlers/template_context_providers/resource_template_context.go` - Add versions to context
- `templates/displayResource.tpl` - Include version panel
