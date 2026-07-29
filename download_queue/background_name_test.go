package download_queue

import (
	"context"
	"io"
	"testing"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// nameCapturingResourceCreator records the arguments the worker hands to
// AddResource, so a test can assert on the resource name and the stored file
// name independently.
type nameCapturingResourceCreator struct {
	called       bool
	fileName     string
	resourceName string
}

func (c *nameCapturingResourceCreator) AddResource(file contracts.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.Copy(io.Discard, file)
	c.called = true
	c.fileName = fileName
	c.resourceName = q.Name
	return &models.Resource{ID: 1, Name: q.Name}, nil
}

// Finding 1 (high): a background remote download threw away the Name the user
// typed and named the resource after the URL's last path segment, while the
// Description was preserved — so only Name was lost, and only in the background
// path. The foreground path (AddRemoteResource) has always preferred the
// user-supplied Name.
func TestDownloadWorker_KeepsUserSuppliedName(t *testing.T) {
	rc := &nameCapturingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = rc

	server := newContentServer(t)
	defer server.Close()

	job := &DownloadJob{
		ID:     "named",
		URL:    server.URL + "/300",
		Status: JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name:        "MY CHOSEN NAME",
				Description: "description survives today; name does not",
			},
		},
		ctx: context.Background(),
	}

	if _, err := dm.downloadWithProgress(job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if !rc.called {
		t.Fatal("AddResource was not called; the rest of this test is meaningless")
	}
	if rc.resourceName != "MY CHOSEN NAME" {
		t.Errorf("background download named the resource %q, want %q.\n"+
			"The worker built the name from FileName then path.Base(URL) and never consulted creator.Name.",
			rc.resourceName, "MY CHOSEN NAME")
	}
}

// With no Name supplied the worker must still fall back to the URL's last path
// segment — the positive control for the test above.
func TestDownloadWorker_FallsBackToURLSegment(t *testing.T) {
	rc := &nameCapturingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = rc

	server := newContentServer(t)
	defer server.Close()

	job := &DownloadJob{
		ID:      "unnamed",
		URL:     server.URL + "/sunset.jpg",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}

	if _, err := dm.downloadWithProgress(job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if rc.resourceName != "sunset.jpg" {
		t.Errorf("unnamed background download produced %q, want the URL segment %q",
			rc.resourceName, "sunset.jpg")
	}
}
