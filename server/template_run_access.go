package server

import (
	"context"
	"fmt"
	"net/http"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/server/api_handlers"
)

// templateRunAccessFactory builds the access re-check a harness run makes around
// every Turn. It reuses the credential logic of currentJobEventsContext: with
// auth on, the original credential is resolved again (under ctx, so the lookup
// is cancelled with the run) and must still be the same account with the same
// role and scope; a changed identity is never adopted. With auth off the
// request's implicit administrator stands.
func templateRunAccessFactory(appCtx *application_context.MahresourcesContext) api_handlers.TemplateRunAccessFactory {
	return func(r *http.Request) func(ctx context.Context) (*application_context.MahresourcesContext, error) {
		original := auth.PrincipalFromContext(r.Context())
		return func(ctx context.Context) (*application_context.MahresourcesContext, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			rebased := appCtx.WithParentContext(ctx)
			principal := original
			if appCtx.AuthEnabled() {
				principal, _, _ = resolvePrincipal(rebased, r)
				// resolvePrincipal reports a failed lookup as no principal. A
				// lookup that failed because ctx ended is cancellation, not
				// revocation: the two end a run differently.
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if principal == nil {
					return nil, fmt.Errorf("%w: the credential is no longer valid", application_context.ErrTemplateRunAccessChanged)
				}
				if !sameAccess(original, principal) {
					return nil, application_context.ErrTemplateRunAccessChanged
				}
			}
			bound, err := rebased.WithPrincipalContext(ctx, principal)
			if err != nil {
				return nil, err
			}
			return bound, nil
		}
	}
}
