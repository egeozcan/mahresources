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
	"strconv"
	"strings"
)

func GetNotesHandler(ctx interfaces.NoteReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.NoteQuery

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if notes, err := ctx.GetNotes(int(offset), constants.MaxResultsPerPage, &query); err != nil {
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
		} else {
			http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(notes)
		}
	}
}

func GetNoteHandler(ctx interfaces.NoteReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		note, err := ctx.GetNote(id)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(note)
	}
}

func GetAddNoteHandler(ctx interfaces.NoteWriteReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteWriteReader)

		var queryVars = query_models.NoteEditor{}
		var sentFields map[string]bool

		// For JSON requests, buffer the body so we can detect which fields
		// were explicitly included (distinguishing absent vs. empty).
		if strings.HasPrefix(request.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, writer, request, http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(bodyBytes, &queryVars); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(bodyBytes, &raw)
			sentFields = make(map[string]bool, len(raw))
			for k := range raw {
				sentFields[k] = true
			}
		} else {
			if err := tryFillStructValuesFromRequest(&queryVars, request); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
		}

		// Pre-populate unset fields from the existing note so partial updates
		// don't clear them. For JSON, use sentFields to distinguish absent vs
		// explicitly empty. For form-encoded, use formHasField.
		if queryVars.ID != 0 {
			existing, getErr := effectiveCtx.GetNote(queryVars.ID)
			if getErr == nil {
				fieldWasSent := func(field string) bool {
					if sentFields != nil {
						return sentFields[field]
					}
					return formHasField(request, field)
				}
				if queryVars.Name == "" {
					queryVars.Name = existing.Name
				}
				if queryVars.Description == "" && !fieldWasSent("Description") {
					queryVars.Description = existing.Description
				}
				if queryVars.Meta == "" {
					queryVars.Meta = string(existing.Meta)
				}
				if queryVars.StartDate == "" && existing.StartDate != nil && !fieldWasSent("StartDate") {
					queryVars.StartDate = existing.StartDate.Format("2006-01-02T15:04")
				}
				if queryVars.EndDate == "" && existing.EndDate != nil && !fieldWasSent("EndDate") {
					queryVars.EndDate = existing.EndDate.Format("2006-01-02T15:04")
				}
				if queryVars.NoteTypeId == 0 && existing.NoteTypeId != nil && !formHasField(request, "NoteTypeId") {
					queryVars.NoteTypeId = *existing.NoteTypeId
				}
				if queryVars.OwnerId == 0 && existing.OwnerId != nil && !formHasField(request, "OwnerId") {
					queryVars.OwnerId = *existing.OwnerId
				}
				// Pre-populate nil association arrays so partial JSON updates
				// don't clear them. Explicit empty arrays ([]uint{}) are left
				// as-is, allowing intentional clearing.
				if queryVars.Tags == nil && len(existing.Tags) > 0 {
					queryVars.Tags = make([]uint, len(existing.Tags))
					for i, t := range existing.Tags {
						queryVars.Tags[i] = t.ID
					}
				}
				if queryVars.Groups == nil && len(existing.Groups) > 0 {
					queryVars.Groups = make([]uint, len(existing.Groups))
					for i, g := range existing.Groups {
						queryVars.Groups[i] = g.ID
					}
				}
				if queryVars.Resources == nil && len(existing.Resources) > 0 {
					queryVars.Resources = make([]uint, len(existing.Resources))
					for i, r := range existing.Resources {
						queryVars.Resources[i] = r.ID
					}
				}
			}
		}

		note, err := effectiveCtx.CreateOrUpdateNote(&queryVars)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/note?id=%v", note.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(note)
	}
}

func GetRemoveNoteHandler(ctx interfaces.NoteDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid note ID"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteNote(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

func GetNoteMetaKeysHandler(ctx interfaces.NoteMetaReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, err := ctx.NoteMetaKeys()

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(keys)
	}
}

func GetNoteTypesHandler(ctx interfaces.NoteTypeReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.NoteTypeQuery
		err := decoder.Decode(&query, request.URL.Query())
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		noteTypes, err := ctx.GetNoteTypes(&query, int(offset), constants.MaxResultsPerPage)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, -1)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(noteTypes)
	}
}

