package application_context

import (
	"testing"

	"github.com/spf13/afero"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestAddLocalResource_CreatesInitialVersion verifies that AddLocalResource
// creates version 1 for the new resource and sets CurrentVersionID, matching
// the behavior of AddResource.
//
// BUG: AddLocalResource does NOT create an initial version record. Resources
// created through AddLocalResource have CurrentVersionID = nil and no version
// rows, unlike resources created through AddResource which get v1 immediately.
// This means GetVersions returns a virtual v1 (ID=0) instead of a real one,
// and the resource is in an inconsistent state until MigrateResourceVersions
// runs at next startup.
func TestAddLocalResource_CreatesInitialVersion(t *testing.T) {
	ctx := createTestContext(t)

	// Migrate ResourceVersion table (not in the default createTestContext setup)
	if err := ctx.db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Set up the alt filesystem so AddLocalResource can find the file.
	altFs := afero.NewMemMapFs()
	ctx.altFileSystems["testfs"] = altFs

	// Create a test file on the alt filesystem
	testContent := []byte("version test file content for local upload")
	if err := afero.WriteFile(altFs, "/version-test.txt", testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a resource via AddLocalResource
	resource, err := ctx.AddLocalResource("version-test.txt", &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Version Test Resource",
			Meta: `{"key":"value"}`,
		},
		LocalPath: "/version-test.txt",
		PathName:  "testfs",
	})
	if err != nil {
		t.Fatalf("AddLocalResource() error = %v", err)
	}
	if resource == nil {
		t.Fatal("AddLocalResource() returned nil resource")
	}

	// Reload the resource from DB to get the latest state
	var reloaded models.Resource
	if err := ctx.db.First(&reloaded, resource.ID).Error; err != nil {
		t.Fatalf("Failed to reload resource: %v", err)
	}

	// BUG: CurrentVersionID should NOT be nil for a newly created resource.
	// AddResource sets it, but AddLocalResource does not.
	if reloaded.CurrentVersionID == nil {
		t.Error("AddLocalResource should set CurrentVersionID (like AddResource does), but it is nil")
	}

	// Verify that a version record exists in the database
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resource.ID).Count(&versionCount)
	if versionCount != 1 {
		t.Errorf("AddLocalResource should create 1 initial version record, got %d", versionCount)
	}

	// If a version was created, verify its fields match the resource
	if versionCount > 0 {
		var version models.ResourceVersion
		ctx.db.Where("resource_id = ?", resource.ID).First(&version)

		if version.VersionNumber != 1 {
			t.Errorf("Initial version should be v1, got v%d", version.VersionNumber)
		}
		if version.Hash != reloaded.Hash {
			t.Errorf("Version hash = %q, want %q", version.Hash, reloaded.Hash)
		}
		if version.FileSize != reloaded.FileSize {
			t.Errorf("Version file size = %d, want %d", version.FileSize, reloaded.FileSize)
		}
		if version.ContentType != reloaded.ContentType {
			t.Errorf("Version content type = %q, want %q", version.ContentType, reloaded.ContentType)
		}
		if version.Location != reloaded.Location {
			t.Errorf("Version location = %q, want %q", version.Location, reloaded.Location)
		}
	}

	// Clean up (shared DB)
	ctx.db.Where("resource_id = ?", resource.ID).Delete(&models.ResourceVersion{})
	ctx.db.Delete(&models.Resource{}, resource.ID)
}
