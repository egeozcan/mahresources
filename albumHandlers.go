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
		name := getFormParameter(request, "name", "")
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
