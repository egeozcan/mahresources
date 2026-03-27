package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"net/url"
	"strconv"
)

func GetAddGroupRelationTypeHandler(ctx interfaces.RelationshipWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipWriter)

		var editor = query_models.RelationshipTypeEditorQuery{}

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		relationType, err := effectiveCtx.AddRelationType(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/relationType?id="+strconv.Itoa(int(relationType.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(relationType)
	}
}

func GetEditGroupRelationTypeHandler(ctx interfaces.RelationshipWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipWriter)

		var editor = query_models.RelationshipTypeEditorQuery{}

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		relationType, err := effectiveCtx.EditRelationType(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/relationType?id="+strconv.Itoa(int(relationType.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(relationType)
	}
}

func GetAddRelationHandler(ctx interfaces.RelationshipWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipWriter)

		var editor = query_models.GroupRelationshipQuery{}

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		var relation *models.GroupRelation
		var err error

		if editor.Id != 0 {
			relation, err = effectiveCtx.EditRelation(editor)
		} else {
			relation, err = effectiveCtx.AddRelation(editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId, editor.Name, editor.Description)
		}

		if err != nil {
			if http_utils.RequestAcceptsHTML(request) {
				backUrl := fmt.Sprintf(
					"/relation/new?FromGroupId=%v&ToGroupId=%v&GroupRelationTypeId=%v&Name=%v&Description=%v&Error=%v",
					editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId,
					url.QueryEscape(editor.Name), url.QueryEscape(editor.Description),
					url.QueryEscape(err.Error()),
				)
				http.Redirect(writer, request, backUrl, http.StatusSeeOther)
				return
			}
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/relation?id=%v", relation.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(relation)
	}
}

func GetRelationTypesHandler(ctx interfaces.RelationshipReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query = query_models.RelationshipTypeQuery{}

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		relationships, err := ctx.GetRelationTypes(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(relationships)
	}
}

func GetRemoveRelationHandler(ctx interfaces.RelationshipDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid relation ID"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteRelationship(id)
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

func GetRemoveRelationTypeHandler(ctx interfaces.RelationshipDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid relation type ID"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteRelationshipType(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/relationTypes") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}
