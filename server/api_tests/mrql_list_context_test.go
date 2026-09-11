package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers"
)

// A decorator is a valid handler dependency too. Keep it around the bound
// context so hydration and template shortcodes cannot bypass its capabilities.
type recordingMRQLContext struct {
	api_handlers.MRQLAPIContext
	request  *http.Request
	requests *[]*http.Request
}

func (ctx *recordingMRQLContext) WithRequest(request *http.Request) any {
	bound := ctx.MRQLAPIContext
	if setter, ok := bound.(contracts.RequestContextSetter); ok {
		bound = setter.WithRequest(request).(api_handlers.MRQLAPIContext)
	}
	return &recordingMRQLContext{MRQLAPIContext: bound, request: request, requests: ctx.requests}
}

func (ctx *recordingMRQLContext) GetResources(offset, limit int, query *query_models.ResourceSearchQuery) ([]models.Resource, error) {
	*ctx.requests = append(*ctx.requests, ctx.request)
	return ctx.MRQLAPIContext.GetResources(offset, limit, query)
}

func TestMRQLListRenderingAcceptsDecoratedContext(t *testing.T) {
	tc := SetupTestEnv(t)
	category := models.ResourceCategory{Name: "decorated", CustomSummary: `<span>Nested count: [mrql query='type = resource' value='count']</span>`}
	require.NoError(t, tc.DB.Create(&category).Error)
	owner := models.Group{Name: "decorated-owner"}
	require.NoError(t, tc.DB.Create(&owner).Error)
	resource := models.Resource{Name: "decorated-resource", OwnerId: &owner.ID, ResourceCategoryId: category.ID}
	require.NoError(t, tc.DB.Create(&resource).Error)
	var requests []*http.Request
	ctx := &recordingMRQLContext{MRQLAPIContext: tc.AppCtx, requests: &requests}
	type requestKey struct{}
	request := httptest.NewRequest(http.MethodPost, "/v1/mrql?render=list", bytes.NewBufferString(`{"query":"type = resource LIMIT 1"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), requestKey{}, "preserved"))
	response := httptest.NewRecorder()
	api_handlers.GetExecuteMRQLHandler(ctx)(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Resources, 1)
	require.Contains(t, result.Resources[0].RenderedHTML, "Nested count: 1")
	require.Len(t, requests, 1)
	require.NotNil(t, requests[0])
	require.Equal(t, "preserved", requests[0].Context().Value(requestKey{}))
	_, bounded := requests[0].Context().Deadline()
	require.True(t, bounded, "shared-card hydration must retain the render deadline")
}

func TestMRQLSharedListsAndMassEditRespectPrincipalScope(t *testing.T) {
	tc := setupAuthEnv(t)
	ancestor := models.Group{Name: "SECRET-list-ancestor"}
	require.NoError(t, tc.DB.Create(&ancestor).Error)
	root, outside := models.Group{Name: "list-scope-root", OwnerId: &ancestor.ID}, models.Group{Name: "list-scope-outside"}
	require.NoError(t, tc.DB.Create(&root).Error)
	require.NoError(t, tc.DB.Create(&outside).Error)
	insideGroup := models.Group{Name: "scope-item-group-inside", OwnerId: &root.ID}
	outsideGroup := models.Group{Name: "scope-item-group-outside", OwnerId: &outside.ID}
	insideNote := models.Note{Name: "scope-item-note-inside", OwnerId: &root.ID}
	outsideNote := models.Note{Name: "scope-item-note-outside", OwnerId: &outside.ID}
	insideResource := models.Resource{Name: "scope-item-resource-inside", OwnerId: &root.ID}
	outsideResource := models.Resource{Name: "scope-item-resource-outside", OwnerId: &outside.ID}
	for _, entity := range []any{&insideGroup, &outsideGroup, &insideNote, &outsideNote, &insideResource, &outsideResource} {
		require.NoError(t, tc.DB.Create(entity).Error)
	}
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "list-scoped", Password: "password1", Role: models.RoleUser, ScopeGroupId: &root.ID})
	require.NoError(t, err)
	token, _, err := tc.AppCtx.CreateApiToken(user.ID, "list-scoped", nil)
	require.NoError(t, err)
	headers := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}
	query := `name ~ "scope-item-" ORDER BY name ASC LIMIT 20`
	body, err := json.Marshal(map[string]any{"query": query})
	require.NoError(t, err)
	response := doReq(tc, http.MethodPost, "/v1/mrql?render=list", headers, nil, bytes.NewReader(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Resources, 1)
	require.Len(t, result.Notes, 1)
	require.Len(t, result.Groups, 1)
	require.Equal(t, insideResource.ID, result.Resources[0].ID)
	require.Equal(t, insideNote.ID, result.Notes[0].ID)
	require.Equal(t, insideGroup.ID, result.Groups[0].ID)
	require.NotContains(t, response.Body.String(), "SECRET-list-ancestor")
	require.NotContains(t, response.Body.String(), "scope-item-resource-outside")
	require.NotContains(t, response.Body.String(), "scope-item-note-outside")
	require.NotContains(t, response.Body.String(), "scope-item-group-outside")
	for _, entity := range []string{"resource", "note", "group"} {
		payload := map[string]any{"Target": "mrql", "MRQLQuery": query, "MetaOp": "merge", "Meta": `{"scopeEdited":true}`, "DryRun": true}
		body, err = json.Marshal(payload)
		require.NoError(t, err)
		response = doReq(tc, http.MethodPost, "/v1/"+entity+"s/massEdit", headers, nil, bytes.NewReader(body))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var probe struct{ Matched int }
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &probe))
		require.Equal(t, 1, probe.Matched)
		payload["DryRun"], payload["ExpectedCount"] = false, probe.Matched
		body, err = json.Marshal(payload)
		require.NoError(t, err)
		response = doReq(tc, http.MethodPost, "/v1/"+entity+"s/massEdit", headers, nil, bytes.NewReader(body))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var changed []string
		require.NoError(t, tc.DB.Table(entity+"s").Where("meta LIKE ?", "%scopeEdited%").Pluck("name", &changed).Error)
		require.Equal(t, []string{"scope-item-" + entity + "-inside"}, changed)
	}
}

func TestMRQLRandomListSnapshotKeepsPagesAndMassEditOnOneSample(t *testing.T) {
	tc := SetupTestEnv(t)
	for i := 0; i < 10; i++ {
		require.NoError(t, tc.DB.Create(&models.Resource{Name: fmt.Sprintf("sample-%02d", i)}).Error)
	}
	query := `type = resource ORDER BY RANDOM() LIMIT 4`
	type listResult struct {
		Resources []models.Resource `json:"resources"`
		Snapshot  string            `json:"snapshot"`
	}
	requestPage := func(page, size int, snapshot string) listResult {
		t.Helper()
		response := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": query, "displayPage": page, "displaySize": size, "snapshot": snapshot})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result listResult
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.NotEmpty(t, result.Snapshot)
		if snapshot != "" {
			require.Equal(t, snapshot, result.Snapshot)
		}
		return result
	}
	first := requestPage(1, 2, "")
	second := requestPage(2, 2, first.Snapshot)
	all := requestPage(1, 100, first.Snapshot)
	repeat := requestPage(1, 2, first.Snapshot)
	ids := func(rows []models.Resource) []uint {
		result := make([]uint, len(rows))
		for i, row := range rows {
			result[i] = row.ID
		}
		return result
	}
	require.Len(t, all.Resources, 4)
	require.Equal(t, ids(first.Resources), ids(repeat.Resources))
	require.Equal(t, ids(all.Resources), append(ids(first.Resources), ids(second.Resources)...))
	tag := models.Tag{Name: "sample-edit"}
	require.NoError(t, tc.DB.Create(&tag).Error)
	payload := map[string]any{"Target": "mrql", "MRQLQuery": query, "MRQLSnapshot": first.Snapshot, "TagsOp": "add", "TagIds": []uint{tag.ID}, "DryRun": true}
	response := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var probe struct{ Matched int }
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &probe))
	require.Equal(t, 4, probe.Matched)
	payload["DryRun"], payload["ExpectedCount"] = false, probe.Matched
	response = tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var tagged []uint
	require.NoError(t, tc.DB.Table("resource_tags").Where("tag_id = ?", tag.ID).Pluck("resource_id", &tagged).Error)
	require.ElementsMatch(t, ids(all.Resources), tagged)

	// A missing/expired client snapshot is a request refusal, not a server
	// failure or permission to silently sample a different mutation target.
	payload["MRQLSnapshot"] = "invalid"
	response = tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}
