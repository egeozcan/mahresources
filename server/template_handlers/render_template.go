package template_handlers

import (
	"encoding/json"
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/server/template_handlers/loaders"
	_ "mahresources/server/template_handlers/template_filters"
	"net/http"
	"strings"
)

import _ "github.com/flosch/pongo2-addons"

func discardFields(fields map[string]bool, intMap map[string]interface{}) map[string]interface{} {
	res := make(map[string]interface{})

	for key, value := range intMap {
		if _, exists := fields[key]; !exists && value != nil {
			res[key] = value
		}
	}

	return res
}

func RenderTemplate(templateName string, templateContextGenerator func(request *http.Request) pongo2.Context) func(writer http.ResponseWriter, request *http.Request) {
	templateSet := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("./templates", make(map[string]string)))
	bodyOnlyTemplateSet := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("./templates", map[string]string{"/base.tpl": "/bodyOnly.tpl"}))

	return func(writer http.ResponseWriter, request *http.Request) {
		renderer := templateSet

		if strings.HasSuffix(request.URL.Path, ".body") {
			renderer = bodyOnlyTemplateSet
		}

		var template = pongo2.Must(renderer.FromFile(templateName))
		var errorTemplate = pongo2.Must(renderer.FromFile("error.tpl"))

		context := templateContextGenerator(request)

		if request.Header.Get("Content-type") == constants.JSON || strings.HasSuffix(request.URL.Path, ".json") {
			writer.Header()["Content-Type"] = []string{constants.JSON}

			err := json.NewEncoder(writer).Encode(discardFields(map[string]bool{
				"partial":   true,
				"path":      true,
				"withQuery": true,
				"hasQuery":  true,
				"stringId":  true,
				"getNextId": true,
			}, context))

			if err != nil {
				fmt.Println(err)
			}

			return
		}

		writer.Header().Add("Content-Type", constants.HTML)

		if errMessage := context["errorMessage"]; errMessage != nil && errMessage != "" {
			err := errorTemplate.ExecuteWriter(context, writer)

			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}

			return
		}

		err := template.ExecuteWriter(context, writer)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
