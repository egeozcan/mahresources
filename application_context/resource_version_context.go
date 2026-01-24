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
