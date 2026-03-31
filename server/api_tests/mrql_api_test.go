package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/application_context"
	"mahresources/models"
)

// setupMRQLTest creates a test environment with MaxOpenConns(1) for SQLite
// in-memory DB sharing, and seeds basic test data.
func setupMRQLTest(t *testing.T) *TestContext {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	return tc
}

// seedMRQLData creates test entities for MRQL query tests.
func seedMRQLData(t *testing.T, tc *TestContext) {
	t.Helper()

	tc.DB.Create(&models.Tag{Name: "testTag"})
	tc.DB.Create(&models.Resource{Name: "testResource", ContentType: "text/plain"})
	tc.DB.Create(&models.Note{Name: "testNote"})
	tc.DB.Create(&models.Group{Name: "testGroup"})

	// Link the tag to the resource
	tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (1, 1)")
}

// ---- Execute endpoint (POST /v1/mrql) ----

func TestMRQLExecuteResourceQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" AND name ~ "testResource"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "resource", result.EntityType)
	assert.Len(t, result.Resources, 1)
	assert.Equal(t, "testResource", result.Resources[0].Name)
	assert.Empty(t, result.Notes)
	assert.Empty(t, result.Groups)
}

func TestMRQLExecuteNoteQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "note" AND name ~ "testNote"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "note", result.EntityType)
	assert.Len(t, result.Notes, 1)
	assert.Equal(t, "testNote", result.Notes[0].Name)
	assert.Empty(t, result.Resources)
	assert.Empty(t, result.Groups)
}

func TestMRQLExecuteGroupQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "group" AND name ~ "testGroup"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "group", result.EntityType)
	assert.Len(t, result.Groups, 1)
	assert.Equal(t, "testGroup", result.Groups[0].Name)
	assert.Empty(t, result.Resources)
	assert.Empty(t, result.Notes)
}

func TestMRQLExecuteEmptyQuery(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": "",
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "required")
}

func TestMRQLExecuteInvalidSyntax(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `name = = "bad"`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.NotEmpty(t, errResp["error"])
}

func TestMRQLExecuteInvalidField(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" AND nonexistentField = "x"`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.NotEmpty(t, errResp["error"])
}

func TestMRQLExecuteCrossEntityQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	// No type specified -- should fan out to all entity types
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `name ~ "test*"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "all", result.EntityType)

	// Should find entities across all types
	totalFound := len(result.Resources) + len(result.Notes) + len(result.Groups)
	assert.GreaterOrEqual(t, totalFound, 3, "cross-entity query should find resources, notes, and groups")
}

func TestMRQLExecuteWithLimit(t *testing.T) {
	tc := setupMRQLTest(t)

	// Create multiple resources
	for i := 1; i <= 5; i++ {
		tc.DB.Create(&models.Resource{
			Name:        fmt.Sprintf("limitTestRes%d", i),
			ContentType: "text/plain",
		})
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" AND name ~ "limitTestRes*" LIMIT 2`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Len(t, result.Resources, 2, "LIMIT 2 should return exactly 2 results")
}

func TestMRQLExecuteWithOrderBy(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "alpha_order", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "charlie_order", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "bravo_order", ContentType: "text/plain"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" AND name ~ "*_order" ORDER BY name ASC`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Resources), 3)
	assert.Equal(t, "alpha_order", result.Resources[0].Name)
	assert.Equal(t, "bravo_order", result.Resources[1].Name)
	assert.Equal(t, "charlie_order", result.Resources[2].Name)
}

// ---- Validate endpoint (POST /v1/mrql/validate) ----

func TestMRQLValidateValidQuery(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query": `type = "resource" AND name ~ "test"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	assert.Equal(t, true, result["valid"])
}

func TestMRQLValidateInvalidSyntax(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query": `name = = "bad"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	assert.Equal(t, false, result["valid"])

	errors, ok := result["errors"].([]any)
	assert.True(t, ok, "errors should be an array")
	assert.Greater(t, len(errors), 0, "should have at least one error")

	firstErr := errors[0].(map[string]any)
	assert.NotEmpty(t, firstErr["message"])
	// pos should be present as a number
	_, hasPos := firstErr["pos"]
	assert.True(t, hasPos, "error should include position info")
}

func TestMRQLValidateInvalidField(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query": `type = "resource" AND fakeField = "x"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	assert.Equal(t, false, result["valid"])

	errors, ok := result["errors"].([]any)
	assert.True(t, ok, "errors should be an array")
	assert.Greater(t, len(errors), 0)
}

