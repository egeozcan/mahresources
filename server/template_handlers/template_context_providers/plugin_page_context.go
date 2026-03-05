package template_context_providers

import (
	"io"
	"net/http"
	"net/url"
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

		// Build headers map (array when multiple values, like query params)
		headerMap := make(map[string]any)
		for k, v := range request.Header {
			if len(v) == 1 {
				headerMap[strings.ToLower(k)] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				headerMap[strings.ToLower(k)] = items
			}
		}

		// Read body for POST requests (limited to 50MB)
		var body string
		paramsMap := make(map[string]any)
		if request.Method == http.MethodPost && request.Body != nil {
			const maxBodySize = 50 << 20 // 50MB
			limited := io.LimitReader(request.Body, maxBodySize)
			bodyBytes, err := io.ReadAll(limited)
			if err == nil {
				body = string(bodyBytes)
				// Parse URL-encoded form data into params
				ct := request.Header.Get("Content-Type")
				if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || (ct == "" && len(bodyBytes) > 0) {
					if formValues, parseErr := url.ParseQuery(body); parseErr == nil {
						for k, v := range formValues {
							if len(v) == 1 {
								paramsMap[k] = v[0]
							} else {
								items := make([]any, len(v))
								for i, val := range v {
									items[i] = val
								}
								paramsMap[k] = items
							}
						}
					}
				}
			}
		}

		pageCtx := plugin_system.PageContext{
			Path:    request.URL.String(),
			Method:  request.Method,
			Query:   queryMap,
			Params:  paramsMap,
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
