package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/spf13/afero"
)

// POST /v1/resource/local with no PathName is the ordinary case and the only
// one `mr resource from-local` can produce -- the CLI has no --path-name flag,
// so every invocation of it sends the empty string.
//
// AddLocalResource passed &resourceQuery.PathName to GetFsForStorageLocation.
// That pointer is never nil, so the empty key became an alt-fs lookup and the
// request failed on every deployment with
//
//	alt fs '' is not attached
//
// AddResource special-cases the empty string as "the default filesystem"
// (BH-023, application_context/resource_upload_context.go:903); the local path
// never got that branch, and its three unit tests all set a real alt-fs key,
// so the default case was exercised nowhere.
func TestResourceFromLocalUsesDefaultFilesystem(t *testing.T) {
	tc := SetupTestEnv(t)

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	if err != nil {
		t.Fatalf("get default fs: %v", err)
	}
	const localPath = "/staged/from-local-default.txt"
	contents := []byte("staged on the default filesystem")
	if err := afero.WriteFile(fs, localPath, contents, 0o644); err != nil {
		t.Fatalf("stage file: %v", err)
	}

	body := map[string]any{
		"LocalPath": localPath,
		"Name":      "from-local default fs",
		"Meta":      "{}",
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/local", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/resource/local = %d, want 200 (body: %s)", resp.Code, resp.Body.String())
	}

	var created struct {
		ID              uint    `json:"ID"`
		FileSize        int64   `json:"FileSize"`
		StorageLocation *string `json:"StorageLocation"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, resp.Body.String())
	}
	if created.ID == 0 {
		t.Fatal("created resource has no id")
	}
	// The row has to store NULL, not a pointer to "", or every read site
	// calling GetFsForStorageLocation fails the same way the create did.
	if created.StorageLocation != nil {
		t.Errorf("StorageLocation = %q, want null for the default filesystem", *created.StorageLocation)
	}
	if created.FileSize != int64(len(contents)) {
		t.Errorf("FileSize = %d, want %d", created.FileSize, len(contents))
	}

	// Storing NULL only works if the "already exists, skipping" lookup asks
	// for NULL: `storage_location = ''` never matches it, so a second call
	// would create a duplicate row instead of returning the first resource.
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/resource/local", body)
	if resp2.Code != http.StatusOK {
		t.Fatalf("second POST /v1/resource/local = %d, want 200 (body: %s)", resp2.Code, resp2.Body.String())
	}
	var second struct {
		ID uint `json:"ID"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if second.ID != created.ID {
		t.Errorf("second call created resource %d instead of returning the existing %d", second.ID, created.ID)
	}
}

// TestResourceFromLocalDefaultsNameAndMeta covers the failure the previous fix
// exposed rather than caused: with the empty PathName no longer fatal, the
// request the CLI actually sends -- a path and nothing else -- reached
// persistence for the first time, and Meta "" became []byte("") on the model.
// That is an invalid json.RawMessage, so the handler's json.NewEncoder().Encode
// failed after the row had committed and after 200 had gone out: the caller got
// a zero-byte body, and `mr resource from-local --json` printed nothing while
// the resource quietly existed.
func TestResourceFromLocalDefaultsNameAndMeta(t *testing.T) {
	tc := SetupTestEnv(t)

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	if err != nil {
		t.Fatalf("get default fs: %v", err)
	}
	const localPath = "/incoming/handler-defaults.txt"
	if err := afero.WriteFile(fs, localPath, []byte("handler defaults"), 0o644); err != nil {
		t.Fatalf("stage file: %v", err)
	}

	// No Name, no Meta: the documented invocation.
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/local", map[string]any{"LocalPath": localPath})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/resource/local = %d, want 200 (body: %s)", resp.Code, resp.Body.String())
	}
	if resp.Body.Len() == 0 {
		t.Fatal("200 with a zero-byte body: the resource was created but the response could not be encoded")
	}

	var created struct {
		ID           uint            `json:"ID"`
		Name         string          `json:"Name"`
		OriginalName string          `json:"OriginalName"`
		Meta         json.RawMessage `json:"Meta"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, resp.Body.String())
	}
	if created.ID == 0 {
		t.Fatal("created resource has no id")
	}
	if created.Name != "handler-defaults.txt" {
		t.Errorf("Name = %q, want the file's base name", created.Name)
	}
	if created.OriginalName != "handler-defaults.txt" {
		t.Errorf("OriginalName = %q, want the file's base name", created.OriginalName)
	}
	if !json.Valid(created.Meta) {
		t.Errorf("Meta is not valid JSON: %q", string(created.Meta))
	}
}

// TestResourceFromLocalRejectsInvalidMeta pins the validation that comes with
// the default, so the default cannot be reached by skipping the check.
func TestResourceFromLocalRejectsInvalidMeta(t *testing.T) {
	tc := SetupTestEnv(t)

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	if err != nil {
		t.Fatalf("get default fs: %v", err)
	}
	const localPath = "/incoming/handler-bad-meta.txt"
	if err := afero.WriteFile(fs, localPath, []byte("bad meta"), 0o644); err != nil {
		t.Fatalf("stage file: %v", err)
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/local", map[string]any{
		"LocalPath": localPath,
		"Meta":      "{not json",
	})
	if resp.Code == http.StatusOK {
		t.Fatalf("POST with invalid Meta = 200, want 4xx (body: %s)", resp.Body.String())
	}
}
