package template_handlers

import (
	"encoding/json"
	"fmt"

	_ "github.com/flosch/pongo2-addons"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
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
				// Function-valued fields (cannot serialize to JSON)
				"partial":     true,
				"path":        true,
				"withQuery":   true,
				"hasQuery":    true,
				"stringId":    true,
				"getNextId":   true,
				"dereference": true,
				"docsURL":     true,
				// Internal/rendering fields (should not leak to JSON consumers)
				"_pluginManager":      true,
				"_statusCode":         true,
				"_appContext":         true, // BH-P05: contains full MahresourcesConfig (DbDsn, FfmpegPath, FileSavePath, AltFileSystems, ...)
				"_requestContext":     true, // BH-P05: nested Go request context
				"_pluginAccess":       true, // a closure over the request principal; never serialisable
				"currentPath":         true,
				"pluginMenuItems":     true,
				"menu":                true,
				"adminMenu":           true,
				"title":               true,
				"assetVersion":        true,
				"queryValues":         true,
				"url":                 true,
				"hasPluginManager":    true,
				"pluginDetailActions": true,
				"pluginCardActions":   true,
				"pluginBulkActions":   true,
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

// renderErrorPage renders the app's error page: full chrome, the message in
// base.tpl's alert region, and the recovery links error.tpl adds under it.
// It is the single implementation behind every HTML error surface — the catch-all
// 404, an authorization rejection, and (through RenderHTMLError) the ~477
// HandleError call sites.
func renderErrorPage(
	writer http.ResponseWriter,
	request *http.Request,
	statusCode int,
	pageTitle string,
	message string,
	recovery []template_context_providers.RecoveryLink,
	contextEnricher func(ctx pongo2.Context) pongo2.Context,
	fallback string,
) {
	renderer := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("./templates", make(map[string]string)))
	errorTpl := pongo2.Must(renderer.FromFile("error.tpl"))
	context := template_context_providers.StaticTemplateCtx(request)
	context["errorMessage"] = message
	context["pageTitle"] = pageTitle
	context["errorRecovery"] = recovery
	if contextEnricher != nil {
		context = contextEnricher(context)
	}
	writer.Header().Set("Content-Type", constants.HTML)
	writer.WriteHeader(statusCode)
	if err := errorTpl.ExecuteWriter(context, writer); err != nil {
		http.Error(writer, fallback, statusCode)
	}
}

// RenderNotFound answers an unmatched route. Under /v1 it answers JSON whoever is
// asking, because that prefix is the JSON API and an HTML body there breaks every
// client's parse (finding 114); elsewhere it renders the app's 404 page.
// The optional contextEnricher function adds plugin context (menu items, etc.)
// to the template context. Pass nil when plugin context is not available.
func RenderNotFound(writer http.ResponseWriter, request *http.Request, contextEnricher func(ctx pongo2.Context) pongo2.Context) {
	if wantsJSONError(request) {
		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"error": fmt.Sprintf("no such endpoint: %s %s", request.Method, request.URL.Path),
		})
		return
	}

	// "Error 404" rather than the old "404 Not Found", so an unmatched path and a
	// missing entity present identically (finding 119); "Page not found" is the
	// message both e2e/tests/plugins/plugin-pages.spec.ts and readers expect.
	renderErrorPage(writer, request, http.StatusNotFound, "Error 404", "Page not found",
		[]template_context_providers.RecoveryLink{template_context_providers.DashboardRecovery},
		contextEnricher, "404 page not found")
}

// wantsJSONError reports whether an unmatched path should answer with a JSON
// error rather than a page. The /v1 prefix decides on its own: it is the JSON
// API, so a browser typing a /v1 URL gets the same answer a client does.
func wantsJSONError(request *http.Request) bool {
	path := request.URL.Path
	if strings.HasPrefix(path, "/v1/") || path == "/v1" {
		return true
	}
	if strings.HasSuffix(path, ".json") {
		return true
	}
	accept := request.Header.Get("Accept")
	return strings.Contains(accept, constants.JSON) && !strings.Contains(accept, constants.HTML)
}

// RenderForbidden renders a styled 403 page using the error template, so an
// authorization or CSRF rejection shows the app chrome instead of a bare
// http.Error string. The optional contextEnricher adds plugin context (menu
// items, etc.); pass nil when unavailable. message is the human-readable reason.
func RenderForbidden(writer http.ResponseWriter, request *http.Request, message string, contextEnricher func(ctx pongo2.Context) pongo2.Context) {
	if message == "" {
		message = "You do not have permission to access this page."
	}
	renderErrorPage(writer, request, http.StatusForbidden, "Error 403", message,
		[]template_context_providers.RecoveryLink{template_context_providers.DashboardRecovery},
		contextEnricher, "403 forbidden")
}

// RenderHTMLError is the renderer installed into http_utils.HandleError, so an
// API rejection a browser is about to display arrives as a page in the app
// instead of a standalone document whose only exit was history.back().
// contextEnricher adds plugin context, matching the 404 and 403 pages.
func RenderHTMLError(writer http.ResponseWriter, request *http.Request, statusCode int, message string, contextEnricher func(ctx pongo2.Context) pongo2.Context) {
	renderErrorPage(writer, request, statusCode,
		fmt.Sprintf("Error %d", statusCode), message,
		template_context_providers.RecoveryLinksForRequest(request.URL, request.Referer()),
		contextEnricher, message)
}
