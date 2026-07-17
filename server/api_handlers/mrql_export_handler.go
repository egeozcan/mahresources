package api_handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/mrql"
	"mahresources/server/http_utils"
)

type mrqlExportRequest struct {
	Query   string         `json:"query" schema:"query"`
	ID      uint           `json:"id" schema:"id"`
	Name    string         `json:"name" schema:"name"`
	Format  string         `json:"format" schema:"format"`
	Limit   int            `json:"limit" schema:"limit"`
	Page    int            `json:"page" schema:"page"`
	Buckets int            `json:"buckets" schema:"buckets"`
	Offset  int            `json:"offset" schema:"offset"`
	Params  map[string]any `json:"params" schema:"-"`
}

var exportFilenameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// GetExportMRQLHandler handles GET|POST /v1/mrql/export — stream query results
// as a CSV or JSON download. Same inputs as execute plus format=csv|json.
func GetExportMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlExportRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// format may arrive in the body or as a URL query param (the URL wins so
		// `?format=json` works even alongside a JSON body).
		format := strings.ToLower(strings.TrimSpace(req.Format))
		if q := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("format"))); q != "" {
			format = q
		}
		if format == "" {
			format = "csv"
		}
		if format != "csv" && format != "json" {
			http_utils.HandleError(errors.New("format must be csv or json"), writer, request, http.StatusBadRequest)
			return
		}

		// Resolve query text + a filename base (saved name, or a default).
		queryText := req.Query
		filenameBase := "mrql-export"
		if queryText == "" {
			saved, err := lookupSavedMRQLQuery(ctx, req.ID, req.Name)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}
			queryText = saved.Query
			filenameBase = sanitizeExportFilename(saved.Name)
		}

		parsed, err := mrql.Parse(queryText)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		if err := mrql.BindParams(parsed, collectMRQLParams(request, req.Params)); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if err := mrql.Validate(parsed); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		entityType := mrql.ExtractEntityType(parsed)
		if request.URL.Query().Get("preflight") == "1" {
			if format == "csv" && parsed.GroupBy == nil && entityType == mrql.EntityUnspecified {
				http_utils.HandleError(errors.New("CSV export requires a single entity type (add type = \"resource|note|group\"); use format=json for cross-entity results"), writer, request, http.StatusBadRequest)
				return
			}
			if parsed.GroupBy != nil {
				if entityType == mrql.EntityUnspecified {
					http_utils.HandleError(errors.New("GROUP BY requires an explicit entity type"), writer, request, http.StatusBadRequest)
					return
				}
				clone := *parsed
				applyGroupedPagination(&clone, req.Limit, req.Buckets, req.Page, req.Offset)
				err = ctx.ValidateMRQLGroupedExportBounds(&clone)
			} else {
				err = ctx.ValidateMRQLFlatExportBounds(parsed, req.Limit, req.Page)
			}
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		filename := fmt.Sprintf("%s-%s.%s", filenameBase, time.Now().Format("2006-01-02"), format)

		if parsed.GroupBy != nil {
			exportGrouped(ctx, writer, request, parsed, entityType, format, filename, req)
			return
		}
		exportFlat(ctx, writer, request, parsed, entityType, format, filename, req)
	}
}

// exportFlat runs a non-grouped query and streams it as CSV or JSON.
func exportFlat(ctx *application_context.MahresourcesContext, writer http.ResponseWriter, request *http.Request, parsed *mrql.Query, entityType mrql.EntityType, format, filename string, req mrqlExportRequest) {
	// CSV is per-entity; cross-entity mixes column shapes. Reject before running.
	if format == "csv" && entityType == mrql.EntityUnspecified {
		http_utils.HandleError(errors.New("CSV export requires a single entity type (add type = \"resource|note|group\"); use format=json for cross-entity results"), writer, request, http.StatusBadRequest)
		return
	}

	result, err := ctx.ExecuteMRQLParsedExport(request.Context(), parsed, req.Limit, req.Page)
	if err != nil {
		http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
		return
	}

	if format == "json" {
		writeDownloadHeaders(writer, filename, "application/json")
		setDefaultLimitHeader(writer, result.DefaultLimitApplied, result.AppliedLimit)
		_ = json.NewEncoder(writer).Encode(result)
		return
	}

	writeDownloadHeaders(writer, filename, "text/csv")
	setDefaultLimitHeader(writer, result.DefaultLimitApplied, result.AppliedLimit)
	cw := csv.NewWriter(writer)
	defer cw.Flush()
	_ = cw.Write(flatCSVHeader(entityType))
	switch entityType {
	case mrql.EntityResource:
		for i := range result.Resources {
			_ = cw.Write(resourceCSVRow(&result.Resources[i]))
		}
	case mrql.EntityNote:
		for i := range result.Notes {
			_ = cw.Write(noteCSVRow(&result.Notes[i]))
		}
	case mrql.EntityGroup:
		for i := range result.Groups {
			_ = cw.Write(groupCSVRow(&result.Groups[i]))
		}
	}
}

