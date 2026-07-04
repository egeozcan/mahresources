package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"mahresources/models"
	"mahresources/models/types"

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

// A JSON update using the model's lower-case response shape ("content"/
// "description") must apply, not be silently dropped by the preservation check.
func TestTemplatePartialLowercaseJSONUpdateApplies(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.DB.Create(&models.TemplatePartial{Name: "lc", Description: "orig desc", Content: "orig"})
	var existing models.TemplatePartial
	require.NoError(t, tc.DB.Where("name = ?", "lc").First(&existing).Error)

	resp := tc.MakeRequest(http.MethodPost, "/v1/templatePartial", map[string]any{
		"id":      existing.ID,
		"content": "<b>new</b>",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var check models.TemplatePartial
	require.NoError(t, tc.DB.First(&check, existing.ID).Error)
	assert.Equal(t, "<b>new</b>", check.Content, "lower-case content update must apply")
	assert.Equal(t, "orig desc", check.Description, "unsent description preserved")
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

// A [partial] inside a CustomMRQLResult must expand on the bucketed GROUP BY
// render path (/v1/mrql?render=1), not just the flat path. Regression for the
// grouped renderer missing WithPartialResolver.
func TestTemplatePartialExpandsInBucketedMRQLRender(t *testing.T) {
	tc := SetupTestEnv(t)

	require.NoError(t, tc.DB.Create(&models.TemplatePartial{
		Name:    "mrql-badge",
		Content: `<span class="mrql-tp">BADGE</span>`,
	}).Error)

	cat := &models.Category{Name: "MRQL Cat", CustomMRQLResult: `[partial name="mrql-badge"]`}
	require.NoError(t, tc.DB.Create(cat).Error)

	for i := 0; i < 2; i++ {
		require.NoError(t, tc.DB.Create(&models.Group{
			Name:       fmt.Sprintf("G%d", i),
			CategoryId: &cat.ID,
			Meta:       types.JSON(`{"status":"active"}`),
		}).Error)
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=1", map[string]any{
		"query": fmt.Sprintf("type = group AND category = %d GROUP BY meta.status", cat.ID),
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var out struct {
		Mode   string `json:"mode"`
		Groups []struct {
			Items []map[string]any `json:"items"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Equal(t, "bucketed", out.Mode, resp.Body.String())

	var rendered string
	for _, g := range out.Groups {
		for _, it := range g.Items {
			if h, ok := it["renderedHTML"].(string); ok {
				rendered += h
			}
		}
	}
	assert.Contains(t, rendered, "BADGE", "partial must expand in bucketed MRQL render")
	assert.NotContains(t, rendered, "not found", "partial must not render the not-found comment")
}
