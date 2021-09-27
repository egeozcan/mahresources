package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"net/http"
	"strconv"
)

func GetAlbumUploadPreviewHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		id := http_utils.GetIntFormParameter(request, "id", 0)
		file, _, err := request.FormFile("resource")

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		defer file.Close()

		resource, err := ctx.AddThumbnailToAlbum(file, id)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetAlbumsHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResults
		var query http_query.AlbumQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(writer, err.Error())
			return
		}

		albums, err := ctx.GetAlbums(int(offset), constants.MaxResults, &query)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(albums)
	}
}

func GetAlbumHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		album, err := ctx.GetAlbum(id)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(album)
	}
}

func GetAddAlbumHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var queryVars = http_query.AlbumEditor{}
		err = decoder.Decode(&queryVars, request.PostForm)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		album, err := ctx.CreateOrUpdateAlbum(&queryVars)

		if err != nil {
			writer.WriteHeader(400)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/album?id="+strconv.Itoa(int(album.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(album)
	}
}
