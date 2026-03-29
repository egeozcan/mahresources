package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/server/http_utils"
)

// -- Request/response types for MRQL endpoints --

type mrqlExecuteRequest struct {
	Query string `json:"query" schema:"query"`
	Limit int    `json:"limit" schema:"limit"`
	Page  int    `json:"page" schema:"page"`
}

type mrqlValidateRequest struct {
	Query string `json:"query" schema:"query"`
}

type mrqlCompleteRequest struct {
	Query  string `json:"query" schema:"query"`
	Cursor int    `json:"cursor" schema:"cursor"`
}

type mrqlValidateResponse struct {
	Valid  bool             `json:"valid"`
	Errors []map[string]any `json:"errors,omitempty"`
}

type mrqlCompleteResponse struct {
	Suggestions any `json:"suggestions"`
}

type mrqlSavedQueryRequest struct {
	Name        string `json:"name" schema:"name"`
	Query       string `json:"query" schema:"query"`
	Description string `json:"description" schema:"description"`
}

// GetExecuteMRQLHandler handles POST /v1/mrql — execute an MRQL query.
func GetExecuteMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlExecuteRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if req.Query == "" {
			http_utils.HandleError(errors.New("query is required"), writer, request, http.StatusBadRequest)
			return
		}

		result, err := ctx.ExecuteMRQL(req.Query, req.Limit, req.Page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

// GetValidateMRQLHandler handles POST /v1/mrql/validate — validate MRQL syntax.
func GetValidateMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlValidateRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		valid, errs := ctx.ValidateMRQL(req.Query)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlValidateResponse{
			Valid:  valid,
			Errors: errs,
		})
	}
}

// GetCompleteMRQLHandler handles POST /v1/mrql/complete — get autocompletion suggestions.
func GetCompleteMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlCompleteRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		suggestions := ctx.CompleteMRQL(req.Query, req.Cursor)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlCompleteResponse{
			Suggestions: suggestions,
		})
	}
}

// GetSavedMRQLQueriesHandler handles GET /v1/mrql/saved — list all saved MRQL queries,
// or GET /v1/mrql/saved?id=N — get a single saved query.
func GetSavedMRQLQueriesHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		if id != 0 {
			query, err := ctx.GetSavedMRQLQuery(id)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
				return
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(query)
			return
		}

		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		queries, err := ctx.GetSavedMRQLQueries(int(offset), constants.MaxResultsPerPage)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(queries)
	}
}

// GetCreateSavedMRQLQueryHandler handles POST /v1/mrql/saved — create a saved MRQL query.
func GetCreateSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlSavedQueryRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		saved, err := ctx.CreateSavedMRQLQuery(req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(saved)
	}
}

// GetUpdateSavedMRQLQueryHandler handles PUT /v1/mrql/saved?id=N — update a saved MRQL query.
func GetUpdateSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			http_utils.HandleError(errors.New("saved MRQL query id is required"), writer, request, http.StatusBadRequest)
			return
		}

		var req mrqlSavedQueryRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		saved, err := ctx.UpdateSavedMRQLQuery(id, req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(saved)
	}
}

// GetDeleteSavedMRQLQueryHandler handles POST /v1/mrql/saved/delete?id=N — delete a saved MRQL query.
func GetDeleteSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(errors.New("saved MRQL query id is required"), writer, request, http.StatusBadRequest)
			return
		}

		err := ctx.DeleteSavedMRQLQuery(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/mrql") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

// GetRunSavedMRQLQueryHandler handles POST /v1/mrql/saved/run?id=N or ?name=X — execute a saved MRQL query.
func GetRunSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		name := http_utils.GetQueryParameter(request, "name", "")

		var saved *models.SavedMRQLQuery
		var err error

		if id != 0 {
			saved, err = ctx.GetSavedMRQLQuery(id)
		} else if name != "" {
			saved, err = ctx.GetSavedMRQLQueryByName(name)
		} else {
			http_utils.HandleError(errors.New("saved MRQL query id or name is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
			return
		}

		limit := int(http_utils.GetUIntQueryParameter(request, "limit", 0))
		page := int(http_utils.GetUIntQueryParameter(request, "page", 0))

		result, err := ctx.ExecuteMRQL(saved.Query, limit, page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}
