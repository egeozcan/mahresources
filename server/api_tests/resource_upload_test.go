package api_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestMultiFileUploadEachFileGetsOwnOriginalName(t *testing.T) {
	tc := SetupTestEnv(t)

	// Build a multipart form with 2 distinct files
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// File 1: a text file named "report.txt"
	part1, err := writer.CreateFormFile("resource", "report.txt")
	assert.NoError(t, err)
	_, _ = part1.Write([]byte("contents of report file"))

	// File 2: a different text file named "notes.txt"
	part2, err := writer.CreateFormFile("resource", "notes.txt")
	assert.NoError(t, err)
	_, _ = part2.Write([]byte("contents of notes file - different"))

	writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resources []*models.Resource
	err = json.Unmarshal(rr.Body.Bytes(), &resources)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(resources), "should return 2 resources")

	// Each resource should have its OWN filename as OriginalName
	// Bug: the second resource inherits the first file's OriginalName
	// because AddResource mutates the shared ResourceCreator
	var res1, res2 models.Resource
	tc.DB.First(&res1, resources[0].ID)
	tc.DB.First(&res2, resources[1].ID)

	assert.Equal(t, "report.txt", res1.OriginalName,
		"First resource should have its own filename as OriginalName")
	assert.Equal(t, "notes.txt", res2.OriginalName,
		"Second resource should have its own filename as OriginalName, not the first file's name")
}

func TestDuplicateUploadAppendsTagsToExistingResource(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create an owner group and two tags
	owner := tc.CreateDummyGroup("Upload Owner")
	tag1 := &models.Tag{Name: "First Tag"}
	tag2 := &models.Tag{Name: "Second Tag"}
	tc.DB.Create(tag1)
	tc.DB.Create(tag2)

	fileContent := []byte("identical content for duplicate test xyz")

	// First upload: file with tag1
	file1 := io.NopCloser(bytes.NewReader(fileContent))
	res, err := tc.AppCtx.AddResource(file1, "test.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:    "First Upload",
			OwnerId: owner.ID,
			Tags:    []uint{tag1.ID},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Verify tag1 is on the resource
	var countT1 int64
	tc.DB.Table("resource_tags").Where("resource_id = ? AND tag_id = ?", res.ID, tag1.ID).Count(&countT1)
	assert.Equal(t, int64(1), countT1, "setup: first upload should have tag1")

	// Second upload: same content, same owner, but with tag2
	file2 := io.NopCloser(bytes.NewReader(fileContent))
	_, dupErr := tc.AppCtx.AddResource(file2, "test.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:    "Second Upload",
			OwnerId: owner.ID,
			Tags:    []uint{tag2.ID},
		},
	})
	// Duplicate error is expected
	assert.Error(t, dupErr)

	// But tag2 should have been appended to the existing resource
	var countT2 int64
	tc.DB.Table("resource_tags").Where("resource_id = ? AND tag_id = ?", res.ID, tag2.ID).Count(&countT2)
	assert.Equal(t, int64(1), countT2,
		"Duplicate upload should append new tags to the existing resource, not silently discard them")
}
