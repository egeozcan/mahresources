package api_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"path"
	"strconv"
	"strings"
)

func GetResourcesHandler(ctx interfaces.ResourceReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query query_models.ResourceSearchQuery

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		resources, err := ctx.GetResources(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
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
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		resource, err := ctx.GetResource(query.ID)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(resource)
	}
}

func GetResourceContentHandler(ctx interfaces.ResourceReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.EntityIdQuery
		var detailsQuery query_models.ResourceSearchQuery
		var resource *models.Resource

		if err := tryFillStructValuesFromRequest(&query, request); err != nil || query.ID == 0 {
			if err := tryFillStructValuesFromRequest(&detailsQuery, request); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			} else {
				resources, err := ctx.GetResources(0, 1, &detailsQuery)

				if err != nil || len(*resources) != 1 {
					http_utils.HandleError(errors.New("no suitable resource found"), writer, request, http.StatusNotFound)
					return
				}

				resource = &(*resources)[0]
			}
		} else {
			resource, err = ctx.GetResource(query.ID)

			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
		}

		storage := "files"

		if resource.StorageLocation != nil && *resource.StorageLocation != "" {
			storage = *resource.StorageLocation
		}

		http.Redirect(writer, request, path.Join("/", storage, resource.GetCleanLocation()), http.StatusFound)
	}
}

func GetResourceUploadHandler(ctx interfaces.ResourceCreator) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceCreator)

		var remoteCreator = query_models.ResourceFromRemoteCreator{}

		if err := tryFillStructValuesFromRequest(&remoteCreator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if remoteCreator.URL != "" {
			res, err := effectiveCtx.AddRemoteResource(&remoteCreator)

			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
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
		var errorMessages = make([]string, 0)

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

				res, err = effectiveCtx.AddResource(file, name, &creator)
				resources[i] = res

				if err != nil {
					errorMessages = append(errorMessages, err.Error())
				}
			}(i)
		}

		if len(errorMessages) > 0 {
			messageText := strings.Join(errorMessages, ", ")
			aggregateError := errors.New(fmt.Sprintf("following errors were encountered: %v", messageText))
			http_utils.HandleError(aggregateError, writer, request, http.StatusInternalServerError)
			return
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

func GetResourceAddLocalHandler(ctx interfaces.ResourceCreator) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceCreator)

		var creator = query_models.ResourceFromLocalCreator{}

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		res, err := effectiveCtx.AddLocalResource(creator.Name, &creator)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", res.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceAddRemoteHandler(ctx interfaces.ResourceCreator) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceCreator)

		var creator = query_models.ResourceFromRemoteCreator{}

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// Check for background parameter - if true, queue for background download
		// Support both "background" and "Background" keys, and values "true", "1", or any truthy string
		bgVal := request.FormValue("background")
		if bgVal == "" {
			bgVal = request.FormValue("Background")
		}
		if bgVal == "" {
			bgVal = request.URL.Query().Get("background")
		}
		background := bgVal == "true" || bgVal == "1" || bgVal == "True" || bgVal == "TRUE"

		if background {
			if queueCtx, ok := effectiveCtx.(DownloadQueueReader); ok {
				jobs, err := queueCtx.DownloadManager().SubmitMultiple(&creator)
				if err != nil {
					http_utils.HandleError(err, writer, request, http.StatusServiceUnavailable)
					return
				}

				// Redirect HTML clients to appropriate page
				redirectURL := "/resources"
				if creator.OwnerId != 0 {
					redirectURL = fmt.Sprintf("/group?id=%d", creator.OwnerId)
				}
				if http_utils.RedirectIfHTMLAccepted(writer, request, redirectURL) {
					return
				}

				writer.Header().Set("Content-Type", constants.JSON)
				writer.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(writer).Encode(map[string]interface{}{
					"queued": true,
					"jobs":   jobs,
				})
				return
			}
		}

		res, err := effectiveCtx.AddRemoteResource(&creator)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", res.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceEditHandler(ctx interfaces.ResourceEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceEditor)

		var editor = query_models.ResourceEditor{}
		err := tryFillStructValuesFromRequest(&editor, request)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		res, err := effectiveCtx.EditResource(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resource?id="+strconv.Itoa(int(res.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(res)
	}
}

func GetResourceThumbnailHandler(ctx interfaces.ResourceThumbnailLoader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query = query_models.ResourceThumbnailQuery{}
		err := tryFillStructValuesFromRequest(&query, request)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		resource, err := ctx.GetResource(query.ID)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
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

		thumbnail, err := ctx.LoadOrCreateThumbnailForResource(query.ID, query.Width, query.Height, request.Context())

		if err != nil || thumbnail == nil {
			fmt.Printf("\n[ERROR]: %v\n", err)
			writer.Header().Set("Cache-Control", "no-cache")
			http.Redirect(writer, request, "/public/placeholders/file.jpg", http.StatusTemporaryRedirect)
			return
		}

		writer.Header().Set("Content-Type", thumbnail.ContentType)
		writer.Header().Set("Content-Length", strconv.Itoa(len(thumbnail.Data)))
		_, err = writer.Write(thumbnail.Data)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
	}
}

func GetRemoveResourceHandler(ctx interfaces.ResourceDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceDeleter)

		var query = query_models.EntityIdQuery{}

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if err := effectiveCtx.DeleteResource(query.ID); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resources") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.Resource{ID: query.ID})
	}
}

func GetResourceMetaKeysHandler(ctx interfaces.ResourceMetaReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, err := ctx.ResourceMetaKeys()

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.Header().Set("Cache-Control", "max-age=259200")
		_ = json.NewEncoder(writer).Encode(keys)
	}
}

func GetAddTagsToResourcesHandler(ctx interfaces.BulkResourceTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceTagEditor)

		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkAddTagsToResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetAddGroupsToResourcesHandler(ctx interfaces.BulkResourceGroupEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceGroupEditor)

		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkAddGroupsToResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetRemoveTagsFromResourcesHandler(ctx interfaces.BulkResourceTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceTagEditor)

		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkRemoveTagsFromResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetReplaceTagsOfResourcesHandler(ctx interfaces.BulkResourceTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceTagEditor)

		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkReplaceTagsFromResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetAddMetaToResourcesHandler(ctx interfaces.BulkResourceMetaEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceMetaEditor)

		var editor = query_models.BulkEditMetaQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkAddMetaToResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetBulkDeleteResourcesHandler(ctx interfaces.BulkResourceDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkResourceDeleter)

		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkDeleteResources(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetMergeResourcesHandler(ctx interfaces.ResourceMerger) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceMerger)

		var editor = query_models.MergeQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.MergeResources(editor.Winner, editor.Losers)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", editor.Winner))
	}
}

func GetRotateResourceHandler(ctx interfaces.ResourceMediaProcessor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.RotateResourceQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.RotateResource(editor.ID, editor.Degrees)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", editor.ID))
	}
}

func GetBulkCalculateDimensionsHandler(ctx interfaces.ResourceMediaProcessor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		encounteredErrors := make([]error, 0)

		for _, id := range editor.ID {
			err = ctx.RecalculateResourceDimensions(&query_models.EntityIdQuery{ID: id})

			if err != nil {
				encounteredErrors = append(encounteredErrors, err)
			}
		}

		if len(encounteredErrors) > 0 {
			http_utils.HandleError(errors.New("encountered errors during dimension calculation"), writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/resources")
	}
}

func GetResourceSetDimensionsHandler(ctx interfaces.ResourceMediaProcessor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.ResourceEditor{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.SetResourceDimensions(editor.ID, editor.Width, editor.Height)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", editor.ID))
	}
}
