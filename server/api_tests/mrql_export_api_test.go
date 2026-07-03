package api_tests

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/application_context"
	"mahresources/models"
)

func parseCSV(t *testing.T, body string) [][]string {
	t.Helper()
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	assert.NoError(t, err)
	return rows
}

func TestMRQLExportFlatCSV(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query":  `type = "resource"`,
		"format": "csv",
	})
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, resp.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, resp.Header().Get("Content-Disposition"), ".csv")

	rows := parseCSV(t, resp.Body.String())
	assert.Equal(t, []string{"id", "name", "description", "content_type", "file_size", "width", "height", "created_at", "updated_at", "owner_id", "category_id", "meta"}, rows[0])
	assert.Len(t, rows, 2) // header + 1 resource
	assert.Equal(t, "testResource", rows[1][1])
	assert.Equal(t, "text/plain", rows[1][3])
	// Default limit was applied → header present.
	assert.NotEmpty(t, resp.Header().Get("X-MRQL-Default-Limit-Applied"))
}

func TestMRQLExportDefaultFormatIsCSV(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query": `type = "note"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Header().Get("Content-Type"), "text/csv")
	rows := parseCSV(t, resp.Body.String())
	assert.Equal(t, []string{"id", "name", "description", "created_at", "updated_at", "owner_id", "note_type_id", "meta"}, rows[0])
}

func TestMRQLExportFlatJSONMatchesExecute(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	query := `type = "resource" AND name ~ "testResource"`

	exec := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": query})
	assert.Equal(t, http.StatusOK, exec.Code)

	exp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=json", map[string]any{"query": query})
	assert.Equal(t, http.StatusOK, exp.Code)
	assert.Contains(t, exp.Header().Get("Content-Disposition"), ".json")

	// Both decode to equivalent MRQLResult bodies.
	var a, b application_context.MRQLResult
	assert.NoError(t, json.Unmarshal(exec.Body.Bytes(), &a))
	assert.NoError(t, json.Unmarshal(exp.Body.Bytes(), &b))
	assert.Equal(t, a.EntityType, b.EntityType)
	assert.Equal(t, len(a.Resources), len(b.Resources))
	assert.Equal(t, a.Resources[0].ID, b.Resources[0].ID)
}

func TestMRQLExportAggregatedCSV(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)
	tc.DB.Create(&models.Resource{Name: "a", ContentType: "image/png"})
	tc.DB.Create(&models.Resource{Name: "b", ContentType: "image/png"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query":  `type = "resource" GROUP BY contentType COUNT()`,
		"format": "csv",
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	rows := parseCSV(t, resp.Body.String())
	assert.Equal(t, []string{"contentType", "count"}, rows[0])
	// One row per distinct contentType; find the image/png count.
	counts := map[string]string{}
	for _, r := range rows[1:] {
		counts[r[0]] = r[1]
	}
	assert.Equal(t, "2", counts["image/png"])
}

func TestMRQLExportBucketedCSV(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query":  `type = "resource" GROUP BY contentType`,
		"format": "csv",
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	rows := parseCSV(t, resp.Body.String())
	// Bucket key column prepended to the flat resource columns.
	assert.Equal(t, "contentType", rows[0][0])
	assert.Equal(t, "id", rows[0][1])
	assert.Equal(t, "name", rows[0][2])
	// The single seeded resource row carries its bucket key.
	assert.GreaterOrEqual(t, len(rows), 2)
	assert.Equal(t, "text/plain", rows[1][0])
}

func TestMRQLExportCrossEntityCSVRejected(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query":  `name ~ "test"`,
		"format": "csv",
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "single entity type")
}

func TestMRQLExportSavedWithParamGET(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Export Saved", `type = "resource" AND name ~ $needle`, "")
	assert.NoError(t, err)

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/mrql/export?id=%d&param.needle=testResource&format=csv", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)
	// Filename derives from the saved query name.
	assert.Contains(t, resp.Header().Get("Content-Disposition"), "Export-Saved")

	rows := parseCSV(t, resp.Body.String())
	assert.Len(t, rows, 2)
	assert.Equal(t, "testResource", rows[1][1])
}

func TestMRQLExportExplicitLimitNoHeader(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/export", map[string]any{
		"query":  `type = "resource" LIMIT 5`,
		"format": "csv",
	})
	assert.Equal(t, http.StatusOK, resp.Code)
	// Explicit LIMIT → no default-limit header.
	assert.Empty(t, resp.Header().Get("X-MRQL-Default-Limit-Applied"))
}