// ---- Complete endpoint (POST /v1/mrql/complete) ----

func TestMRQLCompleteEmptyQuery(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{
		"query":  "",
		"cursor": 0,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	suggestions, ok := result["suggestions"].([]any)
	assert.True(t, ok, "suggestions should be an array")
	assert.Greater(t, len(suggestions), 0, "empty query should return field suggestions")

	// Verify suggestions have the expected shape
	first := suggestions[0].(map[string]any)
	assert.NotEmpty(t, first["value"])
	assert.NotEmpty(t, first["type"])
}

func TestMRQLCompleteAfterField(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{
		"query":  "name ",
		"cursor": 5,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	suggestions, ok := result["suggestions"].([]any)
	assert.True(t, ok, "suggestions should be an array")
	assert.Greater(t, len(suggestions), 0, "after field name should return operator suggestions")

	// Check that at least one suggestion is an operator
	foundOperator := false
	for _, s := range suggestions {
		sm := s.(map[string]any)
		if sm["type"] == "operator" {
			foundOperator = true
			break
		}
	}
	assert.True(t, foundOperator, "should suggest operators after a field name")
}

func TestMRQLCompleteTypeValue(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{
		"query":  `type = `,
		"cursor": 7,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	suggestions, ok := result["suggestions"].([]any)
	assert.True(t, ok, "suggestions should be an array")
	assert.Greater(t, len(suggestions), 0, "after 'type =' should return entity type suggestions")

	// Should include entity types
	values := make([]string, 0, len(suggestions))
	for _, s := range suggestions {
		sm := s.(map[string]any)
		values = append(values, sm["value"].(string))
	}
	assert.Contains(t, values, "resource", "should suggest resource as entity type")
}

// ---- Saved queries CRUD ----

func TestMRQLSavedQueryCreate(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/saved", map[string]any{
		"name":        "My Saved Query",
		"query":       `type = "resource" AND name ~ "test*"`,
		"description": "A test saved query",
	})
	assert.Equal(t, http.StatusCreated, resp.Code)

	var saved models.SavedMRQLQuery
	err := json.Unmarshal(resp.Body.Bytes(), &saved)
	assert.NoError(t, err)
	assert.NotZero(t, saved.ID)
	assert.Equal(t, "My Saved Query", saved.Name)
	assert.Equal(t, `type = "resource" AND name ~ "test*"`, saved.Query)
	assert.Equal(t, "A test saved query", saved.Description)
}

func TestMRQLSavedQueryList(t *testing.T) {
	tc := setupMRQLTest(t)

	// Create two saved queries directly
	tc.DB.Create(&models.SavedMRQLQuery{Name: "Query A", Query: `name ~ "a"`})
	tc.DB.Create(&models.SavedMRQLQuery{Name: "Query B", Query: `name ~ "b"`})

	resp := tc.MakeRequest(http.MethodGet, "/v1/mrql/saved", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var queries []models.SavedMRQLQuery
	err := json.Unmarshal(resp.Body.Bytes(), &queries)
	assert.NoError(t, err)
	assert.Len(t, queries, 2)

	// Should be ordered by name ASC
	assert.Equal(t, "Query A", queries[0].Name)
	assert.Equal(t, "Query B", queries[1].Name)
}

func TestMRQLSavedQueryGetByID(t *testing.T) {
	tc := setupMRQLTest(t)

	saved := &models.SavedMRQLQuery{Name: "GetByID Test", Query: `name ~ "test"`}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/mrql/saved?id=%d", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result models.SavedMRQLQuery
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, saved.ID, result.ID)
	assert.Equal(t, "GetByID Test", result.Name)
}

func TestMRQLSavedQueryUpdate(t *testing.T) {
	tc := setupMRQLTest(t)

	saved := &models.SavedMRQLQuery{Name: "Before Update", Query: `name ~ "old"`, Description: "old desc"}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodPut, fmt.Sprintf("/v1/mrql/saved?id=%d", saved.ID), map[string]any{
		"name":        "After Update",
		"query":       `name ~ "new"`,
		"description": "new desc",
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result models.SavedMRQLQuery
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "After Update", result.Name)
	assert.Equal(t, `name ~ "new"`, result.Query)
	assert.Equal(t, "new desc", result.Description)

	// Verify in DB
	var check models.SavedMRQLQuery
	tc.DB.First(&check, saved.ID)
	assert.Equal(t, "After Update", check.Name)
}

func TestMRQLSavedQueryRunByID(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved := &models.SavedMRQLQuery{
		Name:  "Run By ID",
		Query: `type = "resource" AND name = "testResource"`,
	}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "resource", result.EntityType)
	assert.Len(t, result.Resources, 1)
	assert.Equal(t, "testResource", result.Resources[0].Name)
}

