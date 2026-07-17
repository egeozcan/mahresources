package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
)

func TestRenderedNestedMRQLCannotEscapePrincipalScope(t *testing.T) {
	tc := setupAuthEnv(t)
	root := models.Group{Name: "mrql-scope-root"}
	outside := models.Group{Name: "mrql-scope-outside"}
	require.NoError(t, tc.DB.Create(&root).Error)
	require.NoError(t, tc.DB.Create(&outside).Error)

	category := models.ResourceCategory{
		Name:             "nested-scope-carrier",
		CustomMRQLResult: fmt.Sprintf(`[mrql query='type = "resource" SCOPE %d']`, outside.ID),
	}
	require.NoError(t, tc.DB.Create(&category).Error)
	rootID, outsideID := root.ID, outside.ID
	require.NoError(t, tc.DB.Create(&models.Resource{Name: "visible-outer", OwnerId: &rootID, ResourceCategoryId: category.ID}).Error)
	require.NoError(t, tc.DB.Create(&models.Resource{Name: "secret-outside", OwnerId: &outsideID, ResourceCategoryId: category.ID}).Error)

	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "nested-scoped", Password: "password1", Role: models.RoleUser, ScopeGroupId: &root.ID,
	})
	require.NoError(t, err)
	token, _, err := tc.AppCtx.CreateApiToken(user.ID, "nested", nil)
	require.NoError(t, err)
	headers := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}

	body, _ := json.Marshal(map[string]any{"query": `type = "resource" AND name = "visible-outer" LIMIT 1`})
	rr := doReq(tc, http.MethodPost, "/v1/mrql?render=1", headers, nil, bytes.NewReader(body))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	require.Len(t, result.Resources, 1)
	require.Contains(t, result.Resources[0].RenderedHTML, "scope group not found")
	require.NotContains(t, result.Resources[0].RenderedHTML, "secret-outside")

	// A top-level out-of-subtree SCOPE is ignored in favor of the forced root and
	// must not expose ambiguous/not-found metadata about global groups.
	body, _ = json.Marshal(map[string]any{"query": fmt.Sprintf(`type = "resource" SCOPE %q LIMIT 10`, outside.Name)})
	rr = doReq(tc, http.MethodPost, "/v1/mrql", headers, nil, bytes.NewReader(body))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.False(t, strings.Contains(rr.Body.String(), "categoryId") || strings.Contains(rr.Body.String(), "parentId"), rr.Body.String())
	require.Contains(t, rr.Body.String(), "visible-outer")
	require.NotContains(t, rr.Body.String(), "secret-outside")

	for _, endpoint := range []string{"/v1/mrql/explain", "/v1/mrql/export?format=json"} {
		rr = doReq(tc, http.MethodPost, endpoint, headers, nil, bytes.NewReader(body))
		require.Equal(t, http.StatusOK, rr.Code, "%s: %s", endpoint, rr.Body.String())
		require.False(t, strings.Contains(rr.Body.String(), "categoryId") || strings.Contains(rr.Body.String(), "parentId"), rr.Body.String())
		if strings.Contains(endpoint, "export") {
			require.Contains(t, rr.Body.String(), "visible-outer")
			require.NotContains(t, rr.Body.String(), "secret-outside")
		}
	}
}
