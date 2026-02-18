package api_handlers

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/schema"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
)

var decoder = schema.NewDecoder()

// withRequestContext enables request-aware logging if the context supports it.
// It checks if the context implements RequestContextSetter and returns a
// request-scoped context copy. If not supported, returns the original context.
//
// Usage in handlers:
//
//	effectiveCtx := withRequestContext(ctx, request).(interfaces.TagsWriter)
func withRequestContext(ctx any, r *http.Request) any {
	if setter, ok := ctx.(interfaces.RequestContextSetter); ok {
		return setter.WithRequest(r)
	}
	return ctx
}

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.RegisterConverter(query_models.ColumnMeta{}, func(s string) reflect.Value {
		return reflect.ValueOf(query_models.ParseMeta(s))
	})
}

func tryFillStructValuesFromRequest(dst any, request *http.Request) error {
	contentTypeHeader := request.Header.Get("Content-type")

	if strings.HasPrefix(contentTypeHeader, constants.JSON) {
		return json.NewDecoder(request.Body).Decode(dst)
	}

	if strings.HasPrefix(contentTypeHeader, constants.UrlEncodedForm) {
		if err := request.ParseForm(); err != nil {
			return err
		}
		return decoder.Decode(dst, request.PostForm)
	}

	if strings.HasPrefix(contentTypeHeader, constants.MultiPartForm) {
		if err := request.ParseMultipartForm(int64(32) << 20); err != nil {
			return err
		}
		return decoder.Decode(dst, request.PostForm)
	}

	return decoder.Decode(dst, request.URL.Query())
}
