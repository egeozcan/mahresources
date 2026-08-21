package application_context

import (
	"encoding/json"
	"testing"

	"github.com/spf13/afero"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestAddLocalResource_TagsAndGroupsAssociated verifies that Tags and Groups
// specified in the ResourceFromLocalCreator are actually saved as associations
// on the created resource. This is a regression test for a bug where
// AddLocalResource silently ignores Groups, Tags, and Notes fields —
// unlike AddResource (the upload path) which correctly saves them.
func TestAddLocalResource_TagsAndGroupsAssociated(t *testing.T) {
	ctx := createTestContext(t)

	// Set up the alt filesystem so AddLocalResource can find the file.
	// AddLocalResource calls GetFsForStorageLocation(&PathName), and when
	// PathName is non-empty it looks in altFileSystems.
	altFs := afero.NewMemMapFs()
	ctx.altFileSystems["testfs"] = altFs

	// Create a test file on the alt filesystem
	testContent := []byte("hello world test file content")
	if err := afero.WriteFile(altFs, "/testfile.txt", testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a group to associate
	group, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: "Test Group",
		Meta: "{}",
	})
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Create a tag to associate
	tag, err := ctx.CreateTag(&query_models.TagCreator{
		Name: "Test Tag",
	})
	if err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}

	// Create a note to associate
	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name: "Test Note",
			Meta: "{}",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create note: %v", err)
	}

	// Call AddLocalResource with Groups, Tags, and Notes specified
	resource, err := ctx.AddLocalResource("testfile.txt", &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:        "Test Local Resource",
			Description: "A test resource",
			Groups:      []uint{group.ID},
			Tags:        []uint{tag.ID},
			Notes:       []uint{note.ID},
			Meta:        `{"key": "value"}`,
		},
		LocalPath: "/testfile.txt",
		PathName:  "testfs",
	})
	if err != nil {
		t.Fatalf("AddLocalResource() error = %v", err)
	}

	if resource == nil {
		t.Fatal("AddLocalResource() returned nil resource")
	}

	// Now reload the resource with associations to verify they were saved
	loaded, err := ctx.GetResource(resource.ID)
	if err != nil {
		t.Fatalf("GetResource() error = %v", err)
	}

	// Verify Tags were associated
	if len(loaded.Tags) != 1 {
		t.Errorf("Expected 1 tag associated with resource, got %d", len(loaded.Tags))
	} else if loaded.Tags[0].ID != tag.ID {
		t.Errorf("Expected tag ID %d, got %d", tag.ID, loaded.Tags[0].ID)
	}

	// Verify Groups were associated
	if len(loaded.Groups) != 1 {
		t.Errorf("Expected 1 group associated with resource, got %d", len(loaded.Groups))
	} else if loaded.Groups[0].ID != group.ID {
		t.Errorf("Expected group ID %d, got %d", group.ID, loaded.Groups[0].ID)
	}

	// Verify Notes were associated
	if len(loaded.Notes) != 1 {
		t.Errorf("Expected 1 note associated with resource, got %d", len(loaded.Notes))
	} else if loaded.Notes[0].ID != note.ID {
		t.Errorf("Expected note ID %d, got %d", note.ID, loaded.Notes[0].ID)
	}

	// Also verify direct DB query for associations as a belt-and-suspenders check
	var tagCount int64
	ctx.db.Table("resource_tags").Where("resource_id = ?", resource.ID).Count(&tagCount)
	if tagCount != 1 {
		t.Errorf("Expected 1 resource_tag row, got %d", tagCount)
	}

	var groupCount int64
	ctx.db.Table("groups_related_resources").Where("resource_id = ?", resource.ID).Count(&groupCount)
	if groupCount != 1 {
		t.Errorf("Expected 1 groups_related_resources row, got %d", groupCount)
	}

	var noteCount int64
	ctx.db.Table("resource_notes").Where("resource_id = ?", resource.ID).Count(&noteCount)
	if noteCount != 1 {
		t.Errorf("Expected 1 resource_notes row, got %d", noteCount)
	}

	// Clean up - delete the resource to not affect other tests using shared memory DB
	ctx.db.Delete(&models.Resource{}, resource.ID)
	ctx.db.Delete(&models.Group{}, group.ID)
	ctx.db.Delete(&models.Tag{}, tag.ID)
	ctx.db.Delete(&models.Note{}, note.ID)
}

