package download_queue

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
)

// actorCapturingResourceCreator records whether the worker bound an actor via
// WithActorUserID before calling AddResource.
type actorCapturingResourceCreator struct {
	boundActor  *uint // set by WithActorUserID
	addResource bool
}

func (c *actorCapturingResourceCreator) AddResource(file interfaces.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.Copy(io.Discard, file)
	c.addResource = true
	return &models.Resource{ID: 1, Name: q.Name}, nil
}

func (c *actorCapturingResourceCreator) WithActorUserID(userID uint) ResourceCreator {
	id := userID
	c.boundActor = &id
	return c // return self so AddResource is still observed on this instance
}

func newContentServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded content"))
	}))
}

// The worker must bind the job's submitter as the create actor when the job has
// an owner, so the resource + initial version are stamped with the submitter.
func TestDownloadWorker_BindsSubmitterActor(t *testing.T) {
	rc := &actorCapturingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = rc

	server := newContentServer(t)
	defer server.Close()

	owner := uint(42)
	job := &DownloadJob{
		ID:          "owned",
		URL:         server.URL + "/file.jpg",
		Status:      JobStatusDownloading,
		creator:     &query_models.ResourceFromRemoteCreator{},
		ctx:         context.Background(),
		ownerUserID: &owner,
	}
	if _, err := dm.downloadWithProgress(job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if !rc.addResource {
		t.Fatal("AddResource was not called")
	}
	if rc.boundActor == nil || *rc.boundActor != owner {
		t.Fatalf("worker must bind the submitter actor %d, got %v", owner, rc.boundActor)
	}
}

// With no owner (auth-off super-user), the worker must NOT bind an actor — it
// creates on the base context, where the stamp callback's default actor (root
// under no-auth) applies instead.
func TestDownloadWorker_NoOwnerDoesNotBind(t *testing.T) {
	rc := &actorCapturingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = rc

	server := newContentServer(t)
	defer server.Close()

	job := &DownloadJob{
		ID:      "unowned",
		URL:     server.URL + "/file.jpg",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
		// ownerUserID nil
	}
	if _, err := dm.downloadWithProgress(job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if !rc.addResource {
		t.Fatal("AddResource was not called")
	}
	if rc.boundActor != nil {
		t.Fatalf("worker must not bind an actor for an unowned job, got %v", *rc.boundActor)
	}
}
