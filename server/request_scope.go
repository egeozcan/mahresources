package server

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/jobs"
	"mahresources/server/api_handlers"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers"
)

// currentJobEventsContext revalidates a job event stream's original credential
// each time the stream reads. SSE connections outlive ordinary request auth
// middleware, so a context bound only when the connection opens would preserve
// stale roles and scopes indefinitely. The canonical stream reads through it
// before every durable-event poll; the legacy streams before every frame.
//
// Resolving the credential is a session or token lookup; binding a group-limited
// principal also materializes its whole subtree. The binding is therefore kept
// while the resolved access is unchanged and rebuilt when it changes. The subtree
// it holds is a snapshot, which these streams never consult: a Job's visibility
// is its owner or an administrator, not a group.
type currentJobEventsContext struct {
	appCtx  *application_context.MahresourcesContext
	request *http.Request

	mu      sync.Mutex
	boundAs *auth.Principal
	bound   *application_context.MahresourcesContext
}

func newCurrentJobEventsContext(appCtx *application_context.MahresourcesContext, request *http.Request) *currentJobEventsContext {
	return &currentJobEventsContext{appCtx: appCtx, request: request}
}

func (ctx *currentJobEventsContext) GetPublishedJobEvents(filter jobs.EventFilter, afterDelivery uint64, limit int) ([]jobs.Event, error) {
	scoped, err := ctx.current()
	if err != nil {
		return nil, err
	}
	return scoped.GetPublishedJobEvents(filter, afterDelivery, limit)
}

// GetJobEventSequenceHead revalidates the credential as the event poll does,
// though the head it reads is the database's rather than the principal's.
func (ctx *currentJobEventsContext) GetJobEventSequenceHead() (uint64, error) {
	scoped, err := ctx.current()
	if err != nil {
		return 0, err
	}
	return scoped.GetJobEventSequenceHead()
}

// GetPublishedJobEventHead revalidates the credential as the event poll does: the
// head is read as the principal the stream is delivering to.
func (ctx *currentJobEventsContext) GetPublishedJobEventHead(filter jobs.EventFilter) (uint64, error) {
	scoped, err := ctx.current()
	if err != nil {
		return 0, err
	}
	return scoped.GetPublishedJobEventHead(filter)
}

// GetLiveJobProgress revalidates the credential exactly as the event poll does:
// a live progress frame is as much a read of the Job as its events are.
func (ctx *currentJobEventsContext) GetLiveJobProgress(filter jobs.EventFilter, since time.Time, limit int) ([]jobs.Snapshot, error) {
	scoped, err := ctx.current()
	if err != nil {
		return nil, err
	}
	return scoped.GetLiveJobProgress(filter, since, limit)
}

// CurrentJobEvents is the legacy streams' read of the same credential.
func (ctx *currentJobEventsContext) CurrentJobEvents() (api_handlers.JobEventsContext, error) {
	scoped, err := ctx.current()
	if err != nil {
		return nil, err
	}
	return scoped, nil
}

func (ctx *currentJobEventsContext) current() (*application_context.MahresourcesContext, error) {
	principal := auth.PrincipalFromContext(ctx.request.Context())
	if ctx.appCtx.AuthEnabled() {
		principal, _, _ = resolvePrincipal(ctx.appCtx, ctx.request)
		if principal == nil {
			return nil, errors.New("job event stream authentication is no longer valid")
		}
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.bound == nil || !sameAccess(ctx.boundAs, principal) {
		ctx.bound = ctx.appCtx.WithPrincipal(principal)
		ctx.boundAs = principal
	}
	return ctx.bound, nil
}

// sameAccess reports whether two principals are the same account with the same
// role and scope, which is everything a binding depends on.
func sameAccess(a, b *auth.Principal) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.UserID != b.UserID || a.Role != b.Role || a.SuperUser != b.SuperUser {
		return false
	}
	if a.ScopeGroupID == nil || b.ScopeGroupID == nil {
		return a.ScopeGroupID == b.ScopeGroupID
	}
	return *a.ScopeGroupID == *b.ScopeGroupID
}

// scopedEditName / scopedEditDescription / scopedEditMeta build the per-entity
// edit handlers against a request-scoped EntityWriter, so a group-limited
// principal cannot rename/redescribe/edit-meta of an entity outside its subtree
// (the scoped DB filters the update to zero rows).
func scopedEditName[T contracts.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer := application_context.NewEntityWriter[T](scopedCtx(appCtx, r))
		api_handlers.GetEditEntityNameHandler[T](writer, entityName)(w, r)
	}
}

func scopedEditDescription[T contracts.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer := application_context.NewEntityWriter[T](scopedCtx(appCtx, r))
		api_handlers.GetEditEntityDescriptionHandler[T](writer, entityName)(w, r)
	}
}

// scopedPreviewDescription serves POST /v1/<entity>/previewDescription against
// the request's principal-scoped context, so loading the entity and any [mrql]
// in the preview are confined like the entity's detail page.
func scopedPreviewDescription[T contracts.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := scopedCtx(appCtx, r)
		api_handlers.GetPreviewDescriptionHandler(ctx, entityName, application_context.NewEntityWriter[T](ctx).Exists, appCtx.AuthEnabled())(w, r)
	}
}

// renderedTextBlocks serves GET /v1/note/renderedTextBlocks?noteId=N: the
// note's text blocks rendered for display, read through the request's
// principal-scoped context so a note outside a group-limited principal's
// subtree is not found and its shortcodes see only the subtree.
func renderedTextBlocks(appCtx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := scopedCtx(appCtx, r)
		note, err := ctx.GetNote(http_utils.GetUIntQueryParameter(r, "noteId", 0))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}
		html, err := template_handlers.RenderTextBlocks(ctx, r, note, appCtx.AuthEnabled())
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}
}

func scopedEditMeta[T contracts.BasicEntityReader](appCtx *application_context.MahresourcesContext, entityName string) http.HandlerFunc {
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

// guardedFileServer reserves private storage directories from raw file access,
// then ensures group-limited principals can only fetch files belonging to
// resources inside their subtree. Dedicated download handlers apply the
// artifact-specific authorization for private job files. storage is the
// mount's StorageLocation key ("" for /files/), so a path is matched on the
// mount it was requested from.
func guardedFileServer(appCtx *application_context.MahresourcesContext, prefix, storage, mountRoot string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		if appCtx.IsPrivateStoragePath(mountRoot, rel) {
			http.NotFound(w, r)
			return
		}

		p := auth.PrincipalFromContext(r.Context())
		if !principalIsRestricted(p) {
			next.ServeHTTP(w, r)
			return
		}
		if scopedCtx(appCtx, r).FilePathInScope(storage, rel) {
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

// scopedMRQLAPI avoids the generic ORM subtree allow-list for MRQL-only
// handlers. MRQL applies the principal's subtree as a recursive SQL CTE and the
// request context still carries cancellation and actor identity.
func scopedMRQLAPI[T any](appCtx *application_context.MahresourcesContext, make func(T) func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.PrincipalFromContext(r.Context())
		typed, ok := any(appCtx.WithMRQLPrincipal(r.Context(), p)).(T)
		if !ok {
			http.Error(w, "internal scoping error", http.StatusInternalServerError)
			return
		}
		make(typed)(w, r)
	}
}
