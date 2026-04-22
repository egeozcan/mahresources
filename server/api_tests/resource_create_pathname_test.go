package api_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceCreate_PathNamePersistsStorageLocation verifies that when a
// multipart resource upload includes a PathName field referencing a configured
// alt-fs key, the created resource's StorageLocation is set to that key.
// BH-023 layer 2: ResourceCreator struct must have a PathName field, and
// AddResource must thread it to resource.StorageLocation.
func TestResourceCreate_PathNamePersistsStorageLocation(t *testing.T) {
	tc := SetupTestEnv(t)

	// Configure an alt-fs in the test environment so PathName="archival" is valid.
	// Both the Config map (for PathName validation) and the internal FS map (for actual IO)
	// must be set.
	tc.AppCtx.Config.AltFileSystems = map[string]string{"archival": t.TempDir()}
	altFs := afero.NewMemMapFs()
	tc.AppCtx.RegisterAltFs("archival", altFs)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	part, err := w.CreateFormFile("resource", "bh023.txt")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader([]byte("hello bh023")))
	require.NoError(t, err)

	require.NoError(t, w.WriteField("Name", "bh023-altfs"))
	require.NoError(t, w.WriteField("PathName", "archival"))
	require.NoError(t, w.Close())

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "resource create failed: %s", rr.Body.String())

	var resources []map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resources))
	require.Len(t, resources, 1, "expected exactly one resource in response")

	assert.Equal(t, "archival", resources[0]["StorageLocation"],
		"StorageLocation must be preserved from multipart PathName (BH-023)")
}
