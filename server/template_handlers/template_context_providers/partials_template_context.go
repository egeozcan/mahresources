package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"net/http"
)

// partialContextKeys is an allow-list of request parameters copied into the
// autocompleter template context. Request data must never share a namespace
// with internal control keys (such as _redirect), the raw-JS inputs the
// template interpolates (parameters, excludeIds, onChange, entityId, addUrl,
// removeUrl), or the StaticTemplateCtx helpers.
var partialContextKeys = []string{
	"profile", "entity", "usage", "selectedItems", "title", "id", "elName",
	"standalone", "min", "max", "placeholder", "categoryDecoration",
}

func PartialContextProvider(_ any) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		result := StaticTemplateCtx(request)
		err := request.ParseForm()

		if err != nil {
			return addErrContext(err, result)
		}

		for _, key := range partialContextKeys {
			values := request.Form[key]
			switch len(values) {
			case 0:
			case 1:
				result[key] = values[0]
			default:
				result[key] = values
			}
		}

		return result
	}
}
