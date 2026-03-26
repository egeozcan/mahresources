package api_tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryRunParseErrorReturns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Send request with no query id/name -- triggers parse or not-found error
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/query/run",
		url.Values{"id": {""}, "Values": {"not-valid-json"}})

	// Should be 400 (bad request) or 404 (query not found), NOT 500
	assert.Less(t, resp.Code, http.StatusInternalServerError,
		"query run with bad input should not return 500")
}
