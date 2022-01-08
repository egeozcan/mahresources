package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers/interfaces"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
)

func GetCategoriesHandler(ctx interfaces.CategoryReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query query_models.CategoryQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		categories, err := ctx.GetCategories(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			writer.WriteHeader(404)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(categories)
	}
}

func GetAddCategoryHandler(ctx interfaces.CategoryWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var categoryEditor = query_models.CategoryEditor{}

		if err = tryFillStructValuesFromRequest(&categoryEditor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var category *models.Category

		if categoryEditor.ID != 0 {
			category, err = ctx.UpdateCategory(&categoryEditor)
		} else {
			category, err = ctx.CreateCategory(&categoryEditor.CategoryCreator)
		}

		if err != nil {
			writer.WriteHeader(400)
			_, _ = fmt.Fprint(writer, err.Error())
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

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		if id == 0 {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, "no id found")
			return
		}

		err := ctx.DeleteCategory(id)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/categories") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Category{ID: id})
	}
}
