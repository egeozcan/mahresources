package api_tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
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
