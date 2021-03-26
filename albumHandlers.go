package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func getAlbumsHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (getIntQueryParameter(request, "page", 1) - 1) * MaxResults
		albums, err := ctx.getAlbums(int(offset), MaxResults)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(albums)
	}
}

func getAlbumHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(getIntQueryParameter(request, "id", 0))
		album, err := ctx.getAlbum(id)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(album)
	}
}

func getAddAlbumHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			fmt.Fprint(writer, err.Error())
			return
		}

		name := getFormParameter(request, "name", "")
		fmt.Println("album name:", name)
		album, err := ctx.createAlbum(name)

		if err != nil {
			writer.WriteHeader(400)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(album)
	}
}

func getAlbumUploadPreviewHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		id := getIntFormParameter(request, "id", 0)
		file, _, err := request.FormFile("resource")

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		defer file.Close()

		resource, err := ctx.addThumbnailToAlbum(file, id)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}
