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

func GetResourceHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 1)
		resource, err := ctx.GetResource(id)

		if err != nil {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetResourceUploadPreviewHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
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

		resource, err := ctx.AddThumbnailToResource(file, id)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetAddResourceToNoteHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		resId := http_utils.GetIntFormParameter(request, "resId", 0)
		noteId := http_utils.GetIntFormParameter(request, "noteId", 0)

		resource, err := ctx.AddResourceToNote(resId, noteId)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetResourceUploadHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(int64(4096) << 20); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, "error")
			fmt.Println(err)
			return
		}

		var res *models.Resource
		var err error

		var creator = http_query.ResourceCreator{}
		if err := decoder.Decode(&creator, request.PostForm); err != nil {
			fmt.Println("Error parsing")
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

		name := handler.Filename
		res, err = ctx.AddResource(file, name, &creator)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resource?id="+strconv.Itoa(int(res.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceEditHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, "error")
			fmt.Println(err)
			return
		}

		var editor = http_query.ResourceEditor{}

		if err := decoder.Decode(&editor, request.PostForm); err != nil {
			fmt.Println("Error parsing")
			fmt.Println(err)
			return
		}

		res, err := ctx.EditResource(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resource?id="+strconv.Itoa(int(res.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}
