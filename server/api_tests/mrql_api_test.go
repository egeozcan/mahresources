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
