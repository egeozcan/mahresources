package groupio

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"mahresources/archive"
)

// An archive refused on its content is the uploader's to fix; a staged file the
// server could not read is the server's failure, and its text names where the
// file is kept, so it is not marked as the archive's.
func TestArchiveContentErrorMarksOnlyTheArchivesOwnRefusals(t *testing.T) {
	var archiveErr *ArchiveError
	if err := archiveContentError(errors.New("this file is not a mahresources export archive")); !errors.As(err, &archiveErr) || archiveErr.Unsupported() {
		t.Fatalf("a malformed archive was not marked as the archive's: %v", err)
	}
	unsupported := fmt.Errorf("walk archive: %w", &archive.ErrUnsupportedSchemaVersion{Got: 99, Supported: []int{1}})
	if err := archiveContentError(unsupported); !errors.As(err, &archiveErr) || !archiveErr.Unsupported() {
		t.Fatalf("an unsupported schema version was not marked unsupported: %v", err)
	}
	readFailure := fmt.Errorf("archive: walk entry: %w", &fs.PathError{Op: "read", Path: "_imports/imp-1.tar", Err: errors.New("input/output error")})
	if err := archiveContentError(readFailure); errors.As(err, &archiveErr) {
		t.Fatalf("a failed read of the staged file was marked as the archive's: %v", err)
	}
}
