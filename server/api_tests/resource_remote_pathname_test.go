package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"mahresources/application_context"
)

// B4 at the HTTP layer, which is where it is reachable by a user: the
// create-resource form renders its Storage select only when alt filesystems
// exist, and posts it as PathName on the same form as the URL box. The upload
// branch of that handler threads PathName into its ResourceCreator; the remote
// branch handed the whole ResourceFromRemoteCreator to AddRemoteResource, which
// dropped it. Same form, same field, two different answers.
func TestResourceRemoteHonoursPathName(t *testing.T) {
	altRoot := t.TempDir()

	tc := setupTestEnvWithConfig(t, func(cfg *application_context.MahresourcesConfig) {
		cfg.AltFileSystems = map[string]string{"archive": altRoot}
	})

	// Unique bytes: AddResource returns the *existing* row on a content-hash
	// collision, which would answer with a main-filesystem resource and fail
	// this assertion for an unrelated reason.
	const body = "b4 api-layer body, unique to this test"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, body)
	}))
	defer origin.Close()

	form := url.Values{}
	form.Set("URL", origin.URL+"/remote-file.txt")
	form.Set("Name", "archived remote")
	form.Set("PathName", "archive")
	form.Set("Meta", "{}")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resource/remote", form)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/resource/remote = %d, want 200 (body: %s)", resp.Code, resp.Body.String())
	}

	var created struct {
		ID              uint    `json:"ID"`
		Location        string  `json:"Location"`
		StorageLocation *string `json:"StorageLocation"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, resp.Body.String())
	}
	if created.ID == 0 {
		t.Fatal("created resource has no id")
	}

	if created.StorageLocation == nil {
		t.Fatal("StorageLocation is null: the download ignored PathName and used the main filesystem")
	}
	if *created.StorageLocation != "archive" {
		t.Errorf("StorageLocation = %q, want %q", *created.StorageLocation, "archive")
	}

	// The binding is only half of it. The alt filesystem is a real directory
	// here, so the bytes are checkable on disk.
	onDisk := filepath.Join(altRoot, filepath.FromSlash(created.Location))
	if _, err := os.Stat(onDisk); err != nil {
		t.Errorf("resource is bound to %q but %s is not there: %v", "archive", created.Location, err)
	}
}
