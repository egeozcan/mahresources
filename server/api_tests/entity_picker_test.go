package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/server/api_handlers"
)

func TestEntityPickerAPIEnvelopeAndCustomFallback(t *testing.T) {
	env := SetupTestEnv(t)
	carrier := models.Category{Name: "Picker", CustomCSS: ".shared{}", CustomEntityPickerResultCSS: ".picker{}", CustomEntityPickerResult: `<b>[property path="Name"]</b>`}
	require.NoError(t, env.DB.Create(&carrier).Error)
	for i := 0; i < 51; i++ {
		require.NoError(t, env.DB.Create(&models.Group{Name: "Picker group", CategoryId: &carrier.ID}).Error)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/entity-picker?entity=group", nil)
	w := httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(env.AppCtx)(w, request)
	require.Equal(t, 200, w.Code, w.Body.String())
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope, 5)
	for _, key := range []string{"items", "page", "hasNext", "styles", "warnings"} {
		require.Contains(t, envelope, key)
	}
	var page struct {
		Items []struct {
			Value map[string]any `json:"value"`
			HTML  string         `json:"html"`
		}
		Page     int
		HasNext  bool
		Styles   []map[string]string
		Warnings []string
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 50)
	require.True(t, page.HasNext)
	require.Equal(t, 1, page.Page)
	require.Len(t, page.Styles, 1)
	require.Contains(t, page.Items[0].HTML, "<b>Picker group</b>")
	require.NotContains(t, fmt.Sprint(page.Items[0].Value), "CustomEntityPickerResult")
	require.Contains(t, page.Styles[0]["css"], ".shared{}\n.picker{}")
	require.NoError(t, env.DB.Model(&carrier).Update("custom_entity_picker_result", `[mrql query="not valid mrql"]`).Error)
	w = httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(env.AppCtx)(w, request)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.NotEmpty(t, page.Warnings)
	require.Contains(t, page.Items[0].HTML, `target="_blank"`)
}

func TestEntityPickerAPIScopedResolveAndInvalidInputs(t *testing.T) {
	env := SetupTestEnv(t)
	inside := models.Group{Name: "Inside"}
	outside := models.Group{Name: "Outside"}
	require.NoError(t, env.DB.Create(&inside).Error)
	require.NoError(t, env.DB.Create(&outside).Error)
	principal := &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &inside.ID}
	app := env.AppCtx.WithPrincipal(principal)
	for _, path := range []string{"/v1/entity-picker?entity=group", fmt.Sprintf("/v1/entity-picker/resolve?entity=group&id=%d&id=%d", inside.ID, outside.ID)} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
		if strings.Contains(path, "resolve") {
			api_handlers.GetEntityPickerResolveHandler(app)(w, r)
		} else {
			api_handlers.GetEntityPickerHandler(app)(w, r)
		}
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "Inside")
		require.NotContains(t, w.Body.String(), "Outside")
	}
	for _, query := range []string{"entity=not-real", "entity=group&page=-1", "entity=group&page=nope", "entity=group&filter=Ids%3Dnope"} {
		w := httptest.NewRecorder()
		api_handlers.GetEntityPickerHandler(app)(w, httptest.NewRequest("GET", "/v1/entity-picker?"+query, nil))
		require.Equal(t, 400, w.Code, w.Body.String())
	}
}
