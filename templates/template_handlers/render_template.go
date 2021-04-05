package template_handlers

import (
	"github.com/flosch/pongo2/v4"
	"net/http"
)

func RenderTemplate(templateName string, templateContext func(request *http.Request) pongo2.Context) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var tplExample = pongo2.Must(pongo2.FromFile(templateName))
		err := tplExample.ExecuteWriter(templateContext(request), writer)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
