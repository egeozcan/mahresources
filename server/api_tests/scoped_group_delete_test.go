package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"

	"github.com/stretchr/testify/require"
)

func assertScopedGroupDeleteConflict(t *testing.T, tc *TestContext, prefix string) {
	t.Helper()

	// The Postgres fixture is auth-off by default; enable the same request path
	// used by deployed scoped accounts before minting the bearer below.
	tc.AppCtx.Config.AuthEnabled = true

	scope := &models.Group{Name: prefix + "-inside"}
	require.NoError(t, tc.DB.Create(scope).Error)
	outside := &models.Group{Name: prefix + "-outside-secret"}
	require.NoError(t, tc.DB.Create(outside).Error)

	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username:     prefix + "-user",
		Password:     "password1",
		Role:         models.RoleUser,
		ScopeGroupId: &scope.ID,
	})
	require.NoError(t, err)
	rawToken, _, err := tc.AppCtx.CreateApiToken(user.ID, prefix+"-token", nil)
	require.NoError(t, err)

	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + rawToken,
	}

	// Positive control: this bearer can see its scope root, while confinement
	// already hides the unrelated group.
	before := doReq(tc, http.MethodGet, "/v1/groups", headers, nil, nil)
	require.Equal(t, http.StatusOK, before.Code, before.Body.String())
	require.Contains(t, before.Body.String(), prefix+"-inside")
	require.NotContains(t, before.Body.String(), "outside-secret")

	deleteHeaders := map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + rawToken,
		"Content-Type":  "application/x-www-form-urlencoded",
	}
	deleteResponse := doReq(tc, http.MethodPost, "/v1/group/delete", deleteHeaders, nil,
		strings.NewReader("Id="+itoa(int(scope.ID))))
	require.Equal(t, http.StatusConflict, deleteResponse.Code, deleteResponse.Body.String())

	// A rejected delete must leave the user's scope intact. Otherwise their next
	// request becomes unscoped and exposes unrelated content.
	outsideList := doReq(tc, http.MethodGet, "/v1/groups", headers, nil, nil)
	require.Equal(t, http.StatusOK, outsideList.Code, outsideList.Body.String())
	require.Contains(t, outsideList.Body.String(), prefix+"-inside")
	require.NotContains(t, outsideList.Body.String(), "outside-secret")
}

func TestScopedGroupDeletePreservesScopeConfinement(t *testing.T) {
	assertScopedGroupDeleteConflict(t, setupAuthEnv(t), "sqlite-scope-delete")
}
