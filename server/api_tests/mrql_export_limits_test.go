package api_tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestMRQLExportUsesBoundedLimitPolicy(t *testing.T) {
	tc := SetupTestEnv(t)
	require.NoError(t, tc.DB.Create(&models.Resource{Name: "export-limit"}).Error)

	within := tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=json", map[string]any{
		"query": `type = "resource" LIMIT 10000`,
	})
	require.Equal(t, http.StatusOK, within.Code, within.Body.String())

	over := tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=json", map[string]any{
		"query": `type = "resource" LIMIT 10001`,
	})
	require.Equal(t, http.StatusBadRequest, over.Code)
	require.Contains(t, over.Body.String(), "exceeds maximum")

	preflight := tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=json&preflight=1", map[string]any{
		"query": `type = "resource" LIMIT 10001`,
	})
	require.Equal(t, http.StatusBadRequest, preflight.Code)
	require.Contains(t, preflight.Body.String(), "exceeds maximum")

	preflight = tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=json&preflight=1", map[string]any{
		"query": `type = "resource" LIMIT 10`,
	})
	require.Equal(t, http.StatusNoContent, preflight.Code, preflight.Body.String())

	form := tc.MakeFormRequest(http.MethodPost, "/v1/mrql/export?format=json", url.Values{
		"query":        {`type = "resource" AND name = $target`},
		"param.target": {"export-limit"},
	})
	require.Equal(t, http.StatusOK, form.Code, form.Body.String())
	require.Contains(t, form.Body.String(), "export-limit")

}
