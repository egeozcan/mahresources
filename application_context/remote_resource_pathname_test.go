package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/models"
	"mahresources/models/query_models"
)

// B4: a remote download silently ignores the storage location the caller chose.
//
// ResourceFromRemoteCreator.PathName carries the comment "optional alt-fs key;
// empty = default filesystem", and nothing read it. AddRemoteResource builds a
// ResourceCreator from it field by field and copies every field except that
// one, so the bytes always landed on the main filesystem -- no error, no
// warning, and a resource that looks right in every list.
//
// The create-resource form renders its Storage select only when alt
// filesystems exist, so the control is offered exactly to the deployments that
// have somewhere else to put things, on the same form as the URL box.
func TestAddRemoteResource_HonoursPathName(t *testing.T) {
	ctx := createCoverageTestContext(t, "remote_pathname")

	ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
	ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second

	altFs := afero.NewMemMapFs()
	ctx.altFileSystems["archive"] = altFs
	ctx.Config.AltFileSystems = map[string]string{"archive": "/archive"}

	// Unique bytes: AddResource returns the *existing* row on a content-hash
	// collision, which would hand this test a default-filesystem resource and
	// fail the assertion for an unrelated reason.
	body := "b4 foreground alt-fs body, unique to this test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "archived-remote"},
		URL:               server.URL + "/file.txt",
		PathName:          "archive",
	})
	if err != nil {
		t.Fatalf("AddRemoteResource() error = %v", err)
	}
	if res == nil {
		t.Fatal("AddRemoteResource() returned nil resource")
	}

	if res.StorageLocation == nil {
		t.Fatal("StorageLocation is nil: the download ignored PathName and used the main filesystem")
	}
	if *res.StorageLocation != "archive" {
		t.Errorf("StorageLocation = %q, want %q", *res.StorageLocation, "archive")
	}

	// The binding is only half of it -- the bytes have to be there too.
	exists, existsErr := afero.Exists(altFs, res.Location)
	if existsErr != nil {
		t.Fatalf("stat %q on the alt filesystem: %v", res.Location, existsErr)
	}
	if !exists {
		t.Errorf("resource is bound to %q but %q does not exist on that filesystem", "archive", res.Location)
	}
}

// The other direction: no PathName still means the main filesystem, and an
// unknown key is still refused rather than silently downgraded to it.
func TestAddRemoteResource_PathNameEmptyAndUnknown(t *testing.T) {
	ctx := createCoverageTestContext(t, "remote_pathname_edges")

	ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
	ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second
	ctx.Config.AltFileSystems = map[string]string{"archive": "/archive"}
	ctx.altFileSystems["archive"] = afero.NewMemMapFs()

	serve := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, body)
		}))
	}

	main := serve("b4 edge case: empty PathName goes to the main filesystem")
	defer main.Close()

	res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "default-remote"},
		URL:               main.URL + "/a.txt",
	})
	if err != nil {
		t.Fatalf("AddRemoteResource() with no PathName error = %v", err)
	}
	if res.StorageLocation != nil {
		t.Errorf("StorageLocation = %q, want nil for the main filesystem", *res.StorageLocation)
	}

	unknown := serve("b4 edge case: an unknown key must be refused")
	defer unknown.Close()

	_, err = ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "bad-fs-remote"},
		URL:               unknown.URL + "/b.txt",
		PathName:          "nope",
	})
	if err == nil {
		t.Fatal("AddRemoteResource() accepted an unknown PathName; want an error")
	}
}

// A retry replays the stored payload, so PathName has to survive it or a
// retried download lands somewhere other than the original. This is the half
// that made the queue's copy of the omission worth fixing in the same batch:
// DownloadHistoryPayload decodes the whole submitted creator rather than
// rebuilding it field by field, so once the worker forwards PathName the retry
// carries it for free -- and this test is what keeps that true.
func TestDownloadHistoryPayloadPreservesPathName(t *testing.T) {
	ctx := createCoverageTestContext(t, "history_payload_pathname")

	submitted := &query_models.ResourceFromRemoteCreator{
		URL:      "http://example.invalid/file.bin",
		PathName: "archive",
	}
	encoded, err := json.Marshal(submitted)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	replayed, err := ctx.DownloadHistoryPayload(&models.DownloadHistoryEntry{
		URL:     submitted.URL,
		Payload: encoded,
	})
	if err != nil {
		t.Fatalf("DownloadHistoryPayload() error = %v", err)
	}
	if replayed.PathName != "archive" {
		t.Errorf("PathName after a retry replay = %q, want %q", replayed.PathName, "archive")
	}
}
