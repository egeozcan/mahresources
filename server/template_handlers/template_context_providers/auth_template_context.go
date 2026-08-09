package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
)

// LoginContextProvider builds the context for the standalone login page. It does
// not require the application context but matches the standard provider
// signature so it can be registered like any other template route.
func LoginContextProvider(_ any) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := StaticTemplateCtx(request)
		ctx["pageTitle"] = "Sign in"
		ctx["next"] = request.URL.Query().Get("next")
		switch request.URL.Query().Get("error") {
		case "":
			// no error
		case "rate":
			ctx["loginError"] = "Too many login attempts. Please wait and try again."
		case "busy":
			// The credential was never judged: the server could not claim the
			// user-management lock in time. Saying "invalid password" here would
			// blame the user for a transient database stall.
			ctx["loginError"] = "The server is busy. Please try signing in again."
		case "unavailable":
			// Also never judged, but not from contention — the database or
			// something it depends on is broken, so "try again" is honest advice
			// without promising it will work.
			ctx["loginError"] = "Sign-in is temporarily unavailable. Please try again."
		default:
			ctx["loginError"] = "Invalid username or password."
		}
		return ctx
	}
}
