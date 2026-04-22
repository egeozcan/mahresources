package api_handlers

import (
	"encoding/json"
	"errors"
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

		noteId := http_utils.GetUIntFormValue(request, "noteId", 0)
		if noteId == 0 {
			http_utils.HandleError(
				errors.New("noteId is required"),
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

// GetBulkUnshareNotesHandler powers POST /v1/admin/shares/bulk-revoke. BH-035.
// Accepts a form-encoded body with repeated ids=<noteId> fields (and, for the
// HTML-form fallback, an optional Accept: application/json header to switch
// the response to a JSON summary). Non-numeric IDs and missing notes are
// silently skipped so a partial form submission still makes progress. On
// success, the browser-form path (HTML Accept) redirects back to
// /admin/shares (303 See Other); API consumers get JSON with the revoke
// count.
func GetBulkUnshareNotesHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteSharer)

		if err := request.ParseForm(); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		raw := request.Form["ids"]
		ids := make([]uint, 0, len(raw))
		for _, s := range raw {
			if s == "" {
				continue
			}
			v, err := parseUintStrict(s)
			if err != nil || v == 0 {
				// BH-035: non-numeric or zero IDs are noise in a bulk form
				// submit (e.g. an unchecked "select all" checkbox with no
				// value). Skip rather than 400 — the admin still wants the
				// valid ones revoked.
				continue
			}
			ids = append(ids, v)
		}

		revoked, err := effectiveCtx.BulkUnshareNotes(ids)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		accept := request.Header.Get("Accept")
		if accept == constants.JSON {
			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"success":  true,
				"revoked":  revoked,
				"attempts": len(ids),
			})
			return
		}
		// Default to the browser-form flow: redirect back to the dashboard.
		http.Redirect(writer, request, "/admin/shares", http.StatusSeeOther)
	}
}

// parseUintStrict parses a base-10 uint with no sign and no leading/trailing
// whitespace. Kept local to avoid a dependency loop with http_utils for a
// single call site.
func parseUintStrict(s string) (uint, error) {
	var n uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a uint")
		}
		n = n*10 + uint(c-'0')
	}
	return n, nil
}

func GetUnshareNoteHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Enable request-aware logging if the context supports it
		effectiveCtx := withRequestContext(ctx, request).(interfaces.NoteSharer)

		noteId := http_utils.GetUIntFormValue(request, "noteId", 0)
		if noteId == 0 {
			http_utils.HandleError(
				errors.New("noteId is required"),
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