// purgeResourcesAt clears any resource rows left at a path by an earlier run.
//
// createTestContext opens sqlite "file::memory:?cache=shared", so every test in
// this package shares one database and it survives -count=N within a process.
// Without this, the second iteration of a dedup test is answered by the first
// iteration's row before AddLocalResource selects a filesystem or persists
// anything -- the assertions still pass, having exercised none of the code they
// name. Arming the cleanup before the create, rather than deferring it after,
// is what makes the test independent of what ran before it.
func purgeResourcesAt(t *testing.T, ctx *MahresourcesContext, location string) {
	t.Helper()
	if err := ctx.db.Unscoped().Where("location = ?", location).Delete(&models.Resource{}).Error; err != nil {
		t.Fatalf("purge resources at %q: %v", location, err)
	}
}

// TestAddLocalResource_EmptyPathNameUsesDefaultFilesystem pins the ordinary
// case: no alt filesystem named, so the file is read from the default one.
//
// AddLocalResource passed &resourceQuery.PathName, which is never nil, so
// GetFsForStorageLocation looked up altFileSystems with an empty key and every
// call that named no alt filesystem failed with
//
//	alt fs '' is not attached
//
// AddResource special-cases the empty string as "the default filesystem"
// (BH-023, resource_upload_context.go:903); the local path never got that
// branch, and its three existing tests all set a real alt-fs key, so the
// ordinary case was exercised nowhere.
func TestAddLocalResource_EmptyPathNameUsesDefaultFilesystem(t *testing.T) {
	ctx := createTestContext(t)

	const localPath = "/default-fs-file.txt"
	purgeResourcesAt(t, ctx, localPath)

	// The default filesystem, not an alt one. No ctx.altFileSystems entry.
	testContent := []byte("default filesystem content")
	if err := afero.WriteFile(ctx.fs, localPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	resource, err := ctx.AddLocalResource("default-fs-file.txt", &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Default FS Resource",
			Meta: "{}",
		},
		LocalPath: localPath,
		PathName:  "",
	})
	if err != nil {
		t.Fatalf("AddLocalResource() with empty PathName error = %v", err)
	}
	if resource == nil {
		t.Fatal("AddLocalResource() returned nil resource")
	}

	// Persistence has to normalize too, or the row stores a pointer to the
	// empty string and the read sites calling GetFsForStorageLocation fail the
	// same way the create did. nil is the convention: AddResource
	// (resource_upload_context.go:903) and group import
	// (groupio/apply_import.go:1377) both leave it nil when the key is empty,
	// and export treats nil and empty alike.
	if resource.StorageLocation != nil {
		t.Errorf("StorageLocation should be nil for the default filesystem, got %q", *resource.StorageLocation)
	}

	// The initial version copies the resource's binding, so it has to be nil
	// too -- a version row pointing at an unattached alt fs cannot be served.
	var version models.ResourceVersion
	if err := ctx.db.Where("resource_id = ?", resource.ID).Order("version_number asc").First(&version).Error; err != nil {
		t.Fatalf("load initial version: %v", err)
	}
	if version.StorageLocation != nil {
		t.Errorf("initial version StorageLocation should be nil, got %q", *version.StorageLocation)
	}

	// And the bytes actually came from the default filesystem.
	if resource.FileSize != int64(len(testContent)) {
		t.Errorf("FileSize = %d, want %d", resource.FileSize, len(testContent))
	}
}

