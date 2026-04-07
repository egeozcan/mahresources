package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResourceCategory_ValidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{"Name": "Photos", "AutoDetectRules": `{"contentTypes":["image/jpeg"],"priority":5}`}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusOK, resp.Code)
	var result models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, result.AutoDetectRules)
}

func TestCreateResourceCategory_InvalidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "Bad", "AutoDetectRules": `not json`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_MissingContentTypes(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "No CT", "AutoDetectRules": `{"width":{"min":100}}`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_UnknownField(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "Typo", "AutoDetectRules": `{"contentTypes":["image/png"],"contenTypes":["image/png"]}`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateResourceCategory_PreservesAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{Name: "WithRules", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	tc.DB.Create(rc)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"ID": rc.ID, "Description": "Updated"})
	assert.Equal(t, http.StatusOK, resp.Code)
	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, check.AutoDetectRules)
}

func TestUpdateResourceCategory_ClearsAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{Name: "ClearRules", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	tc.DB.Create(rc)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"ID": rc.ID, "AutoDetectRules": ""})
	assert.Equal(t, http.StatusOK, resp.Code)
	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, "", check.AutoDetectRules)
}
