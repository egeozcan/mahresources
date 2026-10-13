//go:build json1 && fts5

package application_context

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models/query_models"
)

func TestAddResource_ReusedQueryDoesNotMutateOriginalName(t *testing.T) {
	ctx := createIsolatedTestContext(t)

	creator := &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Test Upload",
		},
	}

	content1 := []byte("content 1")
	file1 := io.NopCloser(bytes.NewReader(content1))
	res1, err := ctx.AddResource(file1, "file1.txt", creator)
	assert.NoError(t, err)
	assert.Equal(t, "file1.txt", res1.OriginalName)

	// Ensure the creator wasn't mutated
	assert.Empty(t, creator.OriginalName)

	content2 := []byte("content 2")
	file2 := io.NopCloser(bytes.NewReader(content2))
	res2, err := ctx.AddResource(file2, "file2.txt", creator)
	assert.NoError(t, err)
	assert.Equal(t, "file2.txt", res2.OriginalName)
}

func TestAddLocalResource_ReusedQueryDoesNotMutateOriginalName(t *testing.T) {
	ctx := createIsolatedTestContext(t)

	// Write two temp files to local storage
	fs := ctx.fs
	file1Path := "/tmp/file1.txt"
	file2Path := "/tmp/file2.txt"

	_ = fs.MkdirAll("/tmp", 0777)
	f1, _ := fs.Create(file1Path)
	f1.Write([]byte("local 1"))
	f1.Close()

	f2, _ := fs.Create(file2Path)
	f2.Write([]byte("local 2"))
	f2.Close()

	creator := &query_models.ResourceFromLocalCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Test Local Upload",
		},
		LocalPath: file1Path,
	}

	res1, err := ctx.AddLocalResource("file1.txt", creator)
	assert.NoError(t, err)
	assert.Equal(t, "file1.txt", res1.OriginalName)

	// Ensure the creator wasn't mutated
	assert.Empty(t, creator.OriginalName)

	creator.LocalPath = file2Path
	res2, err := ctx.AddLocalResource("file2.txt", creator)
	assert.NoError(t, err)
	assert.Equal(t, "file2.txt", res2.OriginalName)
}
