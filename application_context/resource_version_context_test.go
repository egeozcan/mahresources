package application_context

import (
	"bytes"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/models"
)

// createVersionTestContext creates a minimal test context for version testing
func createVersionTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	return createTestContext(t)
}

// TestComputeSHA1 tests the SHA1 hash computation
func TestComputeSHA1(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty content",
			input:    []byte{},
			expected: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name:     "hello world",
			input:    []byte("hello world"),
			expected: "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
		},
		{
			name:     "binary content",
			input:    []byte{0x00, 0x01, 0x02, 0x03},
			expected: "a02a05b025b928c039cf1ae7e8ee04e7c190c0db",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := computeSHA1(tc.input)
			if result != tc.expected {
				t.Errorf("computeSHA1(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestDetectContentType tests MIME type detection
func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		wantContain string
	}{
		{
			name:        "plain text",
			input:       []byte("hello world"),
			wantContain: "text/plain",
		},
		{
			name:        "JSON content",
			input:       []byte(`{"key": "value"}`),
			wantContain: "application/json",
		},
		{
			name: "PNG header",
			input: []byte{
				0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
				0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
			},
			wantContain: "image/png",
		},
		{
			name: "JPEG header",
			input: []byte{
				0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
				0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
			},
			wantContain: "image/jpeg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detectContentType(tc.input)
			if !bytes.Contains([]byte(result), []byte(tc.wantContain)) {
				t.Errorf("detectContentType() = %q, want to contain %q", result, tc.wantContain)
			}
		})
	}
}

// TestGetDimensionsFromContent tests image dimension extraction
func TestGetDimensionsFromContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		contentType string
		wantWidth   uint
		wantHeight  uint
	}{
		{
			name:        "non-image content",
			content:     []byte("hello world"),
			contentType: "text/plain",
			wantWidth:   0,
			wantHeight:  0,
		},
		{
			name:        "invalid image content",
			content:     []byte("not an image"),
			contentType: "image/png",
			wantWidth:   0,
			wantHeight:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			width, height := getDimensionsFromContent(tc.content, tc.contentType)
			if width != tc.wantWidth || height != tc.wantHeight {
				t.Errorf("getDimensionsFromContent() = (%d, %d), want (%d, %d)",
					width, height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

// TestBuildVersionResourcePath tests path construction
func TestBuildVersionResourcePath(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		ext      string
		expected string
	}{
		{
			name:     "with extension",
			hash:     "abcdef123456789012345678901234567890abcd",
			ext:      ".txt",
			expected: "/resources/ab/cd/ef/abcdef123456789012345678901234567890abcd.txt",
		},
		{
			name:     "without extension",
			hash:     "1234567890abcdef1234567890abcdef12345678",
			ext:      "",
			expected: "/resources/12/34/56/1234567890abcdef1234567890abcdef12345678",
		},
		{
			name:     "with png extension",
			hash:     "da39a3ee5e6b4b0d3255bfef95601890afd80709",
			ext:      ".png",
			expected: "/resources/da/39/a3/da39a3ee5e6b4b0d3255bfef95601890afd80709.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildVersionResourcePath(tc.hash, tc.ext)
			if result != tc.expected {
				t.Errorf("buildVersionResourcePath(%q, %q) = %q, want %q",
					tc.hash, tc.ext, result, tc.expected)
			}
		})
	}
}

// TestGetExtensionFromFilename tests extension extraction
func TestGetExtensionFromFilename(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		expected    string
	}{
		{
			name:        "filename with extension",
			filename:    "document.pdf",
			contentType: "application/pdf",
			expected:    ".pdf",
		},
		{
			name:        "filename without extension uses contentType",
			filename:    "document",
			contentType: "text/plain",
			expected:    ".txt",
		},
		{
			name:        "uppercase extension",
			filename:    "IMAGE.PNG",
			contentType: "image/png",
			expected:    ".PNG",
		},
		{
			name:        "multiple dots in filename",
			filename:    "file.name.txt",
			contentType: "text/plain",
			expected:    ".txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getExtensionFromFilename(tc.filename, tc.contentType)
			if result != tc.expected {
				t.Errorf("getExtensionFromFilename(%q, %q) = %q, want %q",
					tc.filename, tc.contentType, result, tc.expected)
			}
		})
	}
}

// ptrUint is a helper to create a pointer to a uint
func ptrUint(v uint) *uint {
	return &v
}

