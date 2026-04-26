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

// createResourceWithType inserts a Resource with the given name and content type
// directly into the test database. It is intentionally minimal — no file bytes
// are stored — because the content-type filter operates purely on the DB column.
func createResourceWithType(t *testing.T, tc *TestContext, name, contentType string) *models.Resource {
	t.Helper()
	r := &models.Resource{Name: name, ContentType: contentType}
	require.NoError(t, tc.DB.Create(r).Error)
	return r
}

// TestResourceList_FilterByContentTypes verifies that repeated ContentTypes
// query params are bound and passed through to the database filter, returning
// only resources whose content_type is in the requested set.
func TestResourceList_FilterByContentTypes(t *testing.T) {
	tc := SetupTestEnv(t)

	createResourceWithType(t, tc, "a.png", "image/png")
	createResourceWithType(t, tc, "b.jpg", "image/jpeg")
	createResourceWithType(t, tc, "c.pdf", "application/pdf")

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
}
