package api_handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"net/url"
	"strings"
)

func GetGroupsHandler(ctx interfaces.GroupReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.GroupQuery
		err := decoder.Decode(&query, request.URL.Query())
		query_models.FillMetaQueryFromRequest(request, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		groups, err := ctx.GetGroups(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			if http_utils.IsColumnError(err) {
				http_utils.HandleError(http_utils.ErrInvalidSortColumn, writer, request, http.StatusBadRequest)
				return
			}
			if http_utils.IsDateFilterError(err) {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(groups)
	}
}

func GetGroupHandler(ctx interfaces.GroupReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		group, err := ctx.GetGroup(id)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}

func GetGroupsParentsHandler(ctx interfaces.GroupReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		groups, err := ctx.FindParentsOfGroup(id)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(groups)
	}
}

func GetAddGroupHandler(ctx interfaces.GroupCRUDReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.GroupCRUDReader)

		var editor = query_models.GroupEditor{}
		var sentFields map[string]bool
		var group *models.Group
		var err error

		// For JSON requests, buffer the body so we can detect which fields
		// were explicitly included (distinguishing absent vs. empty).
		if strings.HasPrefix(request.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, writer, request, http.StatusBadRequest)
				return
			}
			if jsonErr := json.Unmarshal(bodyBytes, &editor); jsonErr != nil {
				http_utils.HandleError(jsonErr, writer, request, http.StatusBadRequest)
				return
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(bodyBytes, &raw)
			sentFields = make(map[string]bool, len(raw))
			for k := range raw {
				sentFields[k] = true
			}
		} else {
			err = tryFillStructValuesFromRequest(&editor, request)
		}

		if err != nil {
			http_utils.HandleFormError(writer, request, "/group/new", err, request.PostForm)
			return
		}

		if editor.ID == 0 {
			if editor.Name == "" {
				http_utils.HandleFormError(writer, request, "/group/new", fmt.Errorf("group name is required"), request.PostForm)
				return
			}

			group, err = effectiveCtx.CreateGroup(&editor.GroupCreator)
		} else {
			// Pre-populate unset fields from the existing group so partial
			// updates don't clear them. For JSON, use sentFields to distinguish
			// absent vs explicitly empty. For form-encoded, use formHasField.
			{
				existing, getErr := effectiveCtx.GetGroup(editor.ID)
				if getErr != nil {
					http_utils.HandleError(getErr, writer, request, statusCodeForError(getErr, http.StatusBadRequest))
					return
				}
				if existing != nil {
					fieldWasSent := func(field string) bool {
						if sentFields != nil {
							return sentFields[field]
						}
						return formHasField(request, field)
					}
					if editor.Name == "" {
						editor.Name = existing.Name
					}
					if editor.Description == "" && !fieldWasSent("Description") {
						editor.Description = existing.Description
					}
					if editor.Meta == "" {
						editor.Meta = string(existing.Meta)
					}
					if editor.URL == "" && existing.URL != nil && !fieldWasSent("URL") {
						editor.URL = (*url.URL)(existing.URL).String()
					}
					if editor.OwnerId == 0 && existing.OwnerId != nil && !formHasField(request, "OwnerId") {
						editor.OwnerId = *existing.OwnerId
					}
					if editor.CategoryId == 0 && existing.CategoryId != nil && !formHasField(request, "CategoryId") {
						editor.CategoryId = *existing.CategoryId
					}
					if editor.Tags == nil && len(existing.Tags) > 0 {
						editor.Tags = make([]uint, len(existing.Tags))
						for i, t := range existing.Tags {
							editor.Tags[i] = t.ID
						}
					}
					if editor.Groups == nil && len(existing.RelatedGroups) > 0 {
						editor.Groups = make([]uint, len(existing.RelatedGroups))
						for i, g := range existing.RelatedGroups {
							editor.Groups[i] = g.ID
						}
					}
				}
			}

			group, err = effectiveCtx.UpdateGroup(&editor)
		}

		if err != nil {
			redirectTarget := "/group/new"
			if editor.ID != 0 {
				redirectTarget = fmt.Sprintf("/group/edit?id=%d", editor.ID)
			}
			http_utils.HandleFormError(writer, request, redirectTarget, err, request.PostForm)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/group?id=%v", group.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(group)
	}
}

func GetRemoveGroupHandler(ctx interfaces.GroupDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.GroupDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid group ID"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteGroup(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

func GetAddTagsToGroupsHandler(ctx interfaces.BulkGroupTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkAddTagsToGroups(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			writeJSONOk(writer)
		}
	}
}

func GetRemoveTagsFromGroupsHandler(ctx interfaces.BulkGroupTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkRemoveTagsFromGroups(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			writeJSONOk(writer)
		}
	}
}

func GetBulkDeleteGroupsHandler(ctx interfaces.GroupDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.GroupDeleter)

		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if len(editor.ID) == 0 {
			http_utils.HandleError(fmt.Errorf("at least one group ID is required"), writer, request, http.StatusBadRequest)
			return
		}

		err = effectiveCtx.BulkDeleteGroups(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			writeJSONOk(writer)
		}
	}
}

func GetAddMetaToGroupsHandler(ctx interfaces.BulkGroupMetaEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditMetaQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkAddMetaToGroups(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			writeJSONOk(writer)
		}
	}
}

func GetGroupMetaKeysHandler(ctx interfaces.GroupMetaReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, err := ctx.GroupMetaKeys()

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(keys)
	}
}

func GetMergeGroupsHandler(ctx interfaces.GroupMerger) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.GroupMerger)

		var editor = query_models.MergeQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = effectiveCtx.MergeGroups(editor.Winner, editor.Losers)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/group?id=%v", editor.Winner)) {
			writeJSONOk(writer)
		}
	}
}

func GetGroupTreeChildrenHandler(ctx interfaces.GroupTreeReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		parentID := uint(http_utils.GetIntQueryParameter(request, "parentId", 0))
		limit := int(http_utils.GetIntQueryParameter(request, "limit", 50))

		if limit > 100 {
			limit = 100
		}

		var nodes []query_models.GroupTreeNode
		var err error

		if parentID == 0 {
			nodes, err = ctx.GetGroupTreeRoots(limit)
		} else {
			nodes, err = ctx.GetGroupTreeChildren(parentID, limit)
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(nodes)
	}
}

func GetDuplicateGroupHandler(ctx interfaces.GroupDuplicator) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.GroupDuplicator)

		var editor query_models.EntityIdQuery

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		group, err := effectiveCtx.DuplicateGroup(editor.ID)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/group?id=%v", group.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&group)
	}
}