// TestCountHashReferences tests hash reference counting
func TestCountHashReferences(t *testing.T) {
	ctx := createVersionTestContext(t)

	// Add ResourceVersion to migrations for this test
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	testHash := "abc123def456"

	// Initially, no references
	count, err := ctx.CountHashReferences(testHash)
	if err != nil {
		t.Fatalf("CountHashReferences() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 references initially, got %d", count)
	}

	// Create a resource with this hash
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     testHash,
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Should now have 1 reference (from resource)
	count, err = ctx.CountHashReferences(testHash)
	if err != nil {
		t.Fatalf("CountHashReferences() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 reference after creating resource, got %d", count)
	}

	// Create a version with the same hash
	version := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          testHash,
		HashType:      "SHA1",
		Location:      "/test/location",
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Should now have 2 references
	count, err = ctx.CountHashReferences(testHash)
	if err != nil {
		t.Fatalf("CountHashReferences() error = %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 references after creating version, got %d", count)
	}

	// Create another version with same hash
	version2 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 2,
		Hash:          testHash,
		HashType:      "SHA1",
		Location:      "/test/location",
	}
	if err := db.Create(version2).Error; err != nil {
		t.Fatalf("Failed to create version2: %v", err)
	}

	// Should now have 3 references
	count, err = ctx.CountHashReferences(testHash)
	if err != nil {
		t.Fatalf("CountHashReferences() error = %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 references, got %d", count)
	}
}

// TestGetVersions tests retrieving all versions for a resource
func TestGetVersions(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Initially no versions
	versions, err := ctx.GetVersions(resource.ID)
	if err != nil {
		t.Fatalf("GetVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("Expected 0 versions initially, got %d", len(versions))
	}

	// Create some versions
	for i := 1; i <= 3; i++ {
		version := &models.ResourceVersion{
			ResourceID:    resource.ID,
			VersionNumber: i,
			Hash:          "hash-" + string(rune('a'+i-1)),
			HashType:      "SHA1",
			Location:      "/test/location",
			Comment:       "Version " + string(rune('0'+i)),
		}
		if err := db.Create(version).Error; err != nil {
			t.Fatalf("Failed to create version %d: %v", i, err)
		}
	}

	// Should return versions in descending order
	versions, err = ctx.GetVersions(resource.ID)
	if err != nil {
		t.Fatalf("GetVersions() error = %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got %d", len(versions))
	}

	// Check order (should be descending by version number)
	for i, v := range versions {
		expectedVersionNum := 3 - i
		if v.VersionNumber != expectedVersionNum {
			t.Errorf("Version at index %d has VersionNumber %d, expected %d",
				i, v.VersionNumber, expectedVersionNum)
		}
	}
}

// TestGetVersion tests retrieving a specific version by ID
func TestGetVersion(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create a version
	version := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "version-hash",
		HashType:      "SHA1",
		Location:      "/test/location",
		Comment:       "Test comment",
		FileSize:      1234,
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Retrieve by ID
	retrieved, err := ctx.GetVersion(version.ID)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if retrieved.ID != version.ID {
		t.Errorf("Retrieved version ID = %d, want %d", retrieved.ID, version.ID)
	}
	if retrieved.Hash != "version-hash" {
		t.Errorf("Retrieved version Hash = %q, want %q", retrieved.Hash, "version-hash")
	}
	if retrieved.Comment != "Test comment" {
		t.Errorf("Retrieved version Comment = %q, want %q", retrieved.Comment, "Test comment")
	}
	if retrieved.FileSize != 1234 {
		t.Errorf("Retrieved version FileSize = %d, want %d", retrieved.FileSize, 1234)
	}

	// Try to get non-existent version
	_, err = ctx.GetVersion(99999)
	if err == nil {
		t.Error("Expected error when getting non-existent version, got nil")
	}
}

// TestGetVersionByNumber tests retrieving a version by resource ID and version number
func TestGetVersionByNumber(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create multiple versions
	for i := 1; i <= 3; i++ {
		version := &models.ResourceVersion{
			ResourceID:    resource.ID,
			VersionNumber: i,
			Hash:          "hash-v" + string(rune('0'+i)),
			HashType:      "SHA1",
			Location:      "/test/location",
		}
		if err := db.Create(version).Error; err != nil {
			t.Fatalf("Failed to create version %d: %v", i, err)
		}
	}

	// Retrieve by version number
	v2, err := ctx.GetVersionByNumber(resource.ID, 2)
	if err != nil {
		t.Fatalf("GetVersionByNumber() error = %v", err)
	}
	if v2.VersionNumber != 2 {
		t.Errorf("Retrieved version number = %d, want 2", v2.VersionNumber)
	}
	if v2.Hash != "hash-v2" {
		t.Errorf("Retrieved version hash = %q, want %q", v2.Hash, "hash-v2")
	}

	// Try to get non-existent version number
	_, err = ctx.GetVersionByNumber(resource.ID, 99)
	if err == nil {
		t.Error("Expected error when getting non-existent version number, got nil")
	}

	// Try to get version for non-existent resource
	_, err = ctx.GetVersionByNumber(99999, 1)
	if err == nil {
		t.Error("Expected error when getting version for non-existent resource, got nil")
	}
}

// TestCompareVersions tests version comparison functionality
func TestCompareVersions(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create two versions with different properties
	version1 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "hash-v1",
		HashType:      "SHA1",
		Location:      "/test/location",
		FileSize:      1000,
		ContentType:   "text/plain",
		Width:         100,
		Height:        100,
	}
	if err := db.Create(version1).Error; err != nil {
		t.Fatalf("Failed to create version1: %v", err)
	}

	version2 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 2,
		Hash:          "hash-v2",
		HashType:      "SHA1",
		Location:      "/test/location",
		FileSize:      1500,
		ContentType:   "text/plain",
		Width:         200,
		Height:        150,
	}
	if err := db.Create(version2).Error; err != nil {
		t.Fatalf("Failed to create version2: %v", err)
	}

	// Compare versions
	comparison, err := ctx.CompareVersions(resource.ID, version1.ID, version2.ID)
	if err != nil {
		t.Fatalf("CompareVersions() error = %v", err)
	}

	if comparison.SizeDelta != 500 {
		t.Errorf("SizeDelta = %d, want 500", comparison.SizeDelta)
	}
	if comparison.SameHash {
		t.Error("SameHash = true, want false")
	}
	if !comparison.SameType {
		t.Error("SameType = false, want true")
	}
	if !comparison.DimensionsDiff {
		t.Error("DimensionsDiff = false, want true")
	}
}

