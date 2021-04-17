package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"net/http"
)

func GetTagsHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResults
		var query http_query.TagQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(writer, err.Error())
			return
		}

		albums, err := ctx.GetTags(int(offset), constants.MaxResults, &query)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(albums)
	}
}

func GetAddTagHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var creator = http_query.TagCreator{}
		err = decoder.Decode(&creator, request.PostForm)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		album, err := ctx.CreateTag(&creator)

		if err != nil {
			writer.WriteHeader(400)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectBackIfHTMLAccepted(writer, request) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(album)
	}
}
