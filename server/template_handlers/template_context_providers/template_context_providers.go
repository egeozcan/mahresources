package template_context_providers

import (
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/schema"
	"mahresources/models/query_models"
	"reflect"
)

var decoder = schema.NewDecoder()

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.RegisterConverter(query_models.ColumnMeta{}, func(s string) reflect.Value {
		return reflect.ValueOf(query_models.ParseMeta(s))
	})
}

func addErrContext(err error, ctx pongo2.Context) pongo2.Context {
	statusCode := http.StatusInternalServerError
	errMsg := err.Error()
	if strings.Contains(errMsg, "record not found") {
		statusCode = http.StatusNotFound
	} else if strings.Contains(errMsg, "schema: error converting value") ||
		strings.Contains(errMsg, "schema: invalid path") {
		statusCode = http.StatusBadRequest
	} else if strings.Contains(errMsg, "no such column") ||
		strings.Contains(errMsg, "does not exist") {
		statusCode = http.StatusBadRequest
		errMsg = "invalid sort column"
	}
	return ctx.Update(pongo2.Context{
		"errorMessage": errMsg,
		"_statusCode":  statusCode,
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
