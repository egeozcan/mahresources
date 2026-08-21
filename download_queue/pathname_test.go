package download_queue

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// pathNameCapturingCreator records the whole ResourceCreator the worker builds,
// not just its name, so the assertion can be about the field that was dropped.
type pathNameCapturingCreator struct {
	mu      sync.Mutex
	creator query_models.ResourceCreator
}

func (c *pathNameCapturingCreator) AddResource(file contracts.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.Copy(io.Discard, file)
	c.mu.Lock()
	c.creator = *resourceQuery
	c.mu.Unlock()
	return &models.Resource{ID: 1, Name: resourceQuery.Name}, nil
}

func (c *pathNameCapturingCreator) captured() query_models.ResourceCreator {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creator
}

// B4, background half. The worker converts the submitted
// ResourceFromRemoteCreator into a ResourceCreator field by field and copied
// every field except PathName, so a queued download ignored the storage
// location the submitter chose and wrote to the main filesystem.
//
// This half matters more than the foreground one: the payload is persisted in
// the download history and replayed on retry, so a fix that covered only the
// foreground call would make a retried download land somewhere other than the
// original.
func TestDownloadWithProgressForwardsPathName(t *testing.T) {
	resourceCtx := &pathNameCapturingCreator{}
	dm := createTestManager()
	dm.resourceCtx = resourceCtx

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("b4 background alt-fs body"))
	}))
	defer server.Close()

	job := &DownloadJob{
		ID:     "pathname-forward",
		URL:    server.URL + "/file.bin",
		Status: JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{
			PathName: "archive",
		},
		ctx: context.Background(),
	}

	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress returned error: %v", err)
	}

	if got := resourceCtx.captured().PathName; got != "archive" {
		t.Errorf("PathName reached AddResource as %q, want %q: the queued download ignored the chosen storage location", got, "archive")
	}
}
