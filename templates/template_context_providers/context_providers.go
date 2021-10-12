package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/schema"
)

var decoder = schema.NewDecoder()

func init() {
	decoder.IgnoreUnknownKeys(true)
}

func addErrContext(err error, ctx pongo2.Context) pongo2.Context {
	return ctx.Update(pongo2.Context{
		"errorMessage": err.Error(),
	})
}
