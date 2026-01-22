package api_handlers

import (
	"encoding/json"
	"errors"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"strconv"
)

func GetCategoriesHandler(ctx interfaces.CategoryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query query_models.CategoryQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		categories, err := ctx.GetCategories(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(categories)
	}
}

func GetAddCategoryHandler(ctx interfaces.CategoryWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.CategoryWriter)

		err := request.ParseForm()

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		var categoryEditor = query_models.CategoryEditor{}

		if err = tryFillStructValuesFromRequest(&categoryEditor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		var category *models.Category

		if categoryEditor.ID != 0 {
			category, err = effectiveCtx.UpdateCategory(&categoryEditor)
		} else {
			category, err = effectiveCtx.CreateCategory(&categoryEditor.CategoryCreator)
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/category?id="+strconv.Itoa(int(category.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(category)
	}
}

func GetRemoveCategoryHandler(ctx interfaces.CategoryDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.CategoryDeleter)

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		if id == 0 {
			http_utils.HandleError(errors.New("category id is needed"), writer, request, http.StatusInternalServerError)
			return
		}

		err := effectiveCtx.DeleteCategory(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/categories") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Category{ID: id})
	}
}
