package server

import (
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/server/api_handlers"
	"mahresources/server/interfaces"
)

// scopedEditName / scopedEditDescription / scopedEditMeta build the per-entity
// edit handlers against a request-scoped EntityWriter, so a group-limited
// principal cannot rename/redescribe/edit-meta of an entity outside its subtree
// (the scoped DB filters the update to zero rows).
func scopedEditName[T interfaces.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer := application_context.NewEntityWriter[T](scopedCtx(appCtx, r))
		api_handlers.GetEditEntityNameHandler[T](writer, entityName)(w, r)
	}
}

func scopedEditDescription[T interfaces.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer := application_context.NewEntityWriter[T](scopedCtx(appCtx, r))
		api_handlers.GetEditEntityDescriptionHandler[T](writer, entityName)(w, r)
	}
}

func scopedEditMeta[T interfaces.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer := application_context.NewEntityWriter[T](scopedCtx(appCtx, r))
		api_handlers.GetEditMetaHandler(writer, entityName)(w, r)
	}
}

// principalIsRestricted reports whether the principal is a group-limited
// user/guest whose data access must be confined to a subtree.
func principalIsRestricted(p *auth.Principal) bool {
	return p != nil && !p.IsAdmin() && (p.IsScoped() || p.RequiresScope())
}

// denyScopedPrincipal blocks group-limited (user/guest with a subtree) principals
// from an endpoint entirely. Used for operations that have no coherent
// subtree-confined semantics — notably group import, which creates new top-level
// groups that could not be placed inside the caller's subtree. Unrestricted
// principals (admin, editor, unscoped user, and the auth-off super-user) pass
// through unchanged.
func denyScopedPrincipal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if principalIsRestricted(auth.PrincipalFromContext(r.Context())) {
			http.Error(w, "not available for group-limited accounts", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// guardedFileServer wraps a raw file server so that group-limited principals can
// only fetch files belonging to resources inside their subtree. Unrestricted
// principals (admin, system/auth-off, unscoped users) pass straight through.
func guardedFileServer(appCtx *application_context.MahresourcesContext, prefix string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := auth.PrincipalFromContext(r.Context())
		if !principalIsRestricted(p) {
			next.ServeHTTP(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		if scopedCtx(appCtx, r).FilePathInScope(rel) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

// scopedCtx returns the application context bound to the current request's
// principal, with group-subtree data scoping applied for group-limited
// principals. For admins, the system (auth-off) super-user, and unscoped users
// it returns an unrestricted context.
func scopedCtx(appCtx *application_context.MahresourcesContext, r *http.Request) *application_context.MahresourcesContext {
	return appCtx.WithPrincipal(auth.PrincipalFromContext(r.Context()))
}

// scopedAPI wraps an API handler factory so the handler runs against a
// request-scoped context. This is how read handlers (which would otherwise use
// the unscoped singleton) inherit subtree confinement. Write handlers already
// scope via withRequestContext/WithRequest inside the handler.
func scopedAPI[T any](appCtx *application_context.MahresourcesContext, make func(T) func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typed, ok := any(scopedCtx(appCtx, r)).(T)
		if !ok {
			// The scoped context is the same concrete type as appCtx, so this
			// only fails on a programming error in wiring.
			http.Error(w, "internal scoping error", http.StatusInternalServerError)
			return
		}
		make(typed)(w, r)
	}
}
