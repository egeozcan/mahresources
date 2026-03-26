package api_tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteNoteTypeNonExistentReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note/noteType/delete",
		url.Values{"Id": {"999999"}})
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"deleting a non-existent note type should return 404, not 500")
}
