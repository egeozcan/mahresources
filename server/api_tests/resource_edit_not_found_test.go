package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models/query_models"
)

func TestResourceEditNonExistentReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	editor := query_models.ResourceEditor{
		ID: 999999,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", editor)
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"editing a non-existent resource should return 404, not 500")
}
