package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
)

func decodeExplain(t *testing.T, body []byte) application_context.MRQLExplainResult {
	t.Helper()
	var out application_context.MRQLExplainResult
	assert.NoError(t, json.Unmarshal(body, &out))
	return out
}

func TestMRQLExplainFlatSingleEntity(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" AND fileSize > 1000`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Equal(t, "resource", out.EntityType)
	assert.Len(t, out.Statements, 1)
	st := out.Statements[0]
	assert.Equal(t, "resource", st.Label)
	assert.Contains(t, st.SQL, "resources")
	assert.Contains(t, st.SQL, "file_size")
	assert.Contains(t, st.SQL, "?", "SQL must be parameterized")
	assert.NotEmpty(t, st.Vars)
	assert.Contains(t, st.Interpolated, "1000")
	// No explicit LIMIT → default applied and reported.
	assert.True(t, out.DefaultLimitApplied)
	assert.Greater(t, out.AppliedLimit, 0)
	assert.Contains(t, st.SQL, "LIMIT")
}

func TestMRQLExplainWithParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query":  `type = "resource" AND name ~ $needle`,
		"params": map[string]any{"needle": "sun"},
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, out.Statements[0].Interpolated, "sun")
}

func TestMRQLExplainMissingParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" AND name ~ $needle`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "missing parameter $needle")
}

func TestMRQLExplainRejectsCrossEntitySQLiteRegexLikeExecution(t *testing.T) {
	tc := setupMRQLTest(t)
	for _, endpoint := range []string{"/v1/mrql", "/v1/mrql/explain"} {
		resp := tc.MakeRequest(http.MethodPost, endpoint, map[string]any{"query": `name ~* "benchmark" LIMIT 1`})
		require.Equal(t, http.StatusBadRequest, resp.Code, "%s: %s", endpoint, resp.Body.String())
		assert.Contains(t, resp.Body.String(), "requires PostgreSQL")
	}
}

func TestMRQLExplainCrossEntity(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `name ~ "test"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	// Cross-entity fans out to three labeled statements.
	assert.Len(t, out.Statements, 3)
	labels := []string{out.Statements[0].Label, out.Statements[1].Label, out.Statements[2].Label}
	assert.ElementsMatch(t, []string{"resources", "notes", "groups"}, labels)
}

func TestMRQLExplainAggregatedGroupBy(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, strings.ToUpper(out.Statements[0].SQL), "GROUP BY")
	assert.Contains(t, strings.ToUpper(out.Statements[0].SQL), "COUNT")
}

func TestMRQLExplainBucketedGroupBy(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" GROUP BY contentType`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Equal(t, "bucket keys", out.Statements[0].Label)
	assert.NotEmpty(t, out.Warnings)
}

func TestMRQLExplainSavedQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Explain Me", `type = "note" AND name ~ $needle`, "")
	assert.NoError(t, err)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/explain?id=%d&param.needle=Meeting", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Equal(t, "note", out.EntityType)
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, out.Statements[0].SQL, "notes")
	assert.Contains(t, out.Statements[0].Interpolated, "Meeting")
}

func TestMRQLExplainExplicitLimit(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" LIMIT 5`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.False(t, out.DefaultLimitApplied)
	assert.Equal(t, 5, out.AppliedLimit)
}

func TestMRQLExplainCrossEntityMatchesExecutionPagination(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `name ~ "test" LIMIT 5 OFFSET 7`,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	out := decodeExplain(t, resp.Body.Bytes())
	require.Len(t, out.Statements, 3)
	for _, statement := range out.Statements {
		upper := strings.ToUpper(statement.Interpolated)
		assert.Contains(t, upper, "LIMIT 12", statement.Label)
		assert.NotContains(t, upper, "OFFSET", statement.Label)
	}
	assert.Equal(t, "cross_entity", out.ExecutionShape.Strategy)
	assert.Equal(t, 3, out.ExecutionShape.PlannedStatements)
	assert.Equal(t, 3, out.ExecutionShape.MinimumStatements)
	assert.Equal(t, 3, out.ExecutionShape.MaximumStatements)
}

