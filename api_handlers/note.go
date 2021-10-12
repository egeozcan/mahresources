package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"net/http"
	"strconv"
)

func GetNotesHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResults
		var query http_query.NoteQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(writer, err.Error())
			return
		}

		notes, err := ctx.GetNotes(int(offset), constants.MaxResults, &query)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(notes)
	}
}

func GetNoteHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		note, err := ctx.GetNote(id)

		if err != nil {
			writer.WriteHeader(404)
			fmt.Fprint(writer, err.Error())
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(note)
	}
}

func GetAddNoteHandler(ctx *context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := request.ParseForm()

		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		var queryVars = http_query.NoteEditor{}
		err = decoder.Decode(&queryVars, request.PostForm)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(writer, err.Error())
		}

		note, err := ctx.CreateOrUpdateNote(&queryVars)

		if err != nil {
			writer.WriteHeader(400)
			_, _ = fmt.Fprint(writer, err.Error())
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/note?id="+strconv.Itoa(int(note.ID))) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(note)
	}
}
