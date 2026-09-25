package server

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/jobs"
	"mahresources/server/api_handlers"
)

// currentCanonicalJobEventsContext revalidates the stream's original credential
// before every durable-event poll. SSE connections outlive ordinary request auth
// middleware, so a context bound only when the connection opens would preserve
// stale roles and scopes indefinitely.
type currentCanonicalJobEventsContext struct {
	appCtx  *application_context.MahresourcesContext
	request *http.Request
}

func (ctx currentCanonicalJobEventsContext) GetPublishedJobEvents(afterDelivery uint64, limit int) ([]jobs.Event, error) {
	scoped, err := ctx.current()
	if err != nil {
		return nil, err
	}
	return scoped.GetPublishedJobEvents(afterDelivery, limit)
}

// GetLiveJobProgress revalidates the credential exactly as the event poll does:
// a live progress frame is as much a read of the Job as its events are.
func (ctx currentCanonicalJobEventsContext) GetLiveJobProgress(since time.Time, sinceID string, limit int) ([]jobs.Snapshot, error) {
	scoped, err := ctx.current()
	if err != nil {
		return nil, err
	}
	return scoped.GetLiveJobProgress(since, sinceID, limit)
}

func (ctx currentCanonicalJobEventsContext) current() (*application_context.MahresourcesContext, error) {
	principal := auth.PrincipalFromContext(ctx.request.Context())
	if ctx.appCtx.AuthEnabled() {
		principal, _, _ = resolvePrincipal(ctx.appCtx, ctx.request)
		if principal == nil {
			return nil, errors.New("Job event stream authentication is no longer valid")
		}
	}
	return ctx.appCtx.WithPrincipal(principal), nil
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
// artifact-specific authorization for private job files.
func guardedFileServer(appCtx *application_context.MahresourcesContext, prefix, mountRoot string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		if privateRawStoragePath(rel) || privateRawPhysicalStoragePath(appCtx, mountRoot, rel) {
			http.NotFound(w, r)
			return
		}

		p := auth.PrincipalFromContext(r.Context())
		if !principalIsRestricted(p) {
			next.ServeHTTP(w, r)
			return
		}
		if scopedCtx(appCtx, r).FilePathInScope(rel) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

// privateRawStoragePath reserves hidden root names for application-managed
// private storage. Dedicated handlers enforce ownership and output-specific
// checks for these paths. URL.Path is decoded by net/http; normalize separators
// and dot segments before checking because FileServer applies the same semantics.
func privateRawStoragePath(rel string) bool {
	normalized := normalizedRawStoragePath(rel)
	root := strings.TrimPrefix(normalized, "/")
	root, _, _ = strings.Cut(root, "/")
	return strings.HasPrefix(root, "_") || strings.HasPrefix(root, ".")
}

func privateRawPhysicalStoragePath(appCtx *application_context.MahresourcesContext, mountRoot, rel string) bool {
	target := rawMountedPath(mountRoot, rel)
	if target == "" || appCtx == nil || appCtx.Config == nil {
		return false
	}
	fileSavePath := appCtx.Config.FileSavePath
	privateRoots := []string{appCtx.Config.PluginCommandStagingPath}
	if strings.TrimSpace(fileSavePath) != "" {
		privateRoots = append(privateRoots,
			filepath.Join(fileSavePath, "_exports"),
			filepath.Join(fileSavePath, "_imports"),
			filepath.Join(fileSavePath, jobs.JobReplayKeyFileName),
			filepath.Join(fileSavePath, "_plugin_commands"),
		)
	}
	for _, root := range privateRoots {
		if root != "" && rawPathIsWithin(target, canonicalRawPath(root)) {
			return true
		}
	}
	return rawReplayKeyPublicationTempPath(target, fileSavePath)
}

func rawReplayKeyPublicationTempPath(target, fileSavePath string) bool {
	if strings.TrimSpace(fileSavePath) == "" {
		return false
	}
	root := canonicalRawPath(fileSavePath)
	if root == "" {
		return false
	}
	rel, ok := rawRelativePathWithin(target, root)
	if !ok || rel == "." {
		return false
	}
	tempPrefix := "." + jobs.JobReplayKeyFileName + "-"
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if rawPathHasPrefixFold(component, tempPrefix) {
			return true
		}
	}
	return false
}

func normalizedRawStoragePath(rel string) string {
	rel = strings.ReplaceAll(rel, `\`, "/")
	return path.Clean("/" + strings.TrimLeft(rel, "/"))
}

func rawMountedPath(mountRoot, rel string) string {
	if strings.TrimSpace(mountRoot) == "" {
		return ""
	}
	root := canonicalRawPath(mountRoot)
	if root == "" {
		return ""
	}
	cleanRel := strings.TrimPrefix(normalizedRawStoragePath(rel), "/")
	return canonicalRawPath(filepath.Join(root, filepath.FromSlash(cleanRel)))
}

// canonicalRawPath resolves existing symlinks and preserves any missing suffix.
// Resolving the longest existing prefix also handles directory requests below a
// symlink without granting a raw alias to a configured private root.
func canonicalRawPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return filepath.Clean(value)
	}
	abs = filepath.Clean(abs)
	current := abs
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func rawPathIsWithin(target, root string) bool {
	_, ok := rawRelativePathWithin(target, root)
	return ok
}

// rawRelativePathWithin compares path components without case sensitivity.
// macOS filesystems commonly resolve case variants to the same file, while
// filepath.Rel compares spelling on some platforms and could miss that alias.
func rawRelativePathWithin(target, root string) (string, bool) {
	target = filepath.Clean(target)
	root = filepath.Clean(root)
	targetVolume := filepath.VolumeName(target)
	rootVolume := filepath.VolumeName(root)
	if !strings.EqualFold(targetVolume, rootVolume) {
		return rawRelativePathByIdentity(target, root)
	}
	targetComponents := rawPathComponents(strings.TrimPrefix(target, targetVolume))
	rootComponents := rawPathComponents(strings.TrimPrefix(root, rootVolume))
	if len(targetComponents) < len(rootComponents) {
		return rawRelativePathByIdentity(target, root)
	}
	for i, component := range rootComponents {
		if !strings.EqualFold(targetComponents[i], component) {
			return rawRelativePathByIdentity(target, root)
		}
	}
	if len(targetComponents) == len(rootComponents) {
		return ".", true
	}
	return filepath.Join(targetComponents[len(rootComponents):]...), true
}

func rawRelativePathByIdentity(target, root string) (string, bool) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", false
	}
	var suffix []string
	for current := target; ; current = filepath.Dir(current) {
		if currentInfo, statErr := os.Stat(current); statErr == nil && os.SameFile(currentInfo, rootInfo) {
			if len(suffix) == 0 {
				return ".", true
			}
			for left, right := 0, len(suffix)-1; left < right; left, right = left+1, right-1 {
				suffix[left], suffix[right] = suffix[right], suffix[left]
			}
			return filepath.Join(suffix...), true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		suffix = append(suffix, filepath.Base(current))
	}
}

func rawPathComponents(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
}

func rawPathHasPrefixFold(value, prefix string) bool {
	valueRunes := []rune(value)
	prefixRunes := []rune(prefix)
	if len(valueRunes) < len(prefixRunes) {
		return false
	}
	return strings.EqualFold(string(valueRunes[:len(prefixRunes)]), prefix)
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
