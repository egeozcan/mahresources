package api_handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

// GetTemplatePartialsHandler lists template partials (JSON).
func GetTemplatePartialsHandler(ctx interfaces.TemplatePartialReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.TemplatePartialQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		partials, err := ctx.GetTemplatePartials(&query, int(offset), constants.MaxResultsPerPage)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(partials)
	}
}

// GetAddTemplatePartialHandler creates or updates a template partial. A non-zero
// ID updates; JSON/form partial updates pre-fill absent fields from the existing
// partial so unsent fields are preserved.
func GetAddTemplatePartialHandler(ctx interfaces.TemplatePartialWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.TemplatePartialWriter)

		var editor = query_models.TemplatePartialEditor{}

		if strings.HasPrefix(request.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, writer, request, http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(bodyBytes, &editor); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			if editor.ID != 0 {
				var raw map[string]json.RawMessage
				_ = json.Unmarshal(bodyBytes, &raw)
				// json.Unmarshal matches keys case-insensitively, and the model's
				// JSON shape is lower-case (json:"content"), so a client can send
				// "content"/"description". Normalize sent keys to lower-case so the
				// preservation check doesn't clobber a lower-case update.
				sent := make(map[string]bool, len(raw))
				for k := range raw {
					sent[strings.ToLower(k)] = true
				}
				existing, getErr := effectiveCtx.GetTemplatePartial(editor.ID)
				if getErr == nil {
					if !sent["description"] {
						editor.Description = existing.Description
					}
					if !sent["content"] {
						editor.Content = existing.Content
					}
				}
			}
		} else {
			if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			if editor.ID != 0 {
				existing, getErr := effectiveCtx.GetTemplatePartial(editor.ID)
				if getErr == nil {
					if editor.Description == "" && !formHasField(request, "Description") {
						editor.Description = existing.Description
					}
					if editor.Content == "" && !formHasField(request, "Content") {
						editor.Content = existing.Content
					}
				}
			}
		}

		partial, err := effectiveCtx.CreateOrUpdateTemplatePartial(&editor)
		if err != nil {
			redirectTarget := "/templatePartial/new"
			if editor.ID != 0 {
				redirectTarget = fmt.Sprintf("/templatePartial/edit?id=%d", editor.ID)
			}
			http_utils.HandleFormError(writer, request, redirectTarget, err, request.PostForm)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/templatePartial?id="+strconv.Itoa(int(partial.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(partial)
	}
}

// GetRemoveTemplatePartialHandler deletes a template partial by ID.
func GetRemoveTemplatePartialHandler(ctx interfaces.TemplatePartialDeleter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.TemplatePartialDeleter)

		id := getEntityID(request)
		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid template partial ID"), writer, request, http.StatusBadRequest)
			return
		}

		if err := effectiveCtx.DeleteTemplatePartial(id); err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/templatePartials") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.TemplatePartial{ID: id})
	}
}
