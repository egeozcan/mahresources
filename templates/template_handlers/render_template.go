package template_handlers

import (
	"github.com/flosch/pongo2/v4"
	"net/http"
)

import _ "github.com/flosch/pongo2-addons"

func RenderTemplate(templateName string, templateContextGenerator func(request *http.Request) pongo2.Context) func(writer http.ResponseWriter, request *http.Request) {
	templateSet := pongo2.NewSet("", pongo2.MustNewLocalFileSystemLoader("./templates"))

	return func(writer http.ResponseWriter, request *http.Request) {
		context := templateContextGenerator(request)

		if errMessage := context["errorMessage"]; errMessage != nil {
			templateName = "error.tpl"
		}

		var tplExample = pongo2.Must(templateSet.FromFile(templateName))
		err := tplExample.ExecuteWriter(context, writer)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
