package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

// LoginContextProvider builds the context for the standalone login page. It does
// not require the application context but matches the standard provider
// signature so it can be registered like any other template route.
func LoginContextProvider(_ *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := StaticTemplateCtx(request)
		ctx["pageTitle"] = "Sign in"
		ctx["next"] = request.URL.Query().Get("next")
		switch request.URL.Query().Get("error") {
		case "":
			// no error
		case "rate":
			ctx["loginError"] = "Too many login attempts. Please wait and try again."
		default:
			ctx["loginError"] = "Invalid username or password."
		}
		return ctx
	}
}
