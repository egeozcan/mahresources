package template_context_providers

import (
	"io"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_system"
)

func PluginPageContextProvider(pm *plugin_system.PluginManager) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := staticTemplateCtx(request)

		// Parse /plugins/{pluginName}/{path...} from URL
		path := strings.TrimPrefix(request.URL.Path, "/plugins/")
		parts := strings.SplitN(path, "/", 2)

		pluginName := ""
		pagePath := ""
		if len(parts) >= 1 {
			pluginName = parts[0]
		}
		if len(parts) >= 2 {
			pagePath = parts[1]
		}

		ctx["pageTitle"] = "Plugin: " + pluginName

		if !pm.HasPage(pluginName, pagePath) {
			ctx["pluginError"] = "Page not found"
			ctx["pluginPageTitle"] = "Not Found"
			return ctx
		}

		// Build query map
		queryMap := make(map[string]any)
		for k, v := range request.URL.Query() {
			if len(v) == 1 {
				queryMap[k] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				queryMap[k] = items
			}
		}

		// Build headers map
		headerMap := make(map[string]any)
		for k, v := range request.Header {
			headerMap[strings.ToLower(k)] = v[0]
		}

		// Read body for POST requests
		var body string
		if request.Method == http.MethodPost && request.Body != nil {
			bodyBytes, err := io.ReadAll(request.Body)
			if err == nil {
				body = string(bodyBytes)
			}
		}

		pageCtx := plugin_system.PageContext{
			Path:    request.URL.String(),
			Method:  request.Method,
			Query:   queryMap,
			Headers: headerMap,
			Body:    body,
		}

		html, err := pm.HandlePage(pluginName, pagePath, pageCtx)
		if err != nil {
			ctx["pluginError"] = err.Error()
			ctx["pluginPageTitle"] = "Error"
		} else {
			ctx["pluginContent"] = html
			ctx["pluginPageTitle"] = pluginName + " - " + pagePath
		}

		return ctx
	}
}
