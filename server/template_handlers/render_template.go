package template_handlers

import (
	"encoding/json"
	"fmt"
	_ "github.com/flosch/pongo2-addons"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/server/template_handlers/loaders"
	_ "mahresources/server/template_handlers/template_filters"
	"net/http"
	"strings"
)

func discardFields(fields map[string]bool, intMap map[string]any) map[string]any {
	res := make(map[string]any, len(intMap))

	for key, value := range intMap {
		if _, exists := fields[key]; exists || value == nil {
			continue
		}
		res[key] = value
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

		template := pongo2.Must(renderer.FromFile(templateName))
		errorTemplate := pongo2.Must(renderer.FromFile("error.tpl"))
		context := templateContextGenerator(request)

		if contentType := request.Header.Get("Content-type"); contentType == constants.JSON || strings.HasSuffix(request.URL.Path, ".json") {
			writer.Header().Set("Content-Type", constants.JSON)
			if err := json.NewEncoder(writer).Encode(discardFields(map[string]bool{
				"partial":   true,
				"path":      true,
				"withQuery": true,
				"hasQuery":  true,
				"stringId":  true,
				"getNextId": true,
			}, context)); err != nil {
				fmt.Println(err)
			}
			return
		}

		writer.Header().Set("Content-Type", constants.HTML)
		if errMessage := context["errorMessage"]; errMessage != nil && errMessage != "" {
			if err := errorTemplate.ExecuteWriter(context, writer); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := template.ExecuteWriter(context, writer); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