// TestAddLocalResource_EmptyPathNameDedupes covers the other half of storing
// NULL: the "resource already exists, skipping" lookup compared
// storage_location with =, and SQL three-valued logic never matches NULL that
// way. A create path that stores NULL but dedupes on the empty string silently
// stops being idempotent and creates a second row for the same file.
func TestAddLocalResource_EmptyPathNameDedupes(t *testing.T) {
	ctx := createTestContext(t)

	const localPath = "/dedupe-default-fs.txt"
	purgeResourcesAt(t, ctx, localPath)

	testContent := []byte("dedupe me on the default filesystem")
	if err := afero.WriteFile(ctx.fs, localPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	creator := func() *query_models.ResourceFromLocalCreator {
		return &query_models.ResourceFromLocalCreator{
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "Dedupe Default FS",
				Meta: "{}",
			},
			LocalPath: localPath,
			PathName:  "",
		}
	}

	first, err := ctx.AddLocalResource("dedupe-default-fs.txt", creator())
	if err != nil {
		t.Fatalf("first AddLocalResource() error = %v", err)
	}

	second, err := ctx.AddLocalResource("dedupe-default-fs.txt", creator())
	if err != nil {
		t.Fatalf("second AddLocalResource() error = %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("second call created a new resource (%d) instead of returning the existing one (%d)", second.ID, first.ID)
	}

	// Scoped to the default filesystem: the same path on a different alt fs is
	// a different resource, so counting the path alone would assert too much.
	var rowCount int64
	ctx.db.Model(&models.Resource{}).Where("location = ? AND storage_location IS NULL", localPath).Count(&rowCount)
	if rowCount != 1 {
		t.Errorf("expected exactly 1 default-filesystem resource row for the path, got %d", rowCount)
	}
}

// TestAddLocalResource_AltPathNameStillStored is the other direction: naming a
// real alt filesystem must still bind the resource to it. Normalizing the
// empty key must not normalize a real one away.
func TestAddLocalResource_AltPathNameStillStored(t *testing.T) {
	ctx := createTestContext(t)

	const localPath = "/alt-bound.txt"
	purgeResourcesAt(t, ctx, localPath)

	altFs := afero.NewMemMapFs()
	ctx.altFileSystems["altstore"] = altFs
	if err := afero.WriteFile(altFs, localPath, []byte("on the alt fs"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	resource, err := ctx.AddLocalResource("alt-bound.txt", &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Alt Bound Resource",
			Meta: "{}",
		},
		LocalPath: localPath,
		PathName:  "altstore",
	})
	if err != nil {
		t.Fatalf("AddLocalResource() error = %v", err)
	}

	if resource.StorageLocation == nil {
		t.Fatal("StorageLocation should be set for an alt filesystem, got nil")
	}
	if *resource.StorageLocation != "altstore" {
		t.Errorf("StorageLocation = %q, want %q", *resource.StorageLocation, "altstore")
	}
}

// TestAddLocalResource_DefaultsNameAndMeta pins the two normalizations every
// sibling create path performs and this one did not.
//
// The handler calls AddLocalResource(creator.Name, &creator), so a request that
// names nothing arrives with both fileName and Name empty. AddResource gets a
// real filename from the upload it is handed; the local path is the only name
// this path has.
//
// Meta matters more than it looks. Empty Meta becomes []byte("") on the model,
// which is an invalid json.RawMessage, so encoding the created resource fails
// *after* the row has committed and the status line has been written -- the
// caller gets HTTP 200 with a zero-byte body, and `mr resource from-local
// --json` prints nothing at all while the resource quietly exists. AddResource
// (:891), CreateGroup (group_crud_context.go:26) and the note path
// (note_context.go:33) all default it to "{}" first.
//
// This was unreachable until the empty PathName was fixed: every call that
// named no alt filesystem failed before it got here.
func TestAddLocalResource_DefaultsNameAndMeta(t *testing.T) {
	ctx := createTestContext(t)

	const localPath = "/incoming/defaults-probe.txt"
	purgeResourcesAt(t, ctx, localPath)

	if err := afero.WriteFile(ctx.fs, localPath, []byte("defaults probe"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Exactly what `mr resource from-local --path <p>` sends: nothing else.
	resource, err := ctx.AddLocalResource("", &query_models.ResourceFromLocalCreator{
		LocalPath: localPath,
	})
	if err != nil {
		t.Fatalf("AddLocalResource() error = %v", err)
	}

	if resource.Name != "defaults-probe.txt" {
		t.Errorf("Name = %q, want the file's base name %q", resource.Name, "defaults-probe.txt")
	}
	if resource.OriginalName != "defaults-probe.txt" {
		t.Errorf("OriginalName = %q, want %q", resource.OriginalName, "defaults-probe.txt")
	}

	// The row has to hold JSON a client can decode. Anything else turns the
	// create response into a 200 with no body.
	if !json.Valid(resource.Meta) {
		t.Errorf("Meta is not valid JSON: %q", string(resource.Meta))
	}

	// And the whole resource has to survive the encoder the handler uses.
	if _, err := json.Marshal(resource); err != nil {
		t.Errorf("created resource does not encode, so the API returns 200 with an empty body: %v", err)
	}
}

// TestAddLocalResource_RejectsInvalidMeta is the other half: the default must
// not be reached by way of skipping validation. AddResource validates at :895.
func TestAddLocalResource_RejectsInvalidMeta(t *testing.T) {
	ctx := createTestContext(t)

	const localPath = "/incoming/bad-meta.txt"
	purgeResourcesAt(t, ctx, localPath)

	if err := afero.WriteFile(ctx.fs, localPath, []byte("bad meta"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := ctx.AddLocalResource("bad-meta.txt", &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Meta: "{not json"},
		LocalPath:         localPath,
	})
	if err == nil {
		t.Fatal("AddLocalResource() accepted invalid Meta; want an error")
	}

	var count int64
	ctx.db.Model(&models.Resource{}).Where("location = ?", localPath).Count(&count)
	if count != 0 {
		t.Errorf("a rejected create left %d row(s) behind", count)
	}
}
