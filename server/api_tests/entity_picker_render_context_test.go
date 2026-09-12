package api_tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
)

func TestEntityPickerCustomRenderingKeepsAuthenticatedScope(t *testing.T) {
	env := setupAuthEnv(t)
	inside, outside := models.Group{Name: "Picker inside"}, models.Group{Name: "Picker outside"}
	require.NoError(t, env.DB.Create(&inside).Error)
	require.NoError(t, env.DB.Create(&outside).Error)
	carrier := models.ResourceCategory{Name: "Picker scoped carrier", CustomEntityPickerResult: fmt.Sprintf(`[mrql query='type = "resource" SCOPE %d']`, outside.ID)}
	require.NoError(t, env.DB.Create(&carrier).Error)
	require.NoError(t, env.DB.Create(&models.Resource{Name: "Visible picker resource", OwnerId: &inside.ID, ResourceCategoryId: carrier.ID}).Error)
	require.NoError(t, env.DB.Create(&models.Resource{Name: "Secret outside picker", OwnerId: &outside.ID, ResourceCategoryId: carrier.ID}).Error)
	user, err := env.AppCtx.CreateUser(&application_context.UserInput{Username: "picker-scoped-user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &inside.ID})
	require.NoError(t, err)
	token, _, err := env.AppCtx.CreateApiToken(user.ID, "picker", nil)
	require.NoError(t, err)
	response := doReq(env, http.MethodGet, "/v1/entity-picker?entity=resource", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "Visible picker resource")
	require.NotContains(t, response.Body.String(), "Secret outside picker")
	require.Contains(t, response.Body.String(), "default content was used")
}

func TestEntityPickerCustomRenderingKeepsRequestQueryBudget(t *testing.T) {
	env := setupTestEnvWithConfig(t, func(config *application_context.MahresourcesConfig) { config.MRQLPageQueryBudget = 1 })
	carrier := models.NoteType{Name: "Picker budget", CustomEntityPickerResult: `[mrql query='type = resource' value='count'] [mrql query='type = note' value='count']`}
	require.NoError(t, env.DB.Create(&carrier).Error)
	require.NoError(t, env.DB.Create(&models.Note{Name: "Budget fallback note", NoteTypeId: &carrier.ID}).Error)
	response := doReq(env, http.MethodGet, "/v1/entity-picker?entity=note", nil, nil, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "Budget fallback note")
	require.Contains(t, response.Body.String(), "default content was used")
}
