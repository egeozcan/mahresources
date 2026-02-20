package interfaces

import (
	"mime/multipart"

	"github.com/spf13/afero"
	"mahresources/models"
	"mahresources/models/query_models"
)

// VersionReader handles reading version data
type VersionReader interface {
	GetVersions(resourceID uint) ([]models.ResourceVersion, error)
	GetVersion(versionID uint) (*models.ResourceVersion, error)
}

// VersionWriter handles creating and restoring versions
type VersionWriter interface {
	UploadNewVersion(resourceID uint, file multipart.File, header *multipart.FileHeader, comment string) (*models.ResourceVersion, error)
	RestoreVersion(resourceID, versionID uint, comment string) (*models.ResourceVersion, error)
}

// VersionDeleter handles version deletion
type VersionDeleter interface {
	DeleteVersion(resourceID, versionID uint) error
}

// VersionCleaner handles version cleanup operations
type VersionCleaner interface {
	CleanupVersions(query *query_models.VersionCleanupQuery) ([]uint, error)
	BulkCleanupVersions(query *query_models.BulkVersionCleanupQuery) (map[uint][]uint, error)
}

// VersionComparer handles version comparison
type VersionComparer interface {
	CompareVersions(resourceID, v1ID, v2ID uint) (*models.VersionComparison, error)
}

// VersionFileServer combines version reading with filesystem access for file serving
type VersionFileServer interface {
	VersionReader
	GetFsForStorageLocation(storageLocation *string) (afero.Fs, error)
}
