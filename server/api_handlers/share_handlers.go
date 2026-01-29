package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

type ShareResponse struct {
	ShareToken string `json:"shareToken"`
	ShareUrl   string `json:"shareUrl"`
}

func GetShareNoteHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteSharer)

		noteId := http_utils.GetUIntQueryParameter(request, "noteId", 0)
		if noteId == 0 {
			http_utils.HandleError(
				&json.InvalidUnmarshalError{},
				writer,
				request,
				http.StatusBadRequest,
			)
			return
		}

		token, err := effectiveCtx.ShareNote(noteId)
		if err != nil {
			if err.Error() == "record not found" {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		shareUrl := "/s/" + token
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(ShareResponse{
			ShareToken: token,
			ShareUrl:   shareUrl,
		})
	}
}

func GetUnshareNoteHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteSharer)

		noteId := http_utils.GetUIntQueryParameter(request, "noteId", 0)
		if noteId == 0 {
			http_utils.HandleError(
				&json.InvalidUnmarshalError{},
				writer,
				request,
				http.StatusBadRequest,
			)
			return
		}

		err := effectiveCtx.UnshareNote(noteId)
		if err != nil {
			if err.Error() == "record not found" {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]bool{"success": true})
	}
}
