package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func getResourceHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := getIntQueryParameter(request, "id", 1)
		resource, err := ctx.getResource(id)

		if err != nil {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func getResourceUploadPreviewHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
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

		resource, err := ctx.addThumbnailToResource(file, id)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func getAddResourceToAlbumHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		resId := getIntFormParameter(request, "resId", 0)
		albumId := getIntFormParameter(request, "albumId", 0)

		resource, err := ctx.addResourceToAlbum(resId, albumId)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func getResourceUploadHandler(ctx *mahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(512 << 20); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, "error")
			fmt.Println(err)
			return
		}

		file, handler, err := request.FormFile("resource")

		if err != nil {
			fmt.Println("Error Retrieving the File")
			fmt.Println(err)
			return
		}
		defer file.Close()

		res, err := ctx.addResource(file, handler.Filename)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}