package api_handlers

import (
	"encoding/json"
	"errors"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// Template Revision endpoints. One handler set serves all three carrier types;
// routes.go registers it per type. Every call is bound to the request so the
// operation's own edit-capability check sees the acting principal.

// revisionCarrierID reads the carrier id from the query string only. The label
// and restore endpoints take a body, and getEntityID would consume it.
func revisionCarrierID(request *http.Request) uint {
	return http_utils.GetUIntQueryParameter(request, "id", 0)
}

func revisionNumber(request *http.Request) (uint, bool) {
	n, err := strconv.ParseUint(mux.Vars(request)["number"], 10, 32)
	return uint(n), err == nil && n > 0
}

func writeRevisionJSON(writer http.ResponseWriter, v any) {
	writer.Header().Set("Content-Type", constants.JSON)
	_ = json.NewEncoder(writer).Encode(v)
}

func GetTemplateRevisionsHandler(carrierType string, ctx contracts.TemplateRevisionReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(contracts.TemplateRevisionReader)
		id := revisionCarrierID(request)
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}
		revisions, err := effectiveCtx.ListTemplateRevisions(carrierType, id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}
		writeRevisionJSON(writer, revisions)
	}
}

func GetTemplateRevisionHandler(carrierType string, ctx contracts.TemplateRevisionReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(contracts.TemplateRevisionReader)
		id := revisionCarrierID(request)
		number, ok := revisionNumber(request)
		if id == 0 || !ok {
			http_utils.HandleError(errors.New("id and a revision number are required"), writer, request, http.StatusBadRequest)
			return
		}
		revision, err := effectiveCtx.GetTemplateRevision(carrierType, id, number)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}
		writeRevisionJSON(writer, revision)
	}
}

func GetTemplateRevisionDiffHandler(carrierType string, ctx contracts.TemplateRevisionReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(contracts.TemplateRevisionReader)
		id := revisionCarrierID(request)
		from := http_utils.GetUIntQueryParameter(request, "from", 0)
		if id == 0 || from == 0 {
			http_utils.HandleError(errors.New("id and from are required"), writer, request, http.StatusBadRequest)
			return
		}
		var to *uint
		if raw := request.URL.Query().Get("to"); raw != "" {
			n, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || n == 0 {
				http_utils.HandleError(errors.New("to must be a revision number"), writer, request, http.StatusBadRequest)
				return
			}
			v := uint(n)
			to = &v
		}
		diff, err := effectiveCtx.DiffTemplateRevisions(carrierType, id, from, to)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}
		writeRevisionJSON(writer, diff)
	}
}

func GetLabelTemplateRevisionHandler(carrierType string, ctx contracts.TemplateRevisionWriter) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(contracts.TemplateRevisionWriter)
		id := revisionCarrierID(request)
		number, ok := revisionNumber(request)
		if id == 0 || !ok {
			http_utils.HandleError(errors.New("id and a revision number are required"), writer, request, http.StatusBadRequest)
			return
		}
		var body query_models.TemplateRevisionLabelRequest
		if err := tryFillStructValuesFromRequest(&body, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		summary, err := effectiveCtx.LabelTemplateRevision(carrierType, id, number, body.Label)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}
		writeRevisionJSON(writer, summary)
	}
}

func GetRestoreTemplateRevisionHandler(carrierType string, ctx contracts.TemplateRevisionWriter) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(contracts.TemplateRevisionWriter)
		id := revisionCarrierID(request)
		number, ok := revisionNumber(request)
		if id == 0 || !ok {
			http_utils.HandleError(errors.New("id and a revision number are required"), writer, request, http.StatusBadRequest)
			return
		}
		var body query_models.TemplateRevisionRestoreRequest
		if err := tryFillStructValuesFromRequest(&body, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		var slot *string
		if s := strings.TrimSpace(body.Slot); s != "" {
			slot = &s
		}
		includeCSS := body.IncludeCss == nil || *body.IncludeCss
		summary, err := effectiveCtx.RestoreTemplateRevision(carrierType, id, number, slot, includeCSS)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}
		writeRevisionJSON(writer, summary)
	}
}
