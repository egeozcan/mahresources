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
	"strconv"
	"strings"
)

func GetAddGroupRelationTypeHandler(ctx interfaces.RelationshipWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipWriter)

		var editor = query_models.RelationshipTypeEditorQuery{}

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		relationType, err := effectiveCtx.AddRelationType(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
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
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		relationType, err := effectiveCtx.EditRelationType(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
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
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		var relation *models.GroupRelation
		var err error

		if editor.Id != 0 {
			relation, err = effectiveCtx.EditRelation(editor)
		} else {
			relation, err = effectiveCtx.AddRelation(editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId)
		}

		if err != nil {
			// For HTML requests, redirect back to the form with error
			// For API requests (Accept: application/json), return error as JSON
			accepts := request.Header.Get("Accept")
			if accepts == "" || (accepts != "application/json" && !strings.Contains(accepts, "application/json")) {
				backUrl := fmt.Sprintf(
					"/relation/new?FromGroupId=%v&ToGroupId=%v&GroupRelationTypeId=%v&Error=%v",
					editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId,
					err.Error(),
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
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
		var query = query_models.RelationshipTypeQuery{}

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		relationships, err := ctx.GetRelationTypes(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(relationships)
	}
}

func GetRemoveRelationHandler(ctx interfaces.RelationshipDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipDeleter)

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		err := effectiveCtx.DeleteRelationship(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/groups") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.GroupRelation{ID: id})
	}
}

func GetRemoveRelationTypeHandler(ctx interfaces.RelationshipDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.RelationshipDeleter)

		id := http_utils.GetUIntQueryParameter(request, "Id", 0)

		err := effectiveCtx.DeleteRelationshipType(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/relationTypes") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.GroupRelationType{ID: id})
	}
}
