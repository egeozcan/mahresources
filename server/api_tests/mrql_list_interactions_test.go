package api_tests

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"net/http"
	"testing"
)

func TestMRQLSharedListTemplates(t *testing.T) {
	tc := SetupTestEnv(t)
	for _, path := range []string{"/resources", "/notes", "/groups", "/tags", "/downloads", "/mrql"} {
		t.Run(path, func(t *testing.T) {
			r := massEditFormPost(t, tc, http.MethodGet, path, "text/html", nil)
			require.Equal(t, http.StatusOK, r.Code, r.Body.String())
			require.Contains(t, r.Body.String(), "bulkAction(")
		})
	}
}

func TestMRQLListPaginationAndStandardCards(t *testing.T) {
	tc := SetupTestEnv(t)
	for i := 0; i < 6; i++ {
		tc.CreateResourceWithType(t, fmt.Sprintf("list-page-%02d", i), "image/png")
	}
	var result struct {
		Resources []models.Resource `json:"resources"`
		ListPage  struct {
			Page, Size, Total int
			HasNext           bool
		} `json:"listPage"`
	}
	query := `type = resource AND name ~ "list-page-" ORDER BY name ASC LIMIT 4 OFFSET 1`
	r := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": query, "displaySize": 2, "displayPage": 2})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &result))
	require.Len(t, result.Resources, 2)
	require.Equal(t, "list-page-03", result.Resources[0].Name)
	require.Equal(t, 4, result.ListPage.Total)
	require.False(t, result.ListPage.HasNext)
	require.Contains(t, result.Resources[0].RenderedHTML, "resource-card")
	require.Contains(t, result.Resources[0].RenderedHTML, "selectableItem")
}

func TestMRQLMassEditKeepsMixedQueryBounds(t *testing.T) {
	tc := SetupTestEnv(t)
	r1 := tc.CreateResourceWithType(t, "bounded-a", "text/plain")
	r2 := tc.CreateResourceWithType(t, "bounded-c", "text/plain")
	n := models.Note{Name: "bounded-b"}
	require.NoError(t, tc.DB.Create(&n).Error)
	tag := models.Tag{Name: "mrql-mass-tag"}
	require.NoError(t, tc.DB.Create(&tag).Error)
	payload := map[string]any{"Target": "mrql", "MRQLQuery": `name ~ $name ORDER BY name ASC LIMIT 2`, "MRQLParams": `{"name":"bounded-"}`, "TagsOp": "add", "TagIds": []uint{tag.ID}, "DryRun": true}
	dry := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusOK, dry.Code, dry.Body.String())
	var probe struct{ Matched int }
	require.NoError(t, json.Unmarshal(dry.Body.Bytes(), &probe))
	require.Equal(t, 1, probe.Matched)
	payload["DryRun"] = false
	payload["ExpectedCount"] = 1
	apply := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusOK, apply.Code, apply.Body.String())
	var ids []uint
	require.NoError(t, tc.DB.Table("resource_tags").Where("tag_id = ?", tag.ID).Pluck("resource_id", &ids).Error)
	require.Equal(t, []uint{r1.ID}, ids)
	require.NotContains(t, ids, r2.ID)
	payload["ExpectedCount"] = 2
	conflict := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", payload)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
}

func TestMRQLBucketPagingAndMassEditDeduplicateEntities(t *testing.T) {
	tc := SetupTestEnv(t)
	tagA, tagB := models.Tag{Name: "bucket-a"}, models.Tag{Name: "bucket-b"}
	require.NoError(t, tc.DB.Create(&tagA).Error)
	require.NoError(t, tc.DB.Create(&tagB).Error)
	r1 := tc.CreateResourceWithType(t, "bucket-first", "text/plain")
	r2 := tc.CreateResourceWithType(t, "bucket-second", "text/plain")
	for _, r := range []*models.Resource{r1, r2} {
		require.NoError(t, tc.DB.Model(r).Association("Tags").Append(&tagA, &tagB))
	}
	query := `type = resource GROUP BY tags ORDER BY name ASC LIMIT 1`
	r := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": query, "displaySize": 1})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var first struct {
		Groups []struct {
			Key   map[string]any
			Items []models.Resource
		}
		ListPage struct{ NextOffset *int } `json:"listPage"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &first))
	require.Len(t, first.Groups, 1)
	require.NotNil(t, first.ListPage.NextOffset)
	require.Len(t, first.Groups[0].Items, 1)
	require.Equal(t, r1.ID, first.Groups[0].Items[0].ID)
	r = tc.MakeRequest(http.MethodPost, "/v1/mrql?render=list", map[string]any{"query": query, "displaySize": 1, "displayPage": 2, "displayOffset": *first.ListPage.NextOffset})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var second struct {
		Groups []struct {
			Key   map[string]any
			Items []models.Resource
		}
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &second))
	require.Len(t, second.Groups, 1)
	require.NotEqual(t, first.Groups[0].Key, second.Groups[0].Key)
	require.Equal(t, r1.ID, second.Groups[0].Items[0].ID)
	dry := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{"Target": "mrql", "MRQLQuery": query, "MetaOp": "merge", "Meta": `{"reviewed":true}`, "DryRun": true})
	require.Equal(t, http.StatusOK, dry.Code, dry.Body.String())
	var probe struct{ Matched int }
	require.NoError(t, json.Unmarshal(dry.Body.Bytes(), &probe))
	require.Equal(t, 1, probe.Matched)
}
