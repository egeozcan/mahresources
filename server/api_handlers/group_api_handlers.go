package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers/interfaces"
	"mahresources/server/http_utils"
	"net/http"
)

func GetGroupsHandler(ctx interfaces.GroupReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query query_models.GroupQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		groups, err := ctx.GetGroups(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			writer.WriteHeader(404)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(groups)
	}
}

func GetGroupHandler(ctx interfaces.GroupReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		group, err := ctx.GetGroup(id)

		if err != nil {
			writer.WriteHeader(404)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}

func GetAddGroupHandler(ctx interfaces.GroupWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.GroupEditor{}
		var group *models.Group
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err == nil && editor.ID == 0 {
			if editor.Name == "" {
				writer.WriteHeader(http.StatusInternalServerError)
			}

			group, err = ctx.CreateGroup(&editor.GroupCreator)
		} else if err == nil {
			group, err = ctx.UpdateGroup(&editor)
		}

		if err != nil {
			writer.WriteHeader(400)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/group?id=%v", group.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}

func GetRemoveGroupHandler(ctx interfaces.GroupDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		err := ctx.DeleteGroup(id)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Group{ID: id})
	}
}

func GetGroupMetaKeysHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, err := ctx.GroupMetaKeys()

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(keys)
	}
}
