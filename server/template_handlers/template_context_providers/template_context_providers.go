package template_context_providers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/schema"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"reflect"
)

var decoder = schema.NewDecoder()

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.RegisterConverter(query_models.ColumnMeta{}, func(s string) reflect.Value {
		return reflect.ValueOf(query_models.ParseMeta(s))
	})
}

// addErrContext turns an error raised while building a page into the error page
// the reader sees. It maps the driver's vocabulary to a message about the thing
// the reader asked for, and attaches somewhere to go — findings 119/132/135/111,
// where "record not found" was the entire page body and nothing linked anywhere.
//
// The entity is derived from the request path, which StaticTemplateCtx already
// puts in the context under "url". That keeps the signature — and its 157 call
// sites — unchanged; a provider that builds its context by hand simply falls
// back to the generic message and the dashboard link.
func addErrContext(err error, ctx pongo2.Context) pongo2.Context {
	statusCode := http.StatusInternalServerError
	errMsg := err.Error()
	surface, hasSurface := surfaceForPath(requestPathFromContext(ctx))

	if strings.Contains(errMsg, "record not found") {
		statusCode = http.StatusNotFound
		if hasSurface {
			errMsg = surface.notFoundMessage()
		} else {
			errMsg = "That page doesn't exist, or it has been deleted."
		}
	} else if strings.Contains(errMsg, "schema: error converting value") ||
		strings.Contains(errMsg, "schema: invalid path") {
		statusCode = http.StatusBadRequest
		errMsg = http_utils.SanitizeSchemaError(err).Error()
	} else if strings.Contains(errMsg, "no such column") ||
		strings.Contains(errMsg, "does not exist") {
		statusCode = http.StatusBadRequest
		errMsg = "invalid sort column"
	}

	recovery := []RecoveryLink{DashboardRecovery}
	if hasSurface {
		recovery = surface.recoveryLinks()
	}

	return ctx.Update(pongo2.Context{
		"errorMessage":  errMsg,
		"_statusCode":   statusCode,
		"pageTitle":     fmt.Sprintf("Error %d", statusCode),
		"errorRecovery": recovery,
	})
}

// addMessageErrContext is addErrContext for a failure the provider can describe
// itself, where the driver has no error worth translating — a missing required
// query parameter, say. The status code is chosen by the caller.
func addMessageErrContext(message string, statusCode int, ctx pongo2.Context) pongo2.Context {
	recovery := []RecoveryLink{DashboardRecovery}
	if surface, ok := surfaceForPath(requestPathFromContext(ctx)); ok {
		recovery = surface.recoveryLinks()
	}
	return ctx.Update(pongo2.Context{
		"errorMessage":  message,
		"_statusCode":   statusCode,
		"pageTitle":     fmt.Sprintf("Error %d", statusCode),
		"errorRecovery": recovery,
	})
}

type SortColumn struct {
	Name  string
	Value string
}

type SelectOption struct {
	Link   string
	Title  string
	Active bool
}
