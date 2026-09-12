package api_tests

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/server/api_handlers"
	"net/http/httptest"
	"testing"
)

func TestEntityPickerAllCarriersAndDefaults(t *testing.T) {
	env := SetupTestEnv(t)
	name := "A long <identifying> name that must not be truncated at any legacy summary length"
	groupType := models.Category{Name: "Category", CustomEntityPickerResult: `<strong>[property path="Name"]</strong>`, CustomEntityPickerResultCSS: ".group-picker{}"}
	noteType := models.NoteType{Name: "Note type", CustomEntityPickerResult: `<strong>[property path="Name"]</strong>`, CustomEntityPickerResultCSS: ".note-picker{}"}
	resourceType := models.ResourceCategory{Name: "Resource type", CustomEntityPickerResult: `<strong>[property path="Name"]</strong>`, CustomEntityPickerResultCSS: ".resource-picker{}"}
	require.NoError(t, env.DB.Create(&groupType).Error)
	require.NoError(t, env.DB.Create(&noteType).Error)
	require.NoError(t, env.DB.Create(&resourceType).Error)
	require.NoError(t, env.DB.Create(&models.Group{Name: name, CategoryId: &groupType.ID}).Error)
	require.NoError(t, env.DB.Create(&models.Note{Name: name, NoteTypeId: &noteType.ID}).Error)
	require.NoError(t, env.DB.Create(&models.Resource{Name: name, ResourceCategoryId: resourceType.ID}).Error)
	for _, entity := range []string{"group", "note", "resource"} {
		w := httptest.NewRecorder()
		api_handlers.GetEntityPickerHandler(env.AppCtx)(w, httptest.NewRequest("GET", "/v1/entity-picker?entity="+entity, nil))
		require.Equal(t, 200, w.Code, w.Body.String())
		var response api_handlers.EntityPickerResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		require.Equal(t, name, response.Items[0].Value["Name"])
		require.Contains(t, response.Items[0].HTML, "<strong>A long &lt;identifying&gt; name")
		require.Len(t, response.Styles, 1)
		require.Contains(t, response.Styles[0].CSS, entity+"-picker")
	}
	require.NoError(t, env.DB.Model(&groupType).Update("custom_entity_picker_result", "").Error)
	w := httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(env.AppCtx)(w, httptest.NewRequest("GET", "/v1/entity-picker?entity=group", nil))
	var response api_handlers.EntityPickerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Contains(t, response.Items[0].HTML, `rel="noopener noreferrer"`)
	require.Contains(t, response.Items[0].HTML, "Category")
	require.NotContains(t, response.Items[0].HTML, `type="checkbox"`)
}
