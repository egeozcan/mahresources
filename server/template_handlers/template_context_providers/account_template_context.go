package template_context_providers

import (
	"encoding/json"
	"errors"
	"fmt"
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
			c["userDeleteMessages"] = userDeleteMessages(ctx, users)
		} else {
			c["users"] = []models.User{}
			c["errorMessage"] = err.Error()
		}
		c["roles"] = models.ValidRoles
		// Finding 109: the form never stated the minimum the server enforces, so
		// the only way to learn it was to be rejected. Published from the policy
		// rather than written into the template, so the two cannot drift.
		c["minPasswordLength"] = auth.MinPasswordLength
		c["maxPasswordBytes"] = auth.MaxPasswordBytes
		c["formSubmitted"] = request.URL.Query().Has("error")
		return c
	}
}

// UnfinishedJobCounter counts the unfinished Jobs that act as each account. It is
// optional: a context that cannot count them still renders the page, with the
// delete dialog saying nothing about Jobs.
type UnfinishedJobCounter interface {
	UnfinishedJobCounts() (map[uint]int64, error)
}

// userDeleteMessages is each account's delete confirmation, keyed by its id. Deleting an account leaves the Jobs that act as it with nobody
// to run as, so the dialog says how many there are and what becomes of them.
func userDeleteMessages(ctx any, users []models.User) map[uint]string {
	counts := map[uint]int64{}
	if counter, ok := ctx.(UnfinishedJobCounter); ok {
		if read, err := counter.UnfinishedJobCounts(); err == nil {
			counts = read
		}
	}
	messages := make(map[uint]string, len(users))
	for _, user := range users {
		message := fmt.Sprintf("Delete user %s? This also destroys their tokens and sessions, and clears them as the creator of everything they made.", user.Username)
		switch count := counts[user.ID]; {
		case count == 1:
			message += " One of their jobs has not finished: if it is still waiting or scheduled it will fail rather than run, and if it is already running it may still finish."
		case count > 1:
			message += fmt.Sprintf(" %d of their jobs have not finished: those still waiting or scheduled will fail rather than run, and those already running may still finish.", count)
		}
		messages[user.ID] = message
	}
	return messages
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
		c["maxPasswordBytes"] = auth.MaxPasswordBytes
		c["formSubmitted"] = request.URL.Query().Has("error")
		c["displayNameSubmitted"] = request.URL.Query().Has("displayName")
		c["submittedDisplayName"] = request.URL.Query().Get("displayName")
		c["scopeGroupIdSubmitted"] = request.URL.Query().Has("scopeGroupId")
		c["submittedScopeGroupId"] = request.URL.Query().Get("scopeGroupId")
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
		c["minPasswordLength"] = auth.MinPasswordLength
		c["maxPasswordBytes"] = auth.MaxPasswordBytes
		if p != nil && !p.SuperUser && p.UserID != 0 {
			c["account"] = p
			if tokens, err := ctx.ListApiTokens(p.UserID); err == nil {
				c["tokens"] = tokens
				// encoding/json escapes '<', so a token label containing </script>
				// cannot terminate the inert JSON data element in account.tpl.
				if encoded, marshalErr := json.Marshal(tokens); marshalErr == nil {
					c["tokensJSON"] = string(encoded)
				}
			}
		}
		return c
	}
}
