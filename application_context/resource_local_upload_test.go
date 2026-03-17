package application_context

import (
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
