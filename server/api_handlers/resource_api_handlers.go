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
	"strconv"
)

func GetResourcesHandler(ctx interfaces.ResourceReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query query_models.ResourceSearchQuery

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		resources, err := ctx.GetResources(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			writer.WriteHeader(404)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resources)
	}
}

func GetResourceHandler(ctx interfaces.ResourceReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.EntityIdQuery

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		resource, err := ctx.GetResource(query.ID)

		if err != nil {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetResourceUploadHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		var creator = query_models.ResourceCreator{}

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		files := request.MultipartForm.File["resource"]

		if len(files) == 0 {
			http.Error(writer, "no files found to save", http.StatusBadRequest)
			return
		}

		var resources = make([]*models.Resource, len(files))

		for i := range files {
			func() {
				var res *models.Resource
				file, err := files[i].Open()

				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}

				defer file.Close()

				name := files[i].Filename
				res, err = ctx.AddResource(file, name, &creator)
				resources[i] = res

				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					_, _ = fmt.Fprint(writer, err.Error())
					return
				}
			}()
		}

		var redirectUrl string

		if len(files) == 1 {
			redirectUrl = fmt.Sprintf("/resource?id=%v", resources[0].ID)
		} else {
			redirectUrl = fmt.Sprintf("/group?id=%v", creator.OwnerId)
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, redirectUrl) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resources)
	}
}

func GetResourceAddLocalHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		var creator = query_models.ResourceFromLocalCreator{}

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		res, err := ctx.AddLocalResource(creator.Name, &creator)

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", res.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceAddRemoteHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		var creator = query_models.ResourceFromRemoteCreator{}

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		res, err := ctx.AddRemoteResource(&creator)

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", res.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceEditHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.ResourceEditor{}
		err := tryFillStructValuesFromRequest(&editor, request)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
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

func GetResourceThumbnailHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query = query_models.ResourceThumbnailQuery{}
		err := tryFillStructValuesFromRequest(&query, request)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		thumbnail, err := ctx.LoadOrCreateThumbnailForResource(query.ID, query.Width, query.Height)

		if err != nil || thumbnail == nil {
			http.Redirect(writer, request, "/public/placeholders/file.jpg", http.StatusMovedPermanently)
			return
		}

		writer.Header().Set("Content-Type", thumbnail.ContentType)
		_, err = writer.Write(thumbnail.Data)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}
	}
}

func GetRemoveResourceHandler(ctx interfaces.ResourceDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query = query_models.EntityIdQuery{}

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if err := ctx.DeleteResource(query.ID); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resources") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Resource{ID: query.ID})
	}
}

func GetResourceMetaKeysHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, err := ctx.ResourceMetaKeys()

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(keys)
	}
}
