package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

func TestResourceView_NoIdNoSearchCriteria_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource so the database is not empty
	res := &models.Resource{Name: "Secret Resource", Meta: []byte(`{}`)}
	tc.DB.Create(res)

	// Bug: GET /v1/resource/view with no ID and no search criteria falls through
	// to GetResources(0, 1, &detailsQuery) which returns the first resource,
	// leaking data that was never requested.
	resp := tc.MakeRequest(http.MethodGet, "/v1/resource/view", nil)

	// Should return 400 (resource ID or search criteria required), not 302 redirect
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"GET /v1/resource/view with no ID or search criteria should return 400, not leak the first resource")
}

func TestResourceView_InvalidId_NoSearchCriteria_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource so the database is not empty
	res := &models.Resource{Name: "Another Secret", Meta: []byte(`{}`)}
	tc.DB.Create(res)

	// Bug: GET /v1/resource/view?id=abc triggers parse error, falls through
	// to empty detailsQuery, and returns the first resource
	resp := tc.MakeRequest(http.MethodGet, "/v1/resource/view?id=abc", nil)

	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"GET /v1/resource/view with invalid id and no search criteria should return 400")
}

// An unparseable id must be rejected even when other search criteria are
// present; otherwise the request silently turns into a name search and
// redirects to an unrelated resource.
func TestResourceView_InvalidId_WithSearchCriteria_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	res := &models.Resource{Name: "Findable Resource", Meta: []byte(`{}`)}
	tc.DB.Create(res)

	resp := tc.MakeRequest(http.MethodGet, "/v1/resource/view?id=abc&name=Findable", nil)

	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"GET /v1/resource/view with an invalid id must be 400 even when a name filter is present")
	assert.Empty(t, resp.Header().Get("Location"), "must not redirect to a resource")
}
