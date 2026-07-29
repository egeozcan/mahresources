package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"

	"mahresources/auth"
	"mahresources/models"
)

// AdminUsersContextProvider renders the admin user-management page: the list of
// accounts plus the assignable roles for the create form.
func AdminUsersContextProvider(ctx AccountPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		c := StaticTemplateCtx(request)
		c["pageTitle"] = "Users"
		if users, err := ctx.GetUsers(0, 0); err == nil {
			c["users"] = users
		} else {
			c["users"] = []models.User{}
			c["errorMessage"] = err.Error()
		}
		c["roles"] = models.ValidRoles
		// Finding 109: the form never stated the minimum the server enforces, so
		// the only way to learn it was to be rejected. Published from the policy
		// rather than written into the template, so the two cannot drift.
		c["minPasswordLength"] = auth.MinPasswordLength
		return c
	}
}

// AccountContextProvider renders the self-service account page for the
// authenticated user: identity, and their API tokens.
func AccountContextProvider(ctx AccountPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		c := StaticTemplateCtx(request)
		c["pageTitle"] = "Account"
		p := auth.PrincipalFromContext(request.Context())
		if p != nil && !p.SuperUser && p.UserID != 0 {
			c["account"] = p
			if tokens, err := ctx.ListApiTokens(p.UserID); err == nil {
				c["tokens"] = tokens
			}
		}
		return c
	}
}
