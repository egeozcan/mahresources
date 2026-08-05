package template_context_providers

import (
	"errors"
	"net/http"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
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

// AdminUserEditContextProvider renders /admin/users/edit?id=N.
//
// Product decision 107: the three routine operations — change role, reset
// password, disable — used to require *deleting* the account, which destroys its
// tokens and sessions and nulls CreatedByUserId across fifteen tables. That is a
// destructive workaround for routine administration, and it was the only thing the
// list page offered.
//
// A separate page rather than inline rows, matching every other editable entity in
// the app (/group/edit, /tag/edit, /category/edit, …): the same shape, so the same
// rejection path — HandleFormErrorWithStatus redirects back here with the typed
// values and an `error` param, which the template's banner renders.
func AdminUserEditContextProvider(ctx AccountPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		c := StaticTemplateCtx(request)
		c["pageTitle"] = "Edit user"
		c["roles"] = models.ValidRoles
		c["minPasswordLength"] = auth.MinPasswordLength
		// formCancelURL only answers for two-segment /X/new and /X/edit paths, so
		// this three-segment one has to say where Cancel goes itself. Finding 129
		// is about edit forms with no way out, and it applies here too.
		c["cancelUrl"] = "/admin/users"

		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, c)
		}
		// addMessageErrContext, not a bare c["errorMessage"]: the status code is what
		// makes render_template render error.tpl instead of this page. Setting only
		// the message answers 200 with an alert *and* an empty edit form.
		if query.ID == 0 {
			return addMessageErrContext("No user id given.", http.StatusBadRequest, c)
		}

		user, err := ctx.GetUser(query.ID)
		if err != nil {
			// GetUser translates gorm.ErrRecordNotFound into ErrUserNotFound, whose
			// message is "user not found" — and addErrContext keys on the literal
			// "record not found", so it would answer 500 for an id that simply does
			// not exist. Every other entity page answers 404 there
			// (entity-not-found-returns-404.spec.ts), and so does this one.
			if errors.Is(err, application_context.ErrUserNotFound) {
				return addMessageErrContext("That user doesn't exist, or the account has been deleted.",
					http.StatusNotFound, c)
			}
			return addErrContext(err, c)
		}
		// Not `user`: base.tpl and the plugin context already publish request-scoped
		// names, and a generic one here is how a template variable quietly shadows
		// something else. `editUser` says which user this is.
		c["editUser"] = user
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
