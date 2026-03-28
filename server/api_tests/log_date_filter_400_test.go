package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Bug 1: Log API returns HTTP 500 for invalid date filters instead of 400.
// All other list handlers (resources, notes, groups, tags) correctly return 400
// for invalid date filters, but the log handler returns 500.

func TestLogEntries_InvalidCreatedBefore_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/v1/logs?CreatedBefore=notadate", nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"GET /v1/logs with invalid CreatedBefore should return 400, not 500")
}

func TestLogEntries_InvalidCreatedAfter_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/v1/logs?CreatedAfter=yesterday", nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"GET /v1/logs with invalid CreatedAfter should return 400, not 500")
}
