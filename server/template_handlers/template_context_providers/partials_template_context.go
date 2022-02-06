package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"net/http"
)

func PartialContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		result := staticTemplateCtx(request)
		err := request.ParseForm()

		if err != nil {
			return addErrContext(err, result)
		}

		for key, values := range request.Form {
			if len(values) == 1 {
				result[key] = values[0]
			} else {
				result[key] = values
			}
		}

		for key, values := range request.URL.Query() {
			if len(values) == 1 {
				result[key] = values[0]
			} else {
				result[key] = values
			}
		}

		return result
	}
}
