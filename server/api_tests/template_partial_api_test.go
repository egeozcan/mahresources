package api_tests

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplatePartialCreateListGetUpdateDelete(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create
	createResp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial", map[string]any{
		"Name":        "status-badge",
		"Description": "renders a status pill",
		"Content":     `<span>[meta path="status"]</span>`,
	})
	require.Equal(t, http.StatusOK, createResp.Code, createResp.Body.String())

	var created models.TemplatePartial
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	assert.NotZero(t, created.ID)
	assert.Equal(t, "status-badge", created.Name)

	// List
	listResp := tc.MakeRequest(http.MethodGet, "/v1/templatePartials", nil)
	require.Equal(t, http.StatusOK, listResp.Code)
	var list []models.TemplatePartial
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, "status-badge", list[0].Name)

	// Update: change only Content via a partial JSON body; Description preserved.
	updateResp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial", map[string]any{
		"ID":      created.ID,
		"Content": `<b>updated</b>`,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())

	var check models.TemplatePartial
	require.NoError(t, tc.DB.First(&check, created.ID).Error)
	assert.Equal(t, `<b>updated</b>`, check.Content)
	assert.Equal(t, "renders a status pill", check.Description, "description preserved on partial update")
	assert.Equal(t, "status-badge", check.Name, "name preserved on partial update")

	// Delete
	delResp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial/delete?id="+strconv.Itoa(int(created.ID)), nil)
	require.Equal(t, http.StatusOK, delResp.Code)
	var count int64
	tc.DB.Model(&models.TemplatePartial{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestTemplatePartialRejectsNonKebabName(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial", map[string]any{
		"Name":    "Not Kebab",
		"Content": "<b>x</b>",
	})
	assert.NotEqual(t, http.StatusOK, resp.Code)

	var count int64
	tc.DB.Model(&models.TemplatePartial{}).Count(&count)
	assert.Equal(t, int64(0), count, "invalid-name partial must not be created")
}

func TestTemplatePartialUniqueNameConflict(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.DB.Create(&models.TemplatePartial{Name: "dup", Content: "a"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial", map[string]any{
		"Name":    "dup",
		"Content": "b",
	})
	assert.NotEqual(t, http.StatusOK, resp.Code)

	var count int64
	tc.DB.Model(&models.TemplatePartial{}).Where("name = ?", "dup").Count(&count)
	assert.Equal(t, int64(1), count)
}
