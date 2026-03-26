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
		context := templateContextGenerator(request)

		// Check for redirect signal from context provider
		if redirectURL, ok := context["_redirect"].(string); ok && redirectURL != "" {
			http.Redirect(writer, request, redirectURL, http.StatusFound)
			return
		}

		if accept := request.Header.Get("Accept"); strings.Contains(accept, constants.JSON) || strings.HasSuffix(request.URL.Path, ".json") {
			writer.Header().Set("Content-Type", constants.JSON)
			if statusCode, ok := context["_statusCode"].(int); ok && statusCode != http.StatusOK {
				writer.WriteHeader(statusCode)
			}
			if err := json.NewEncoder(writer).Encode(discardFields(map[string]bool{
				"partial":        true,
				"path":           true,
				"withQuery":      true,
				"hasQuery":       true,
				"stringId":       true,
				"getNextId":      true,
				"dereference":    true,
				"_pluginManager":  true,
				"_statusCode":     true,
				"currentPath":     true,
				"pluginMenuItems": true,
			}, context)); err != nil {
				fmt.Println(err)
			}
			return
		}

		writer.Header().Set("Content-Type", constants.HTML)
		if statusCode, ok := context["_statusCode"].(int); ok && statusCode != http.StatusOK {
			writer.WriteHeader(statusCode)
			// Render the error template instead of the entity template to avoid
			// panics from templates that access nil entity variables.
			errorTpl := pongo2.Must(renderer.FromFile("error.tpl"))
			if err := errorTpl.ExecuteWriter(context, writer); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		if err := template.ExecuteWriter(context, writer); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

// RenderNotFound renders a styled 404 page using the error template.
func RenderNotFound(writer http.ResponseWriter, request *http.Request) {
	renderer := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("./templates", make(map[string]string)))
	errorTpl := pongo2.Must(renderer.FromFile("error.tpl"))
	context := pongo2.Context{
		"errorMessage": "Page not found",
		"title":        "mahresources",
		"pageTitle":    "404 Not Found",
	}
	writer.Header().Set("Content-Type", constants.HTML)
	writer.WriteHeader(http.StatusNotFound)
	if err := errorTpl.ExecuteWriter(context, writer); err != nil {
		http.Error(writer, "404 page not found", http.StatusNotFound)
	}
}
