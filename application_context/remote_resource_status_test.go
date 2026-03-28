package application_context

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

// =============================================
// Bug 1: AddRemoteResource saves error pages as resources
//
// After httpClient.Get(url), there is no status code check.
// Non-2xx responses (404, 500) have their HTML error bodies
// saved as resource files. The download_queue correctly checks
// status codes but AddRemoteResource does not.
// =============================================

func TestAddRemoteResource_404_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "remote_404")

	// Set up timeouts so the HTTP client works
	ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
	ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "<html><body>404 Not Found</body></html>")
	}))
	defer server.Close()

	res, err := ctx.AddRemoteResource(&query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "should-not-be-saved",
		},
		URL: server.URL + "/nonexistent",
	})

	if err == nil {
		t.Error("AddRemoteResource should return an error for HTTP 404, but got nil")
	}
	if res != nil {
		t.Errorf("AddRemoteResource should not return a resource for HTTP 404, but got resource ID %d", res.ID)
	}
}

func TestAddRemoteResource_500_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "remote_500")

	ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
	ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "<html><body>500 Internal Server Error</body></html>")
	}))
	defer server.Close()

	res, err := ctx.AddRemoteResource(&query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "should-not-be-saved",
		},
		URL: server.URL + "/error",
	})

	if err == nil {
		t.Error("AddRemoteResource should return an error for HTTP 500, but got nil")
	}
	if res != nil {
		t.Errorf("AddRemoteResource should not return a resource for HTTP 500, but got resource ID %d", res.ID)
	}
}

func TestAddRemoteResource_200_Succeeds(t *testing.T) {
	ctx := createCoverageTestContext(t, "remote_200")

	ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
	ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "valid file content")
	}))
	defer server.Close()

	res, err := ctx.AddRemoteResource(&query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "valid-resource",
		},
		URL: server.URL + "/file.txt",
	})

	if err != nil {
		t.Errorf("AddRemoteResource should succeed for HTTP 200, got error: %v", err)
	}
	if res == nil {
		t.Error("AddRemoteResource should return a resource for HTTP 200, got nil")
	}
}

// =============================================
// Bug 2: Bulk addMeta returns 500 for empty Meta string
//
// ValidateMeta("") returns nil (OK), but the empty string passes
// to json_patch(meta, '') which SQLite rejects with an error.
// The fix is to early-return for empty/whitespace Meta strings.
// =============================================

func TestBulkAddMetaToResources_EmptyMeta_NoError(t *testing.T) {
	ctx := createCoverageTestContext(t, "bulk_meta_res_empty")

	// Create a resource to operate on
	res := &models.Resource{Name: "Test Resource", Meta: []byte(`{"key":"val"}`), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Setup: Create resource: %v", err)
	}

	err := ctx.BulkAddMetaToResources(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{res.ID}},
		Meta:      "",
	})
	if err != nil {
		t.Errorf("BulkAddMetaToResources with empty Meta should not error, got: %v", err)
	}
}

func TestBulkAddMetaToResources_WhitespaceMeta_NoError(t *testing.T) {
	ctx := createCoverageTestContext(t, "bulk_meta_res_ws")

	res := &models.Resource{Name: "Test Resource", Meta: []byte(`{"key":"val"}`), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Setup: Create resource: %v", err)
	}

	err := ctx.BulkAddMetaToResources(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{res.ID}},
		Meta:      "   ",
	})
	if err != nil {
		t.Errorf("BulkAddMetaToResources with whitespace Meta should not error, got: %v", err)
	}
}

func TestBulkAddMetaToNotes_EmptyMeta_NoError(t *testing.T) {
	ctx := createCoverageTestContext(t, "bulk_meta_note_empty")

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Test Note"},
	})
	if err != nil {
		t.Fatalf("Setup: CreateOrUpdateNote: %v", err)
	}

	err = ctx.BulkAddMetaToNotes(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{note.ID}},
		Meta:      "",
	})
	if err != nil {
		t.Errorf("BulkAddMetaToNotes with empty Meta should not error, got: %v", err)
	}
}

func TestBulkAddMetaToGroups_EmptyMeta_NoError(t *testing.T) {
	ctx := createCoverageTestContext(t, "bulk_meta_grp_empty")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Test Group"})
	if err != nil {
		t.Fatalf("Setup: CreateGroup: %v", err)
	}

	err = ctx.BulkAddMetaToGroups(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{group.ID}},
		Meta:      "",
	})
	if err != nil {
		t.Errorf("BulkAddMetaToGroups with empty Meta should not error, got: %v", err)
	}
}
