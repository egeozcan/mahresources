package api_handlers

import (
	"encoding/json"
	"errors"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
)

func GetCategoriesHandler(ctx contracts.CategoryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
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

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(categories)
	}
}

func GetAddCategoryHandler(ctx contracts.CategoryWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(contracts.CategoryWriter)

		err := request.ParseForm()

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		var categoryEditor = query_models.CategoryEditor{}

		if err = tryFillStructValuesFromRequest(&categoryEditor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
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

func GetRemoveCategoryHandler(ctx contracts.CategoryDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(contracts.CategoryDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(errors.New("category id is needed"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteCategory(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/categories") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

func GetRemoveResourceCategoryHandler(ctx contracts.ResourceCategoryDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(contracts.ResourceCategoryDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(errors.New("resource category id is needed"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteResourceCategory(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resourceCategories") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}
