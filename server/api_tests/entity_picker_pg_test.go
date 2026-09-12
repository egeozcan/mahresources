//go:build postgres

package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/server/api_handlers"
)

func TestEntityPickerPostgresScopedTiedPaginationAndIntersection(t *testing.T) {
	env := SetupPostgresTestEnv(t)
	inside, outside := models.Group{Name: "Inside"}, models.Group{Name: "Outside"}
	require.NoError(t, env.DB.Create(&inside).Error)
	require.NoError(t, env.DB.Create(&outside).Error)
	a, b := models.Category{Name: "A"}, models.Category{Name: "B"}
	require.NoError(t, env.DB.Create(&a).Error)
	require.NoError(t, env.DB.Create(&b).Error)
	// More than a page of invisible identities precedes the visible rows. Scope
	// must run inside identity collection, not filter an already paginated page.
	for i := 0; i < 55; i++ {
		require.NoError(t, env.DB.Create(&models.Group{Name: "Tied", OwnerId: &outside.ID, CategoryId: &a.ID}).Error)
	}
	expected := []uint{}
	for i := 0; i < 51; i++ {
		row := models.Group{Name: "Tied", OwnerId: &inside.ID, CategoryId: &a.ID}
		require.NoError(t, env.DB.Create(&row).Error)
		expected = append(expected, row.ID)
	}
	require.NoError(t, env.DB.Create(&models.Group{Name: "Tied", OwnerId: &inside.ID, CategoryId: &b.ID}).Error)
	principal := &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &inside.ID}
	app := env.AppCtx.WithPrincipal(principal)
	browse := func(filter, constraints string, page int) api_handlers.EntityPickerResponse {
		t.Helper()
		params := url.Values{"entity": {"group"}, "filter": {filter}, "constraints": {constraints}, "page": {fmt.Sprint(page)}}
		r := httptest.NewRequest("GET", "/v1/entity-picker?"+params.Encode(), nil)
		r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
		w := httptest.NewRecorder()
		api_handlers.GetEntityPickerHandler(app)(w, r)
		require.Equal(t, 200, w.Code, w.Body.String())
		var result api_handlers.EntityPickerResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result
	}
	filter := fmt.Sprintf("Name=Tied&Categories=%d&Categories=%d&MaxResults=2&SortBy=name", a.ID, b.ID)
	constraints := fmt.Sprintf("Categories=%d", a.ID)
	first := browse(filter, constraints, 1)
	second := browse(filter, constraints, 2)
	require.Len(t, first.Items, 50)
	require.True(t, first.HasNext)
	require.Len(t, second.Items, 1)
	require.False(t, second.HasNext)
	got := []uint{}
	for _, row := range append(first.Items, second.Items...) {
		got = append(got, uint(row.Value["ID"].(float64)))
	}
	require.Equal(t, expected, got)
	require.Equal(t, first.Items, browse(filter, constraints, 1).Items)
	require.Empty(t, browse(fmt.Sprintf("Categories=%d", b.ID), constraints, 1).Items)
}

func TestEntityPickerPostgresMetadataNoteAssociationAndResolve(t *testing.T) {
	env := SetupPostgresTestEnv(t)
	inside, outside := models.Group{Name: "Inside"}, models.Group{Name: "Outside"}
	require.NoError(t, env.DB.Create(&inside).Error)
	require.NoError(t, env.DB.Create(&outside).Error)
	note := models.Note{Name: "Associated", OwnerId: &inside.ID}
	require.NoError(t, env.DB.Create(&note).Error)
	large := models.Resource{Name: "Large", OwnerId: &inside.ID, Meta: []byte(`{"size":2000}`)}
	small := models.Resource{Name: "Small", OwnerId: &inside.ID, Meta: []byte(`{"size":20}`)}
	hidden := models.Resource{Name: "Hidden", OwnerId: &outside.ID, Meta: []byte(`{"size":2000}`)}
	unrelated := models.Resource{Name: "Unrelated", OwnerId: &inside.ID, Meta: []byte(`{"size":2000}`)}
	for _, row := range []*models.Resource{&large, &small, &hidden, &unrelated} {
		require.NoError(t, env.DB.Create(row).Error)
	}
	require.NoError(t, env.DB.Model(&note).Association("Resources").Append(&large, &small, &hidden))
	principal := &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &inside.ID}
	app := env.AppCtx.WithPrincipal(principal)
	constraints := fmt.Sprintf("Notes=%d", note.ID)
	params := url.Values{"entity": {"resource"}, "filter": {"MetaQuery=size:GT:1000"}, "constraints": {constraints}}
	r := httptest.NewRequest("GET", "/v1/entity-picker?"+params.Encode(), nil)
	r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
	w := httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(app)(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	var page api_handlers.EntityPickerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, float64(large.ID), page.Items[0].Value["ID"])
	params.Del("filter")
	params["id"] = []string{fmt.Sprint(small.ID), fmt.Sprint(hidden.ID), fmt.Sprint(large.ID), fmt.Sprint(unrelated.ID)}
	r = httptest.NewRequest("GET", "/v1/entity-picker/resolve?"+params.Encode(), nil)
	r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
	w = httptest.NewRecorder()
	api_handlers.GetEntityPickerResolveHandler(app)(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	var resolved api_handlers.EntityPickerResolveResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resolved))
	require.Len(t, resolved.Items, 2)
	require.Equal(t, float64(small.ID), resolved.Items[0]["ID"])
	require.Equal(t, float64(large.ID), resolved.Items[1]["ID"])
}
