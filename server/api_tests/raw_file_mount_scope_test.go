package api_tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"mahresources/models"
	"mahresources/server"
)

// The same relative path can name a different file on each storage mount. A
// group-limited principal that can see the main-storage resource at a path must
// not open another owner's file at that path on another mount.
func TestRawFileGuardMatchesTheMount(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "mount-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "mount-outside"}
	tc.DB.Create(outside)

	archiveDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(archiveDir, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "shared", "x.txt"), []byte("archive secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(tc.Fs, "/shared/x.txt", []byte("main visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := "archive"
	tc.DB.Create(&models.Resource{Name: "visible", OwnerId: &root.ID, Location: "shared/x.txt"})
	tc.DB.Create(&models.Resource{Name: "hidden", OwnerId: &outside.ID, Location: "shared/x.txt", StorageLocation: &archive})
	tc.Router = server.CreateServer(tc.AppCtx, tc.Fs, map[string]string{archive: archiveDir}).Handler

	bearer := scopedUserBearer(t, tc, root.ID)
	get := func(path string) (int, string) {
		resp := doReq(tc, http.MethodGet, path, map[string]string{"Authorization": bearer}, nil, nil)
		return resp.Code, resp.Body.String()
	}
	if code, body := get("/files/shared/x.txt"); code != http.StatusOK || body != "main visible" {
		t.Errorf("visible main-storage file = %d %q", code, body)
	}
	if code, body := get("/archive/shared/x.txt"); code != http.StatusNotFound {
		t.Errorf("another owner's file at the same path on another mount = %d %q, want 404", code, body)
	}
}
