package api_handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

// SQLToResultSet converts a raw-SQL result set into the columns-and-rows body
// POST /v1/query/run returns. See contracts.SQLResultSet for why the shape is an
// array of arrays and not an array of objects (finding 147).
//
// Exported because the table-block endpoint and the public share server both run
// saved queries and must present a cell the same way POST /v1/query/run does;
// the share server used to do its own []byte-to-string conversion and disagreed.
func SQLToResultSet(rows *sqlx.Rows) (*contracts.SQLResultSet, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("column error: %v", err)
	}

	// The declared database type of each column, which is what decides how a
	// value is represented (see cellValue). ColumnTypes is advisory: a driver
	// that does not implement it leaves the names empty, and an empty name means
	// "no declared type", which is the same answer go-sqlite3 gives for every
	// expression.
	dbTypes := make([]string, len(cols))
	if colTypes, err := rows.ColumnTypes(); err == nil {
		for i, ct := range colTypes {
			if i < len(dbTypes) && ct != nil {
				dbTypes[i] = ct.DatabaseTypeName()
			}
		}
	}

	out := contracts.NewSQLResultSet(cols)

	for rows.Next() {
		columnPointers := make([]any, len(cols))
		scanned := make([]any, len(cols))
		for i := range scanned {
			columnPointers[i] = &scanned[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		values := make([]any, len(cols))
		for i := range cols {
			values[i] = cellValue(*(columnPointers[i].(*any)), dbTypes[i])
		}

		out.Rows = append(out.Rows, values)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	return out, nil
}

// cellValue decides how one scanned column value is represented in JSON, from
// the column's **declared database type** rather than from the bytes.
//
// Two facts make the decision necessary at all. Whether a driver hands a value
// back as a []byte or as a typed Go value is a property of the driver, not of the
// data: go-sqlite3 returns TEXT as a string and BLOB as []byte, and lib/pq — the
// driver behind the production read-only handle on Postgres — returns numeric,
// uuid, arrays, json, jsonb and bytea as []byte while returning text, int, bool
// and timestamps as typed values. And encoding/json base64s any []byte, so
// `SELECT sum(file_size)` on Postgres once answered `"MS41"` for the number the
// query existed to read.
//
// Deciding from the bytes instead — "it parses as a JSON object, so it is one" —
// made a value's JSON type depend on what it happened to spell. Measured on
// SQLite, one document written two ways:
//
//	SELECT json_object('a', 1)                -> "{\"a\":1}"  (a JSON string)
//	SELECT CAST(json_object('a', 1) AS BLOB)  -> {"a":1}      (a JSON object)
//
// and on Postgres a bytea holding the bytes 7b 22 61 22 3a 31 7d became an
// object, while `'123'::jsonb` — a column whose declared purpose is to hold a
// JSON document — came back as the string "123".
//
// So:
//
//   - A column declared json/jsonb (Postgres) or JSON (the SQLite decltype GORM
//     writes for models.Group.Meta and friends) holds a JSON *document*, and a
//     document is allowed to be a scalar. Its value is parsed, whether the driver
//     handed it over as bytes or as a string, so both drivers answer alike.
//     Content that does not parse falls through to the text case rather than
//     failing the query — a legacy row is still readable.
//   - Everything else that is a []byte is the driver's *text representation* of
//     the value (numeric, uuid, arrays) and is emitted as the string it spells.
//   - Bytes that are not valid UTF-8 have no text form, so they keep the base64
//     encoding. That is what a bytea column of real binary gets, and it is the
//     only honest answer for it.
//
// What this deliberately does not do is give SQLite *expressions* a type: SQLite
// has no declared type for a computed column, so `SELECT json_group_array(name)`
// stays a string there while `SELECT json_agg(name)` is structure on Postgres.
// That difference is in SQLite's type system, not in this function, and inventing
// a type by sniffing is the defect this replaced.
func cellValue(val any, dbType string) any {
	if isJSONColumnType(dbType) {
		if parsed, ok := jsonDocument(val); ok {
			return parsed
		}
	}
	raw, ok := val.([]uint8)
	if !ok {
		return val
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	return raw
}

// isJSONColumnType reports whether a driver's declared type name says the column
// holds a JSON document. lib/pq reports JSON and JSONB; go-sqlite3 reports the
// decltype, which is JSON for the columns GORM creates from datatypes.JSON.
func isJSONColumnType(dbType string) bool {
	return strings.EqualFold(dbType, "json") || strings.EqualFold(dbType, "jsonb")
}

// jsonDocument re-emits an already-valid JSON document verbatim. json.RawMessage
// round-trips the exact bytes, so a number too large for a float64 keeps its
// digits instead of being re-rendered through one.
func jsonDocument(val any) (json.RawMessage, bool) {
	var raw []byte
	switch v := val.(type) {
	case []uint8:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return nil, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	var out json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

func GetDatabaseSchemaHandler(ctx contracts.SchemaReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		schema, err := ctx.GetDatabaseSchema()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.Header().Set("Cache-Control", "max-age=300")
		_ = json.NewEncoder(writer).Encode(schema)
	}
}

// fillQueryParameters populates a map[string]any from the request body or
// query string.  Unlike tryFillStructValuesFromRequest, it does not use
// gorilla/schema (which requires a struct pointer) and instead parses form
// values directly into the map.
func fillQueryParameters(dst *query_models.QueryParameters, request *http.Request) error {
	contentTypeHeader := request.Header.Get("Content-type")

	if strings.HasPrefix(contentTypeHeader, constants.JSON) {
		return json.NewDecoder(request.Body).Decode(dst)
	}

	// For form-encoded, multipart, or no content-type: parse form values into the map.
	var formValues url.Values
	// stripRouteKeys controls whether "id" and "name" are removed from the
	// parameter map.  For form-encoded / multipart requests the query lookup
	// keys (id, name) come from the URL query string (parsed separately by
	// the handler), so the POST body values are genuine SQL bind parameters
	// and must be preserved.  For the no-content-type fallback path, the URL
	// query string doubles as both lookup and parameter source, so we strip
	// the routing keys to avoid passing them to the SQL query.
	var stripRouteKeys bool

	if strings.HasPrefix(contentTypeHeader, constants.MultiPartForm) {
		if err := request.ParseMultipartForm(int64(32) << 20); err != nil {
			return err
		}
		formValues = request.PostForm
	} else if strings.HasPrefix(contentTypeHeader, constants.UrlEncodedForm) {
		if err := request.ParseForm(); err != nil {
			return err
		}
		formValues = request.PostForm
	} else {
		formValues = request.URL.Query()
		stripRouteKeys = true
	}

	params := make(query_models.QueryParameters, len(formValues))
	for key, vals := range formValues {
		// In the URL-query-string fallback path, skip the routing parameters
		// (id/name) that are used to identify which saved query to run.
		if stripRouteKeys && (key == "id" || key == "name") {
			continue
		}
		if len(vals) == 1 {
			params[key] = vals[0]
		} else {
			params[key] = strings.Join(vals, ",")
		}
	}
	*dst = params
	return nil
}

func GetRunQueryHandler(ctx contracts.QueryRunner) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		name := http_utils.GetQueryParameter(request, "name", "")

		var values query_models.QueryParameters

		if err := fillQueryParameters(&values, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		var result *sqlx.Rows
		var err error

		if id != 0 {
			result, err = ctx.RunReadOnlyQuery(id, values)
		} else {
			result, err = ctx.RunReadOnlyQueryByName(name, values)
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		defer result.Close()

		resultSet, err := SQLToResultSet(result)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resultSet)
	}
}

func GetQueryHandler(ctx contracts.QueryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		query, err := ctx.GetQuery(id)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(query)
	}
}

func GetQueriesHandler(ctx contracts.QueryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.QueryQuery
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		queries, err := ctx.GetQueries(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(queries)
	}
}

func GetAddQueryHandler(ctx contracts.QueryWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(contracts.QueryWriter)

		var queryEditor = query_models.QueryEditor{}

		if err := tryFillStructValuesFromRequest(&queryEditor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		var query *models.Query
		var err error

		if queryEditor.ID != 0 {
			query, err = effectiveCtx.UpdateQuery(&queryEditor)
		} else {
			query, err = effectiveCtx.CreateQuery(&queryEditor.QueryCreator)
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/query?id="+strconv.Itoa(int(query.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(query)
	}
}

func GetRemoveQueryHandler(ctx contracts.QueryDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(contracts.QueryDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(errors.New("query id is needed"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteQuery(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/queries") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}
