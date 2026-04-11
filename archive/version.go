package archive

import "fmt"

// SchemaVersion is the manifest format major version. Bumped only on breaking
// changes. Readers reject manifests whose schema_version exceeds this constant.
const SchemaVersion = 1

// SupportedVersions enumerates the manifest versions this package can read.
// Today there is exactly one. Add older versions here when introducing v2+.
var SupportedVersions = []int{1}

// ErrUnsupportedSchemaVersion is returned by Reader.ReadManifest when the
// manifest's schema_version isn't in SupportedVersions.
type ErrUnsupportedSchemaVersion struct {
	Got       int
	Supported []int
}

func (e *ErrUnsupportedSchemaVersion) Error() string {
	return fmt.Sprintf("archive: unsupported schema_version %d (supported: %v)", e.Got, e.Supported)
}
