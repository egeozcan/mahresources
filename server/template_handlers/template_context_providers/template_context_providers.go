package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/schema"
	"mahresources/models/query_models"
	"net/http"
	"reflect"
	"strings"
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
	if strings.Contains(err.Error(), "record not found") {
		statusCode = http.StatusNotFound
	}
	return ctx.Update(pongo2.Context{
		"errorMessage": err.Error(),
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