func TestMRQLExplainReportsBucketFanoutBounds(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" GROUP BY contentType`,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	out := decodeExplain(t, resp.Body.Bytes())
	assert.Equal(t, "bucket_fanout", out.ExecutionShape.Strategy)
	assert.Equal(t, 1, out.ExecutionShape.PlannedStatements)
	assert.Equal(t, 1, out.ExecutionShape.MinimumStatements)
	assert.Equal(t, 201, out.ExecutionShape.MaximumStatements)
	assert.True(t, out.ExecutionShape.DataDependent)
}

func TestMRQLExplainFingerprintRedactsBoundValues(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	fingerprint := func(value string) string {
		resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
			"query":  `type = "resource" AND name ~ $needle LIMIT 5`,
			"params": map[string]any{"needle": value},
		})
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		out := decodeExplain(t, resp.Body.Bytes())
		require.NotEmpty(t, out.QueryFingerprint)
		require.NotContains(t, out.QueryFingerprint, value)
		return out.QueryFingerprint
	}
	assert.Equal(t, fingerprint("secret-one"), fingerprint("secret-two"))
}

func TestMRQLExplainNativePlanCancellationStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		want    int
	}{
		{name: "cancelled", context: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, func() {}
		}, want: http.StatusRequestTimeout},
		{name: "deadline", context: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, want: http.StatusGatewayTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			tc := setupMRQLTest(t)
			body, err := json.Marshal(map[string]any{"query": `type = "resource" SCOPE 1 LIMIT 1`, "nativePlan": true})
			require.NoError(t, err)
			ctx, cancel := test.context()
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "/v1/mrql/explain", bytes.NewReader(body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			tc.Router.ServeHTTP(rr, req)
			require.Equal(t, test.want, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "nativePlan")
		})
	}
}

func TestMRQLExplainNativePlannerFailureIsAtomicServerError(t *testing.T) {
	tc := setupMRQLTest(t)
	require.NoError(t, tc.DB.Migrator().DropTable(&models.Note{}))
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `name ~ "benchmark" LIMIT 1`, "nativePlan": true,
	})
	require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	assert.NotContains(t, resp.Body.String(), "nativePlan")
}

func TestMRQLExplainNativePlanAuthOffImplicitAdmin(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query":      `type = "resource" AND name = "Vacation.jpg"`,
		"nativePlan": true,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	out := decodeExplain(t, resp.Body.Bytes())
	require.Len(t, out.Statements, 1)
	require.NotNil(t, out.Statements[0].NativePlan)
	assert.Equal(t, "sqlite", out.Statements[0].NativePlan.Dialect)
	assert.Equal(t, "query-plan", out.Statements[0].NativePlan.Format)
	assert.NotEmpty(t, out.Statements[0].NativePlan.Plan)
}

func TestMRQLExplainGeneratedSQLRemainsAvailableToAllRoles(t *testing.T) {
	tc := setupAuthEnv(t)
	body, err := json.Marshal(map[string]any{"query": `type = "resource" LIMIT 1`})
	require.NoError(t, err)
	for _, role := range []models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser, models.RoleGuest} {
		t.Run(string(role), func(t *testing.T) {
			rr := doReq(tc, http.MethodPost, "/v1/mrql/explain", map[string]string{
				"Authorization": roleBearer(t, tc, role), "Content-Type": "application/json",
			}, nil, bytes.NewReader(body))
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		})
	}
}

func TestMRQLExplainNativePlanRequiresAdmin(t *testing.T) {
	tc := setupAuthEnv(t)
	body, err := json.Marshal(map[string]any{
		"query":      `type = "resource" LIMIT 1`,
		"nativePlan": true,
	})
	require.NoError(t, err)

	for _, test := range []struct {
		role models.Role
		want int
	}{
		{role: models.RoleAdmin, want: http.StatusOK},
		{role: models.RoleEditor, want: http.StatusForbidden},
		{role: models.RoleUser, want: http.StatusForbidden},
		{role: models.RoleGuest, want: http.StatusForbidden},
	} {
		t.Run(string(test.role), func(t *testing.T) {
			bearer := roleBearer(t, tc, test.role)
			rr := doReq(tc, http.MethodPost, "/v1/mrql/explain", map[string]string{
				"Authorization": bearer,
				"Content-Type":  "application/json",
			}, nil, bytes.NewReader(body))
			require.Equal(t, test.want, rr.Code, rr.Body.String())
		})
	}
}
