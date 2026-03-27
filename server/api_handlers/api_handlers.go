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

// formHasField reports whether a form-encoded request explicitly included the
// named field.  For JSON requests it always returns false (JSON partial-update
// semantics rely on zero-value checks instead).
func formHasField(request *http.Request, field string) bool {
	ct := request.Header.Get("Content-type")
	if strings.HasPrefix(ct, constants.UrlEncodedForm) || strings.HasPrefix(ct, constants.MultiPartForm) {
		if request.PostForm != nil {
			_, ok := request.PostForm[field]
			return ok
		}
	}
	return false
}

func tryFillStructValuesFromRequest(dst any, request *http.Request) error {
	contentTypeHeader := request.Header.Get("Content-type")

	if strings.HasPrefix(contentTypeHeader, constants.JSON) {
		// KAN-22: No JSON body size limit is by design. Mahresources is a personal information
		// management application designed to run on private/internal networks with no authentication
		// layer. All users are trusted, and large JSON payloads are expected for bulk operations.
		return json.NewDecoder(request.Body).Decode(dst) // KAN-22: no size limit by design — internal network app, all users trusted
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

// isEmptyResourceSearchQuery returns true when none of the meaningful search
// fields are populated, which means the caller did not provide any criteria to
// narrow down the result set.
func isEmptyResourceSearchQuery(q *query_models.ResourceSearchQuery) bool {
	return q.Name == "" && q.Description == "" && q.ContentType == "" &&
		q.OwnerId == 0 && q.ResourceCategoryId == 0 &&
		len(q.Groups) == 0 && len(q.Tags) == 0 && len(q.Notes) == 0 && len(q.Ids) == 0 &&
		q.OriginalName == "" && q.OriginalLocation == "" && q.Hash == "" &&
		q.MinWidth == 0 && q.MinHeight == 0 && q.MaxWidth == 0 && q.MaxHeight == 0 &&
		len(q.MetaQuery) == 0 &&
		q.CreatedBefore == "" && q.CreatedAfter == "" &&
		q.UpdatedBefore == "" && q.UpdatedAfter == ""
}
