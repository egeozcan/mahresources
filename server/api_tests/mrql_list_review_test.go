package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers"
)

type disappearingMRQLContext struct{ api_handlers.MRQLAPIContext }

func (ctx disappearingMRQLContext) GetResources(offset, limit int, query *query_models.ResourceSearchQuery) ([]models.Resource, error) {
	rows, err := ctx.MRQLAPIContext.GetResources(offset, limit, query)
	visible := rows[:0]
	for _, row := range rows {
		if row.Name != "vanishing" {
			visible = append(visible, row)
		}
	}
	rows = visible
	return rows, err
}

func TestMRQLListDropsDisappearingRows(t *testing.T) {
	tc := SetupTestEnv(t)
	for _, name := range []string{"vanishing", "survivor"} {
		tc.CreateResourceWithType(t, name, "text/plain")
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/mrql?render=list", bytes.NewBufferString(`{"query":"type = resource ORDER BY id ASC LIMIT 2"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api_handlers.GetExecuteMRQLHandler(disappearingMRQLContext{tc.AppCtx})(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Resources, 1)
	require.Equal(t, "survivor", result.Resources[0].Name)
}

func TestMRQLListSharesCSSAndHonorsCustomResult(t *testing.T) {
	tc := SetupTestEnv(t)
	category := models.Category{Name: "styled", CustomCSS: ".review-card { color: red }", CustomMRQLResult: `<h4 class="review-card">[name]</h4>`}
	require.NoError(t, tc.DB.Create(&category).Error)
	for i := 0; i < 6; i++ {
		require.NoError(t, tc.DB.Create(&models.Group{Name: "custom-result", CategoryId: &category.ID}).Error)
	}
	response := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": "type = group LIMIT 6"})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	html := ""
	for _, group := range result.Groups {
		require.Contains(t, group.RenderedHTML, `class="review-card"`)
		require.Contains(t, group.RenderedHTML, "selectableItem")
		html += group.RenderedHTML
	}
	require.Equal(t, 1, strings.Count(html, "<style data-mr-custom-css="))
}

func TestMRQLBucketListBoundsCardsWithoutLosingItems(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := models.Tag{Name: "review-bucket"}
	require.NoError(t, tc.DB.Create(&tag).Error)
	for i := 0; i < 7; i++ {
		r := tc.CreateResourceWithType(t, "bucket-card", "text/plain")
		require.NoError(t, tc.DB.Model(r).Association("Tags").Append(&tag))
	}
	seen := map[uint]bool{}
	offset, itemOffset := 0, 0
	for page := 1; page <= 4; page++ {
		response := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": "type = resource GROUP BY tags ORDER BY id ASC LIMIT 7", "displaySize": 2, "displayPage": page, "displayOffset": offset, "displayItemOffset": itemOffset})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result struct {
			Groups   []struct{ Items []models.Resource }
			ListPage struct {
				NextOffset     *int
				NextItemOffset int
				HasNext        bool
			} `json:"listPage"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		count := 0
		for _, group := range result.Groups {
			for _, r := range group.Items {
				require.False(t, seen[r.ID], "repeated item")
				seen[r.ID] = true
				count++
			}
		}
		require.LessOrEqual(t, count, 2, "per page must bound cards, including within a bucket")
		if !result.ListPage.HasNext {
			break
		}
		require.NotNil(t, result.ListPage.NextOffset)
		offset, itemOffset = *result.ListPage.NextOffset, result.ListPage.NextItemOffset
	}
	require.Len(t, seen, 7, "continuations must preserve all authored bucket items")
}

func TestMRQLSavedListUsesSharedRenderer(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.CreateResourceWithType(t, "saved-card", "text/plain")
	saved, err := tc.AppCtx.CreateSavedMRQLQuery("saved-list-review", "type = resource LIMIT 1", "")
	require.NoError(t, err)
	response := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&render=list", saved.ID), map[string]any{"displaySize": 1})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "listPage")
	require.Contains(t, response.Body.String(), "selectableItem")
}

func TestMRQLListBudgetWarningExplainsPageSize(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(config *application_context.MahresourcesConfig) { config.MRQLPageQueryBudget = 1 })
	noteType := models.NoteType{Name: "budget-review", CustomSummary: `[mrql query='type = resource' value='count'] [mrql query='type = note' value='count']`}
	require.NoError(t, tc.DB.Create(&noteType).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "budget-review", NoteTypeId: &noteType.ID}).Error)
	response := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": "type = note LIMIT 1"})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result application_context.MRQLResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Contains(t, strings.Join(result.Warnings, " "), "Choose fewer items per page")
}

func TestMRQLListPagesPreserveMixedQueryOrder(t *testing.T) {
	tc := SetupTestEnv(t)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("paging-parity-%d", i)
		tc.CreateResourceWithType(t, name, "text/plain")
		require.NoError(t, tc.DB.Create(&models.Note{Name: name}).Error)
		require.NoError(t, tc.DB.Create(&models.Group{Name: name}).Error)
	}
	for _, query := range []string{
		`name ~ "paging-parity" ORDER BY name DESC LIMIT 7 OFFSET 1`,
		`name ~ "paging-parity" ORDER BY id DESC LIMIT 7 OFFSET 1`,
		`name ~ "paging-parity" AND ((type = resource AND contentType = "text/plain") OR type = note) LIMIT 7`,
	} {
		t.Run(query, func(t *testing.T) {
			response := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": query})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var original application_context.MRQLResult
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &original))
			var seen application_context.MRQLResult
			for page := 1; ; page++ {
				response = tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": query, "displayPage": page, "displaySize": 2})
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var result struct {
					application_context.MRQLResult
					ListPage struct{ HasNext bool } `json:"listPage"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				seen.Resources = append(seen.Resources, result.Resources...)
				seen.Notes = append(seen.Notes, result.Notes...)
				seen.Groups = append(seen.Groups, result.Groups...)
				if !result.ListPage.HasNext {
					break
				}
				require.Less(t, page, 10)
			}
			require.Equal(t, mrqlResultIdentities(original), mrqlResultIdentities(seen))
		})
	}
}

func mrqlResultIdentities(result application_context.MRQLResult) []application_context.MRQLEntityIdentity {
	var ids []application_context.MRQLEntityIdentity
	for _, r := range result.Resources {
		ids = append(ids, application_context.MRQLEntityIdentity{EntityType: "resource", ID: r.ID})
	}
	for _, n := range result.Notes {
		ids = append(ids, application_context.MRQLEntityIdentity{EntityType: "note", ID: n.ID})
	}
	for _, g := range result.Groups {
		ids = append(ids, application_context.MRQLEntityIdentity{EntityType: "group", ID: g.ID})
	}
	return ids
}

func TestMRQLListPreservesUnicodeBoundedMembership(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.CreateResourceWithType(t, "Ä", "text/plain")
	require.NoError(t, tc.DB.Create(&models.Note{Name: "á"}).Error)
	body := map[string]any{"query": "type != group ORDER BY name ASC LIMIT 1"}
	plain := tc.MakeRequest(http.MethodPost, "/v1/mrql", body)
	cards := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", body)
	require.Equal(t, http.StatusOK, plain.Code, plain.Body.String())
	require.Equal(t, http.StatusOK, cards.Code, cards.Body.String())
	var expected, actual application_context.MRQLResult
	require.NoError(t, json.Unmarshal(plain.Body.Bytes(), &expected))
	require.NoError(t, json.Unmarshal(cards.Body.Bytes(), &actual))
	require.Equal(t, mrqlResultIdentities(expected), mrqlResultIdentities(actual))
}
