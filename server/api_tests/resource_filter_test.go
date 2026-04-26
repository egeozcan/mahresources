//go:build json1 && fts5

package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// TestResourceList_FilterByContentTypes verifies that repeated ContentTypes
// query params are bound and passed through to the database filter, returning
// only resources whose content_type is in the requested set.
func TestResourceList_FilterByContentTypes(t *testing.T) {
	tc := SetupTestEnv(t)

	tc.CreateResourceWithType(t, "a.png", "image/png")
	tc.CreateResourceWithType(t, "b.jpg", "image/jpeg")
	tc.CreateResourceWithType(t, "c.pdf", "application/pdf")

	req, err := http.NewRequest("GET", "/v1/resources?ContentTypes=image%2Fpng&ContentTypes=image%2Fjpeg", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Resource
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	if len(got) != 2 {
		t.Fatalf("expected 2 resources (image/png + image/jpeg), got %d", len(got))
	}
	for _, r := range got {
		if r.ContentType != "image/png" && r.ContentType != "image/jpeg" {
			t.Errorf("unexpected content type in response: %s", r.ContentType)
		}
	}
}
