package api_handlers

import (
	"fmt"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
)

func GetEditEntityNameHandler[T interfaces.BasicEntityReader](ctx interfaces.BasicEntityWriter[T], name string) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BasicEntityQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		err = ctx.UpdateName(id, editor.Name)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/%s?id=%v", name, id))
	}
}

func GetEditEntityDescriptionHandler[T interfaces.BasicEntityReader](ctx interfaces.BasicEntityWriter[T], name string) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BasicEntityQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		err = ctx.UpdateDescription(id, editor.Description)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/%s?id=%v", name, id))
	}
}