// TestCompareVersions_SameHash tests comparison when versions have same hash
func TestCompareVersions_SameHash(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create two versions with same hash (restored version scenario)
	sameHash := "identical-hash"
	version1 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          sameHash,
		HashType:      "SHA1",
		Location:      "/test/location",
		FileSize:      1000,
		ContentType:   "image/png",
		Width:         100,
		Height:        100,
	}
	if err := db.Create(version1).Error; err != nil {
		t.Fatalf("Failed to create version1: %v", err)
	}

	version2 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 2,
		Hash:          sameHash,
		HashType:      "SHA1",
		Location:      "/test/location",
		FileSize:      1000,
		ContentType:   "image/png",
		Width:         100,
		Height:        100,
	}
	if err := db.Create(version2).Error; err != nil {
		t.Fatalf("Failed to create version2: %v", err)
	}

	comparison, err := ctx.CompareVersions(resource.ID, version1.ID, version2.ID)
	if err != nil {
		t.Fatalf("CompareVersions() error = %v", err)
	}

	if !comparison.SameHash {
		t.Error("SameHash = false, want true")
	}
	if comparison.SizeDelta != 0 {
		t.Errorf("SizeDelta = %d, want 0", comparison.SizeDelta)
	}
	if !comparison.SameType {
		t.Error("SameType = false, want true")
	}
	if comparison.DimensionsDiff {
		t.Error("DimensionsDiff = true, want false")
	}
}

