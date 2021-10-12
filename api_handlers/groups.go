package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"mahresources/models"
	"net/http"
	"strconv"
)

func GetGroupsHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResults
		var query http_query.GroupQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(writer, err.Error())
			return
		}

		groups, err := ctx.GetGroups(int(offset), constants.MaxResults, &query)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(groups)
	}
}

func GetGroupsAutocompleteHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		name := http_utils.GetQueryParameter(request, "name", "")

		groups, err := ctx.GetGroupsAutoComplete(name, constants.MaxResults)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(groups)
	}
}

func GetGroupHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		group, err := ctx.GetGroup(id)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}

func GetAddGroupHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var editor = http_query.GroupEditor{}
		var group *models.Group

		if err = decoder.Decode(&editor, request.PostForm); err != nil || editor.ID == 0 {
			var creator = http_query.GroupCreator{}
			err = decoder.Decode(&creator, request.PostForm)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(writer, err.Error())
			}

			group, err = ctx.CreateGroup(&creator)
		} else {
			group, err = ctx.UpdateGroup(&editor)
		}

		if err != nil {
			writer.WriteHeader(400)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/group?id="+strconv.Itoa(int(group.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}
