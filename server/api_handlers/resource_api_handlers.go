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
	"strings"
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

		var remoteCreator = query_models.ResourceFromRemoteCreator{}

		if err := tryFillStructValuesFromRequest(&remoteCreator, request); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if remoteCreator.URL != "" {
			res, err := ctx.AddRemoteResource(&remoteCreator)

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

			return
		}

		creator := query_models.ResourceCreator{ResourceQueryBase: remoteCreator.ResourceQueryBase}

		files := request.MultipartForm.File["resource"]

		if len(files) == 0 {
			http.Error(writer, "no files found to save", http.StatusBadRequest)
			return
		}

		var resources = make([]*models.Resource, len(files))

		for i := range files {
			func(i int) {
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
			}(i)
		}

		var redirectUrl string

		if len(files) == 1 {
			redirectUrl = fmt.Sprintf("/resource?id=%v", resources[0].ID)
		} else {
			redirectUrl = fmt.Sprintf("/group?id=%v", remoteCreator.OwnerId)
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

		resource, err := ctx.GetResource(query.ID)

		if err != nil {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		e := fmt.Sprintf(`"%v"`, resource.Hash)

		writer.Header().Set("Etag", e)
		writer.Header().Set("Cache-Control", "max-age=2592000")

		if match := request.Header.Get("If-None-Match"); match != "" {
			if strings.Contains(match, e) {
				writer.WriteHeader(http.StatusNotModified)
				return
			}
		}

		thumbnail, err := ctx.LoadOrCreateThumbnailForResource(query.ID, query.Width, query.Height)

		if err != nil || thumbnail == nil {
			http.Redirect(writer, request, "/public/placeholders/file.jpg", http.StatusMovedPermanently)
			return
		}

		writer.Header().Set("Content-Type", thumbnail.ContentType)
		writer.Header().Set("Content-Length", strconv.Itoa(len(thumbnail.Data)))
		_, err = writer.Write(thumbnail.Data)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
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
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.Header().Set("Cache-Control", "max-age=259200")
		_ = json.NewEncoder(writer).Encode(keys)
	}
}

func GetAddTagsToResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.BulkAddTagsToResources(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetAddGroupsToResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.BulkAddGroupsToResources(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetRemoveTagsFromResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.BulkRemoveTagsFromResources(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetAddMetaToResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditMetaQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.BulkAddMetaToResources(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetBulkDeleteResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.BulkDeleteResources(&editor)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetMergeResourcesHandler(ctx interfaces.ResourceWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.MergeQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		err = ctx.MergeResources(editor.Winner, editor.Losers)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", editor.Winner))
	}
}
