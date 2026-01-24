package application_context

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
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