// exportGrouped runs a GROUP BY query and streams it as CSV or JSON.
func exportGrouped(ctx *application_context.MahresourcesContext, writer http.ResponseWriter, request *http.Request, parsed *mrql.Query, entityType mrql.EntityType, format, filename string, req mrqlExportRequest) {
	if entityType == mrql.EntityUnspecified {
		http_utils.HandleError(errors.New("GROUP BY requires an explicit entity type"), writer, request, http.StatusBadRequest)
		return
	}
	parsed.EntityType = entityType
	applyGroupedPagination(parsed, req.Limit, req.Buckets, req.Page, req.Offset)

	grouped, err := ctx.ExecuteMRQLGroupedExport(request.Context(), parsed)
	if err != nil {
		http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
		return
	}

	if format == "json" {
		writeDownloadHeaders(writer, filename, "application/json")
		setDefaultLimitHeader(writer, grouped.DefaultLimitApplied, grouped.AppliedLimit)
		_ = json.NewEncoder(writer).Encode(grouped)
		return
	}

	writeDownloadHeaders(writer, filename, "text/csv")
	setDefaultLimitHeader(writer, grouped.DefaultLimitApplied, grouped.AppliedLimit)
	cw := csv.NewWriter(writer)
	defer cw.Flush()

	if grouped.Mode == "aggregated" {
		cols := mrql.AggregatedColumns(parsed)
		_ = cw.Write(cols)
		for _, row := range grouped.Rows {
			_ = cw.Write(aggregatedCSVRow(cols, row))
		}
		return
	}

	// Bucketed: bucket-key columns prepended to the flat item columns.
	keyCols := mrql.BucketKeyColumns(parsed)
	header := append(append([]string{}, keyCols...), flatCSVHeader(entityType)...)
	_ = cw.Write(header)
	for i := range grouped.Groups {
		bucket := &grouped.Groups[i]
		keyPrefix := make([]string, len(keyCols))
		for j, k := range keyCols {
			keyPrefix[j] = anyToCSV(bucket.Key[k])
		}
		writeBucketItemsCSV(cw, bucket.Items, keyPrefix)
	}
}

// -- CSV column shapes per entity --

func flatCSVHeader(entityType mrql.EntityType) []string {
	switch entityType {
	case mrql.EntityResource:
		return []string{"id", "name", "description", "content_type", "file_size", "width", "height", "created_at", "updated_at", "owner_id", "category_id", "meta"}
	case mrql.EntityNote:
		return []string{"id", "name", "description", "created_at", "updated_at", "owner_id", "note_type_id", "meta"}
	case mrql.EntityGroup:
		return []string{"id", "name", "description", "created_at", "updated_at", "owner_id", "category_id", "meta"}
	}
	return nil
}

// writeBucketItemsCSV writes the items of one bucket, each row prefixed with the
// bucket's key column values.
func writeBucketItemsCSV(cw *csv.Writer, items any, keyPrefix []string) {
	switch typed := items.(type) {
	case []models.Resource:
		for i := range typed {
			_ = cw.Write(append(cloneRow(keyPrefix), resourceCSVRow(&typed[i])...))
		}
	case []models.Note:
		for i := range typed {
			_ = cw.Write(append(cloneRow(keyPrefix), noteCSVRow(&typed[i])...))
		}
	case []models.Group:
		for i := range typed {
			_ = cw.Write(append(cloneRow(keyPrefix), groupCSVRow(&typed[i])...))
		}
	}
}

func resourceCSVRow(r *models.Resource) []string {
	return []string{
		uintToCSV(r.ID),
		r.Name,
		r.Description,
		r.ContentType,
		strconv.FormatInt(r.FileSize, 10),
		uintToCSV(uint(r.Width)),
		uintToCSV(uint(r.Height)),
		r.CreatedAt.Format(time.RFC3339),
		r.UpdatedAt.Format(time.RFC3339),
		ptrUintToCSV(r.OwnerId),
		uintToCSV(r.ResourceCategoryId),
		metaToCSV(r.Meta),
	}
}

func noteCSVRow(n *models.Note) []string {
	return []string{
		uintToCSV(n.ID),
		n.Name,
		n.Description,
		n.CreatedAt.Format(time.RFC3339),
		n.UpdatedAt.Format(time.RFC3339),
		ptrUintToCSV(n.OwnerId),
		ptrUintToCSV(n.NoteTypeId),
		metaToCSV(n.Meta),
	}
}

func groupCSVRow(g *models.Group) []string {
	return []string{
		uintToCSV(g.ID),
		g.Name,
		g.Description,
		g.CreatedAt.Format(time.RFC3339),
		g.UpdatedAt.Format(time.RFC3339),
		ptrUintToCSV(g.OwnerId),
		ptrUintToCSV(g.CategoryId),
		metaToCSV(g.Meta),
	}
}

func aggregatedCSVRow(cols []string, row map[string]any) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = anyToCSV(row[c])
	}
	return out
}

// -- small formatting helpers --

func cloneRow(prefix []string) []string {
	if len(prefix) == 0 {
		return []string{}
	}
	return append([]string{}, prefix...)
}

func uintToCSV(v uint) string { return strconv.FormatUint(uint64(v), 10) }

func ptrUintToCSV(p *uint) string {
	if p == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*p), 10)
}

func metaToCSV(m types.JSON) string {
	if len(m) == 0 {
		return ""
	}
	return string(m)
}

func anyToCSV(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case *any:
		// GORM's map-scan returns *interface{} for aggregate/group columns.
		if t == nil {
			return ""
		}
		return anyToCSV(*t)
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	case int:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case float64:
		// Render integral floats without a trailing ".0".
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func writeDownloadHeaders(writer http.ResponseWriter, filename, contentType string) {
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
}

func setDefaultLimitHeader(writer http.ResponseWriter, applied bool, limit int) {
	if applied {
		writer.Header().Set("X-MRQL-Default-Limit-Applied", strconv.Itoa(limit))
	}
}

func sanitizeExportFilename(name string) string {
	name = strings.TrimSpace(name)
	name = exportFilenameUnsafe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "mrql-export"
	}
	return name
}
