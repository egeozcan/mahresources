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

// CrossVersionCompareQuery for comparing versions across different resources
type CrossVersionCompareQuery struct {
	Resource1ID uint `schema:"r1"`
	Version1    int  `schema:"v1"`
	Resource2ID uint `schema:"r2"`
	Version2    int  `schema:"v2"`
}
