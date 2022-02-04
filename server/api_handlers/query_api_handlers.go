package api_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers/interfaces"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
)

func sQLToMap(rows *sqlx.Rows) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("column error: %v", err)
	}

	data := make([]map[string]interface{}, 0)

	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i, _ := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		m := make(map[string]interface{})
		for i, colName := range cols {

			switch columns[i].(type) {
			case []uint8:
				var jsonVal json.RawMessage
				if err := json.Unmarshal(columns[i].([]byte), &jsonVal); err == nil {
					m[colName] = jsonVal
				} else {
					val := columnPointers[i].(*interface{})
					m[colName] = *val
				}
			default:
				val := columnPointers[i].(*interface{})
				m[colName] = *val
			}

		}

		data = append(data, m)
	}

	return data, nil
}

func GetRunQueryHandler(ctx interfaces.QueryRunner) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		name := http_utils.GetQueryParameter(request, "name", "")

		var values query_models.QueryParameters

		if err := tryFillStructValuesFromRequest(&values, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
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
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		resultMap, err := sQLToMap(result)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resultMap)
	}
}

func GetQueryHandler(ctx interfaces.QueryReader) func(writer http.ResponseWriter, request *http.Request) {
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

func GetQueriesHandler(ctx interfaces.QueryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.QueryQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		queries, err := ctx.GetQueries(&query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(queries)
	}
}

func GetAddQueryHandler(ctx interfaces.QueryWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var queryEditor = query_models.QueryEditor{}

		if err := tryFillStructValuesFromRequest(&queryEditor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		var query *models.Query
		var err error

		if queryEditor.ID != 0 {
			query, err = ctx.UpdateQuery(&queryEditor)
		} else {
			query, err = ctx.CreateQuery(&queryEditor.QueryCreator)
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

func GetRemoveQueryHandler(ctx interfaces.QueryDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		if id == 0 {
			http_utils.HandleError(errors.New("query id is needed"), writer, request, http.StatusInternalServerError)
			return
		}

		err := ctx.DeleteQuery(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/queries") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Query{ID: id})
	}
}