// TestCompareVersions_InvalidResource tests error handling for mismatched resources
func TestCompareVersions_InvalidResource(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create two different resources
	resource1 := &models.Resource{Name: "resource1", Hash: "h1", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l1"}
	resource2 := &models.Resource{Name: "resource2", Hash: "h2", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l2"}
	if err := db.Create(resource1).Error; err != nil {
		t.Fatalf("Failed to create resource1: %v", err)
	}
	if err := db.Create(resource2).Error; err != nil {
		t.Fatalf("Failed to create resource2: %v", err)
	}

	// Create versions for each resource
	v1 := &models.ResourceVersion{ResourceID: resource1.ID, VersionNumber: 1, Hash: "v1h", HashType: "SHA1", Location: "/l1"}
	v2 := &models.ResourceVersion{ResourceID: resource2.ID, VersionNumber: 1, Hash: "v2h", HashType: "SHA1", Location: "/l2"}
	if err := db.Create(v1).Error; err != nil {
		t.Fatalf("Failed to create v1: %v", err)
	}
	if err := db.Create(v2).Error; err != nil {
		t.Fatalf("Failed to create v2: %v", err)
	}

	// Try to compare versions from different resources
	_, err := ctx.CompareVersions(resource1.ID, v1.ID, v2.ID)
	if err == nil {
		t.Error("Expected error when comparing versions from different resources, got nil")
	}
}

// TestDeleteVersion tests version deletion logic
func TestDeleteVersion(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource with multiple versions
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create 3 versions
	var versions []*models.ResourceVersion
	for i := 1; i <= 3; i++ {
		v := &models.ResourceVersion{
			ResourceID:    resource.ID,
			VersionNumber: i,
			Hash:          "hash-" + string(rune('a'+i-1)),
			HashType:      "SHA1",
			Location:      "/test/location",
		}
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("Failed to create version %d: %v", i, err)
		}
		versions = append(versions, v)
	}

	// Set version 3 as current
	currentVersionID := versions[2].ID
	if err := db.Model(resource).Update("current_version_id", currentVersionID).Error; err != nil {
		t.Fatalf("Failed to set current version: %v", err)
	}

	// Cannot delete current version
	err := ctx.DeleteVersion(resource.ID, currentVersionID)
	if err == nil {
		t.Error("Expected error when deleting current version, got nil")
	}

	// Can delete non-current version
	err = ctx.DeleteVersion(resource.ID, versions[0].ID)
	if err != nil {
		t.Fatalf("DeleteVersion() error = %v", err)
	}

	// Verify version was deleted
	var count int64
	db.Model(&models.ResourceVersion{}).Where("id = ?", versions[0].ID).Count(&count)
	if count != 0 {
		t.Error("Version was not deleted")
	}
}

// TestDeleteVersion_LastVersion tests that the last version cannot be deleted
func TestDeleteVersion_LastVersion(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource with only one version
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	version := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "only-version-hash",
		HashType:      "SHA1",
		Location:      "/test/location",
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Try to delete the only version
	err := ctx.DeleteVersion(resource.ID, version.ID)
	if err == nil {
		t.Error("Expected error when deleting last version, got nil")
	}
}

// TestDeleteVersion_WrongResource tests deleting a version that belongs to a different resource
func TestDeleteVersion_WrongResource(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create two resources
	resource1 := &models.Resource{Name: "r1", Hash: "h1", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l1"}
	resource2 := &models.Resource{Name: "r2", Hash: "h2", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l2"}
	if err := db.Create(resource1).Error; err != nil {
		t.Fatalf("Failed to create resource1: %v", err)
	}
	if err := db.Create(resource2).Error; err != nil {
		t.Fatalf("Failed to create resource2: %v", err)
	}

	// Create version for resource2
	version := &models.ResourceVersion{
		ResourceID:    resource2.ID,
		VersionNumber: 1,
		Hash:          "version-hash",
		HashType:      "SHA1",
		Location:      "/test/location",
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Try to delete version using wrong resource ID
	err := ctx.DeleteVersion(resource1.ID, version.ID)
	if err == nil {
		t.Error("Expected error when deleting version with wrong resource ID, got nil")
	}
}

// TestRestoreVersion tests version restoration
func TestRestoreVersion(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create initial versions
	v1 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "v1-hash",
		HashType:      "SHA1",
		Location:      "/v1/location",
		FileSize:      100,
		ContentType:   "text/plain",
	}
	if err := db.Create(v1).Error; err != nil {
		t.Fatalf("Failed to create v1: %v", err)
	}

	v2 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 2,
		Hash:          "v2-hash",
		HashType:      "SHA1",
		Location:      "/v2/location",
		FileSize:      200,
		ContentType:   "text/html",
	}
	if err := db.Create(v2).Error; err != nil {
		t.Fatalf("Failed to create v2: %v", err)
	}

	// Restore v1 (creates v3 with v1's data)
	restored, err := ctx.RestoreVersion(resource.ID, v1.ID, "Custom restore comment")
	if err != nil {
		t.Fatalf("RestoreVersion() error = %v", err)
	}

	if restored.VersionNumber != 3 {
		t.Errorf("Restored version number = %d, want 3", restored.VersionNumber)
	}
	if restored.Hash != v1.Hash {
		t.Errorf("Restored hash = %q, want %q", restored.Hash, v1.Hash)
	}
	if restored.FileSize != v1.FileSize {
		t.Errorf("Restored file size = %d, want %d", restored.FileSize, v1.FileSize)
	}
	if restored.Comment != "Custom restore comment" {
		t.Errorf("Restored comment = %q, want %q", restored.Comment, "Custom restore comment")
	}

	// Verify resource's current version is updated
	var updatedResource models.Resource
	if err := db.First(&updatedResource, resource.ID).Error; err != nil {
		t.Fatalf("Failed to get updated resource: %v", err)
	}
	if updatedResource.CurrentVersionID == nil || *updatedResource.CurrentVersionID != restored.ID {
		t.Error("Resource current version was not updated to restored version")
	}
}

// TestRestoreVersion_DefaultComment tests default comment generation
func TestRestoreVersion_DefaultComment(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a test resource
	resource := &models.Resource{
		Name:     "test-resource",
		Hash:     "test-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/test/location",
	}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	v1 := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 1,
		Hash:          "v1-hash",
		HashType:      "SHA1",
		Location:      "/v1/location",
	}
	if err := db.Create(v1).Error; err != nil {
		t.Fatalf("Failed to create v1: %v", err)
	}

	// Restore with empty comment - should get default
	restored, err := ctx.RestoreVersion(resource.ID, v1.ID, "")
	if err != nil {
		t.Fatalf("RestoreVersion() error = %v", err)
	}

	expectedComment := "Restored from version 1"
	if restored.Comment != expectedComment {
		t.Errorf("Default comment = %q, want %q", restored.Comment, expectedComment)
	}
}

// TestRestoreVersion_WrongResource tests restoring a version from a different resource
func TestRestoreVersion_WrongResource(t *testing.T) {
	ctx := createVersionTestContext(t)
	db := ctx.db
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create a test group as owner
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create two resources
	resource1 := &models.Resource{Name: "r1", Hash: "h1", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l1"}
	resource2 := &models.Resource{Name: "r2", Hash: "h2", HashType: "SHA1", OwnerId: ptrUint(ownerGroup.ID), Location: "/l2"}
	if err := db.Create(resource1).Error; err != nil {
		t.Fatalf("Failed to create resource1: %v", err)
	}
	if err := db.Create(resource2).Error; err != nil {
		t.Fatalf("Failed to create resource2: %v", err)
	}

	// Create version for resource2
	version := &models.ResourceVersion{
		ResourceID:    resource2.ID,
		VersionNumber: 1,
		Hash:          "version-hash",
		HashType:      "SHA1",
		Location:      "/test/location",
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Try to restore version to wrong resource
	_, err := ctx.RestoreVersion(resource1.ID, version.ID, "")
	if err == nil {
		t.Error("Expected error when restoring version to wrong resource, got nil")
	}
}

// TestMigrateResourceVersions_MigratesResourcesWithoutVersions tests that migration creates versions for resources without them
func TestMigrateResourceVersions_MigratesResourcesWithoutVersions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Migrate models
	err = db.AutoMigrate(
		&models.Resource{},
		&models.Group{},
		&models.ResourceVersion{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Create context using the shared test helper pattern
	ctx := createVersionTestContext(t)
	// Override db to use our test db
	ctx.db = db

	// Migrate ResourceVersion manually for this context
	if err := db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("Failed to migrate ResourceVersion: %v", err)
	}

	// Create an owner group
	ownerGroup := &models.Group{Name: "test-owner"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}

	// Create a resource with a version (simulates already migrated)
	resource1 := &models.Resource{
		Name:     "migrated-resource",
		Hash:     "migrated-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/migrated",
	}
	if err := db.Create(resource1).Error; err != nil {
		t.Fatalf("Failed to create resource1: %v", err)
	}

	version := &models.ResourceVersion{
		ResourceID:    resource1.ID,
		VersionNumber: 1,
		Hash:          "migrated-hash",
		HashType:      "SHA1",
		Location:      "/migrated",
	}
	if err := db.Create(version).Error; err != nil {
		t.Fatalf("Failed to create existing version: %v", err)
	}

	// Update resource1 to have CurrentVersionID set
	db.Model(resource1).Update("current_version_id", version.ID)

	// Create a resource without version (should be migrated)
	resource2 := &models.Resource{
		Name:     "unmigrated-resource",
		Hash:     "unmigrated-hash",
		HashType: "SHA1",
		OwnerId:  ptrUint(ownerGroup.ID),
		Location: "/unmigrated",
	}
	if err := db.Create(resource2).Error; err != nil {
		t.Fatalf("Failed to create resource2: %v", err)
	}

	// Run migration - should migrate resource2 even though resource1 has versions
	err = ctx.MigrateResourceVersions()
	if err != nil {
		t.Fatalf("MigrateResourceVersions() error = %v", err)
	}

	// Verify resource2 WAS migrated
	var count int64
	db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resource2.ID).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 version for unmigrated resource, got %d", count)
	}

	// Verify resource1 still has only 1 version (not duplicated)
	db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resource1.ID).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 version for already migrated resource, got %d", count)
	}

	// Verify resource2 now has CurrentVersionID set
	var updatedResource models.Resource
	db.First(&updatedResource, resource2.ID)
	if updatedResource.CurrentVersionID == nil {
		t.Error("Expected CurrentVersionID to be set after migration")
	}
}