func TestMRQLSavedQueryRunByName(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved := &models.SavedMRQLQuery{
		Name:  "Run By Name",
		Query: `type = "note" AND name = "testNote"`,
	}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/saved/run?name=Run+By+Name", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "note", result.EntityType)
	assert.Len(t, result.Notes, 1)
	assert.Equal(t, "testNote", result.Notes[0].Name)
}

func TestMRQLSavedQueryDelete(t *testing.T) {
	tc := setupMRQLTest(t)

	saved := &models.SavedMRQLQuery{Name: "To Delete", Query: `name ~ "x"`}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/delete?id=%d", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var deleteResp map[string]any
	json.Unmarshal(resp.Body.Bytes(), &deleteResp)
	assert.Equal(t, float64(saved.ID), deleteResp["id"])

	// Verify it is gone
	var count int64
	tc.DB.Model(&models.SavedMRQLQuery{}).Where("id = ?", saved.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

// ---- GROUP BY endpoint tests ----

func TestMRQL_GroupByAggregated(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed several resources with varying content types
	tc.DB.Create(&models.Resource{Name: "aggRes1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "aggRes2", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "aggRes3", ContentType: "image/png"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)

	assert.Equal(t, "aggregated", result["mode"])
	assert.Equal(t, "resource", result["entityType"])

	rows, ok := result["rows"].([]any)
	assert.True(t, ok, "expected 'rows' to be an array")
	assert.NotEmpty(t, rows, "expected at least one aggregated row")

	// Verify each row has the group key and count
	for _, r := range rows {
		row := r.(map[string]any)
		assert.Contains(t, row, "contentType", "row should contain the group-by field")
		assert.Contains(t, row, "count", "row should contain count aggregate")
	}
}

func TestMRQL_GroupByBucketed(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed several resources with varying content types
	tc.DB.Create(&models.Resource{Name: "bucketRes1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "bucketRes2", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "bucketRes3", ContentType: "image/png"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType LIMIT 5`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)

	assert.Equal(t, "bucketed", result["mode"])
	assert.Equal(t, "resource", result["entityType"])

	groups, ok := result["groups"].([]any)
	assert.True(t, ok, "expected 'groups' to be an array")
	assert.NotEmpty(t, groups, "expected at least one bucket group")

	// Verify each group has a key and items
	for _, g := range groups {
		group := g.(map[string]any)
		assert.Contains(t, group, "key", "group should contain 'key'")
		assert.Contains(t, group, "items", "group should contain 'items'")
	}
}

func TestMRQL_GroupByWithoutEntityTypeFails(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `name ~ "test" GROUP BY name COUNT()`,
	})
	assert.NotEqual(t, http.StatusOK, resp.Code, "GROUP BY without entity type should fail")

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.NotEmpty(t, errResp["error"])
}

func TestMRQLSavedQueryCreateEmptyName(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/saved", map[string]any{
		"name":  "",
		"query": `name ~ "test"`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "non-empty")
}

func TestMRQLSavedQueryCreateDuplicateName(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.SavedMRQLQuery{Name: "Duplicate", Query: `name ~ "a"`})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/saved", map[string]any{
		"name":  "Duplicate",
		"query": `name ~ "b"`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.NotEmpty(t, errResp["error"])
}

// ---- Saved query GROUP BY paging ----

func TestMRQLSavedQueryRunGroupByInlineLimitAndPage(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed resources with 2 distinct content types
	tc.DB.Create(&models.Resource{Name: "pageSaved1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "pageSaved2", ContentType: "image/png"})
	tc.DB.Create(&models.Resource{Name: "pageSaved3", ContentType: "image/png"})

	// Save a GROUP BY query with inline LIMIT 1
	saved := &models.SavedMRQLQuery{
		Name:  "GroupByPaged",
		Query: `type = "resource" GROUP BY contentType LIMIT 1`,
	}
	tc.DB.Create(saved)

	// Page 1: should return exactly 1 bucket
	resp1 := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&page=1", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp1.Code)

	var result1 map[string]any
	err := json.Unmarshal(resp1.Body.Bytes(), &result1)
	assert.NoError(t, err)
	assert.Equal(t, "bucketed", result1["mode"])

	groups1, _ := result1["groups"].([]any)
	assert.Len(t, groups1, 1, "page 1 should have exactly 1 bucket (inline LIMIT 1 as bucket page size)")

	// Page 2: should return the second bucket (not empty)
	resp2 := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&page=2", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp2.Code)

	var result2 map[string]any
	err = json.Unmarshal(resp2.Body.Bytes(), &result2)
	assert.NoError(t, err)
	assert.Equal(t, "bucketed", result2["mode"])

	groups2, _ := result2["groups"].([]any)
	assert.Len(t, groups2, 1, "page 2 should have exactly 1 bucket")

	// Pages should have different bucket keys
	if len(groups1) > 0 && len(groups2) > 0 {
		key1 := groups1[0].(map[string]any)["key"].(map[string]any)["contentType"]
		key2 := groups2[0].(map[string]any)["key"].(map[string]any)["contentType"]
		assert.NotEqual(t, key1, key2, "page 1 and page 2 should have different bucket keys")
	}
}

func TestMRQLSavedQueryRunGroupByWithBucketsParam(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "bucketParam1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "bucketParam2", ContentType: "image/png"})

	saved := &models.SavedMRQLQuery{
		Name:  "GroupByBucketsParam",
		Query: `type = "resource" GROUP BY contentType`,
	}
	tc.DB.Create(saved)

	// Use explicit buckets=1 param to page through groups
	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&buckets=1&page=1", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "bucketed", result["mode"])

	groups, _ := result["groups"].([]any)
	assert.Len(t, groups, 1, "buckets=1 should return exactly 1 bucket")
}

func TestMRQLSavedQueryRunGroupByRevalidation(t *testing.T) {
	tc := setupMRQLTest(t)

	// Insert a saved query with an invalid field directly (bypassing validation)
	tc.DB.Exec(
		"INSERT INTO saved_mrql_queries (name, query, description) VALUES (?, ?, ?)",
		"InvalidSaved",
		`type = "resource" AND bogusField = "x"`,
		"",
	)

	var inserted models.SavedMRQLQuery
	tc.DB.Where("name = ?", "InvalidSaved").First(&inserted)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d", inserted.ID), nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "running an invalid saved query should return validation error")
}

// ---- Execute endpoint GROUP BY paging ----

func TestMRQLExecuteGroupByInlineLimitAndPage(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "execPage1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "execPage2", ContentType: "image/png"})

	// Page 1 with inline LIMIT 1
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType LIMIT 1`,
		"page":  1,
	})
	assert.Equal(t, http.StatusOK, resp1.Code)

	var result1 map[string]any
	json.Unmarshal(resp1.Body.Bytes(), &result1)
	groups1, _ := result1["groups"].([]any)
	assert.Len(t, groups1, 1, "page 1 with inline LIMIT 1 should return 1 bucket")

	// Page 2
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType LIMIT 1`,
		"page":  2,
	})
	assert.Equal(t, http.StatusOK, resp2.Code)

	var result2 map[string]any
	json.Unmarshal(resp2.Body.Bytes(), &result2)
	groups2, _ := result2["groups"].([]any)
	assert.Len(t, groups2, 1, "page 2 with inline LIMIT 1 should return 1 bucket")

	// Different keys on different pages
	if len(groups1) > 0 && len(groups2) > 0 {
		key1 := groups1[0].(map[string]any)["key"].(map[string]any)["contentType"]
		key2 := groups2[0].(map[string]any)["key"].(map[string]any)["contentType"]
		assert.NotEqual(t, key1, key2, "pages should have different bucket keys")
	}
}

// Bucketed query with page-only (no buckets/limit params) must not show truncation warning.
func TestMRQLExecuteGroupByPageOnlyNoWarning(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "pageOnly1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "pageOnly2", ContentType: "image/png"})

	// Send only page=1, no limit or buckets param
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType`,
		"page":  1,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "bucketed", result["mode"])

	// Should NOT have any warnings — this is a paginated request, not a truncation
	warnings, _ := result["warnings"].([]any)
	assert.Empty(t, warnings, "page-only bucketed query should not produce truncation warnings")
}

// Same test for saved-query path.
func TestMRQLSavedQueryRunGroupByPageOnlyNoWarning(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "savedPageOnly1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "savedPageOnly2", ContentType: "image/png"})

	saved := &models.SavedMRQLQuery{
		Name:  "PageOnlyTest",
		Query: `type = "resource" GROUP BY contentType`,
	}
	tc.DB.Create(saved)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&page=1", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "bucketed", result["mode"])

	warnings, _ := result["warnings"].([]any)
	assert.Empty(t, warnings, "page-only saved-query bucketed request should not produce truncation warnings")
}

// P1: nextOffset must account for truncated buckets — if a bucket was cut short
// by the item cap, the next page should re-include it, not skip past it.
func TestMRQLExecuteGroupByNextOffsetAccountsTruncation(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed 2 content types, each with items
	tc.DB.Create(&models.Resource{Name: "noPg1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "noPg2", ContentType: "image/png"})

	// Request buckets=1 (one bucket per page) with limit=1 (one item per bucket)
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query":   `type = "resource" GROUP BY contentType`,
		"buckets": 1,
		"limit":   1,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)

	// Should have nextOffset and totalGroups in the response
	nextOffset := result["nextOffset"]
	totalGroups := result["totalGroups"]
	assert.NotNil(t, nextOffset, "expected nextOffset in response")
	assert.NotNil(t, totalGroups, "expected totalGroups in response")

	// nextOffset should be 1 (we showed 1 bucket, next starts at 1)
	if nf, ok := nextOffset.(float64); ok {
		assert.Equal(t, float64(1), nf, "nextOffset should be 1 after showing 1 bucket")
	}

	// Use nextOffset for page 2
	if nf, ok := nextOffset.(float64); ok {
		resp2 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
			"query":   `type = "resource" GROUP BY contentType`,
			"buckets": 1,
			"limit":   1,
			"offset":  int(nf),
		})
		assert.Equal(t, http.StatusOK, resp2.Code)

		var result2 map[string]any
		json.Unmarshal(resp2.Body.Bytes(), &result2)

		groups2, _ := result2["groups"].([]any)
		assert.Len(t, groups2, 1, "page 2 should have 1 bucket")

		// The two pages should have different keys
		groups1, _ := result["groups"].([]any)
		if len(groups1) > 0 && len(groups2) > 0 {
			key1 := groups1[0].(map[string]any)["key"].(map[string]any)["contentType"]
			key2 := groups2[0].(map[string]any)["key"].(map[string]any)["contentType"]
			assert.NotEqual(t, key1, key2, "page 1 and page 2 should show different buckets")
		}
	}
}

// Aggregated GROUP BY with a large limit must not have its page size clamped
// by the bucketed item cap. limit=N&page=2 must skip exactly N rows.
func TestMRQLExecuteAggregatedLargeLimitPageNotClamped(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed 3 distinct content types
	tc.DB.Create(&models.Resource{Name: "lp1", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "lp2", ContentType: "image/png"})
	tc.DB.Create(&models.Resource{Name: "lp3", ContentType: "application/pdf"})

	// Page 1: limit=2 returns first 2 aggregated rows
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
		"limit": 2,
		"page":  1,
	})
	assert.Equal(t, http.StatusOK, resp1.Code)

	var result1 map[string]any
	json.Unmarshal(resp1.Body.Bytes(), &result1)
	assert.Equal(t, "aggregated", result1["mode"])
	rows1, _ := result1["rows"].([]any)
	assert.Len(t, rows1, 2, "page 1 with limit=2 should return 2 rows")

	// Page 2: should return the remaining row(s), not overlap with page 1
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
		"limit": 2,
		"page":  2,
	})
	assert.Equal(t, http.StatusOK, resp2.Code)

	var result2 map[string]any
	json.Unmarshal(resp2.Body.Bytes(), &result2)
	rows2, _ := result2["rows"].([]any)
	assert.NotEmpty(t, rows2, "page 2 should have remaining rows")

	// Verify no overlap between pages
	if len(rows1) > 0 && len(rows2) > 0 {
		ct1 := rows1[0].(map[string]any)["contentType"]
		ct2 := rows2[0].(map[string]any)["contentType"]
		assert.NotEqual(t, ct1, ct2, "page 1 and page 2 should not overlap")
	}

	// Now test with a very large limit — the bucketed item cap (10000) must NOT
	// clamp this. With limit=20000&page=2, offset should be 20000, not 10000.
	// Since we only have 3 rows, page 2 should be empty (offset 20000 > 3 rows).
	resp3 := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
		"limit": 20000,
		"page":  2,
	})
	assert.Equal(t, http.StatusOK, resp3.Code)

	var result3 map[string]any
	json.Unmarshal(resp3.Body.Bytes(), &result3)
	rows3, _ := result3["rows"].([]any)
	assert.Empty(t, rows3, "limit=20000&page=2 should skip past all 3 rows (offset=20000)")
}