func GetAddNoteTypeHandler(ctx interfaces.NoteTypeWriter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteTypeWriter)

		var editor = query_models.NoteTypeEditor{}

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
			// For JSON partial updates, pre-fill absent fields from existing entity
			if editor.ID != 0 {
				var raw map[string]json.RawMessage
				_ = json.Unmarshal(bodyBytes, &raw)
				existing, getErr := effectiveCtx.GetNoteType(editor.ID)
				if getErr == nil {
					if _, sent := raw["Description"]; !sent {
						editor.Description = existing.Description
					}
					if _, sent := raw["CustomHeader"]; !sent {
						editor.CustomHeader = existing.CustomHeader
					}
					if _, sent := raw["CustomSidebar"]; !sent {
						editor.CustomSidebar = existing.CustomSidebar
					}
					if _, sent := raw["CustomSummary"]; !sent {
						editor.CustomSummary = existing.CustomSummary
					}
					if _, sent := raw["CustomAvatar"]; !sent {
						editor.CustomAvatar = existing.CustomAvatar
					}
					if _, sent := raw["CustomMRQLResult"]; !sent {
						editor.CustomMRQLResult = existing.CustomMRQLResult
					}
					if _, sent := raw["MetaSchema"]; !sent {
						editor.MetaSchema = existing.MetaSchema
					}
					if _, sent := raw["SectionConfig"]; !sent {
						editor.SectionConfig = string(existing.SectionConfig)
					}
				}
			}
		} else {
			if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			// For form-encoded partial updates, pre-fill absent fields from
			// the existing entity so unsent fields are not wiped.
			if editor.ID != 0 {
				existing, getErr := effectiveCtx.GetNoteType(editor.ID)
				if getErr == nil {
					if editor.Description == "" && !formHasField(request, "Description") {
						editor.Description = existing.Description
					}
					if editor.CustomHeader == "" && !formHasField(request, "CustomHeader") {
						editor.CustomHeader = existing.CustomHeader
					}
					if editor.CustomSidebar == "" && !formHasField(request, "CustomSidebar") {
						editor.CustomSidebar = existing.CustomSidebar
					}
					if editor.CustomSummary == "" && !formHasField(request, "CustomSummary") {
						editor.CustomSummary = existing.CustomSummary
					}
					if editor.CustomAvatar == "" && !formHasField(request, "CustomAvatar") {
						editor.CustomAvatar = existing.CustomAvatar
					}
					if editor.MetaSchema == "" && !formHasField(request, "MetaSchema") {
						editor.MetaSchema = existing.MetaSchema
					}
					if editor.SectionConfig == "" && !formHasField(request, "SectionConfig") {
						editor.SectionConfig = string(existing.SectionConfig)
					}
				}
			}
		}

		noteType, err := effectiveCtx.CreateOrUpdateNoteType(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/noteType?id="+strconv.Itoa(int(noteType.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(noteType)
	}
}

func GetRemoveNoteTypeHandler(ctx interfaces.NoteTypeDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteTypeDeleter)

		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid note type ID"), writer, request, http.StatusBadRequest)
			return
		}

		err := effectiveCtx.DeleteNoteType(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/noteTypes") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(&models.NoteType{ID: id})
	}
}

func GetAddTagsToNotesHandler(ctx interfaces.BulkNoteTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkAddTagsToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			writeJSONOk(writer)
		}
	}
}

func GetRemoveTagsFromNotesHandler(ctx interfaces.BulkNoteTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkRemoveTagsFromNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			writeJSONOk(writer)
		}
	}
}

func GetAddGroupsToNotesHandler(ctx interfaces.BulkNoteGroupEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkAddGroupsToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			writeJSONOk(writer)
		}
	}
}

func GetAddMetaToNotesHandler(ctx interfaces.BulkNoteMetaEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditMetaQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		err = ctx.BulkAddMetaToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			writeJSONOk(writer)
		}
	}
}

func GetBulkDeleteNotesHandler(ctx interfaces.NoteDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteDeleter)

		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if len(editor.ID) == 0 {
			http_utils.HandleError(fmt.Errorf("at least one note ID is required"), writer, request, http.StatusBadRequest)
			return
		}

		err = effectiveCtx.BulkDeleteNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, errorStatusCode(err))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/notes") {
			writeJSONOk(writer)
		}
	}
}
