package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
)

// capability is the access level a request requires. Levels are not a strict
// hierarchy across roles (taxonomy and system are both admin-only); the mapping
// from level to role lives in principalSatisfies.
type capability int

const (
	// capRead — any authenticated principal, including guests. Group-subtree
	// data scoping for users/guests is enforced separately at the data layer.
	capRead capability = iota
	// capWrite — base entity writes: resources, notes, groups, tags.
	// Granted to admin, editor, and user (not guest).
	capWrite
	// capEditor — editor-level writes: relations, relation/note types, series,
	// saved queries, group import/export, note sharing, plugin-action execution.
	// Granted to admin and editor (not user).
	capEditor
	// capTaxonomy — create/edit Categories and Resource Categories. Admin only;
	// editors are explicitly excluded per the role spec.
	capTaxonomy
	// capSystem — system settings, plugin management, user administration.
	// Admin only.
	capSystem
)

// principalSatisfies reports whether p is allowed the given capability.
func principalSatisfies(p *auth.Principal, c capability) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin() { // admin or super-user (auth disabled) — full access
		return true
	}
	switch c {
	case capRead:
		return true
	case capWrite:
		return p.CanWrite()
	case capEditor:
		return p.CanEditorWrite()
	default: // capTaxonomy, capSystem — admin-only, already returned above
		return false
	}
}

// requiredCapability classifies a request into the capability it requires.
//
// The classification is deliberately centralized and documented here so the
// policy is auditable in one place. The enforcement that matters most is on the
// /v1/ API routes (the real mutation surface). Template form pages are largely
// left readable; their submit endpoints under /v1/ are what gate the action, so
// a non-privileged user may load a form but cannot complete the write.
func requiredCapability(method, rawPath string) capability {
	// Normalize the dual-response suffixes used by template routes.
	path := strings.TrimSuffix(strings.TrimSuffix(rawPath, ".json"), ".body")

	safe := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || isReadViaPost(path)

	switch {
	// Session/account endpoints: any authenticated principal (so guests can log
	// out and manage their own password / API tokens).
	case path == "/login" || path == "/logout" || path == "/account" ||
		strings.HasPrefix(path, "/v1/auth/") || strings.HasPrefix(path, "/v1/account/"):
		return capRead

	// System administration — restricted for reads and writes alike.
	case isSystemPath(path):
		return capSystem

	// Group import/export UI pages are write-oriented: viewable only by principals
	// who may actually import/export (users and up), not guests.
	case path == "/admin/export" || path == "/admin/import":
		return capWrite

	// Category / Resource Category management — admin-only writes, open reads.
	case isTaxonomyPath(path):
		if safe {
			return capRead
		}
		return capTaxonomy

	// Editor-level operations — admin/editor writes, open reads.
	case isEditorPath(path):
		if safe {
			return capRead
		}
		return capEditor

	// Everything else: reads open to all; writes need base write access.
	default:
		if safe {
			return capRead
		}
		return capWrite
	}
}

// isReadViaPost lists POST endpoints that are semantically reads (query
// execution), so read-only principals may use them.
func isReadViaPost(path string) bool {
	switch path {
	case "/v1/mrql", "/v1/mrql/validate", "/v1/mrql/complete",
		"/v1/query/run", "/v1/mrql/saved/run", "/v1/search",
		"/v1/groups/export/estimate":
		return true
	default:
		return false
	}
}

// isSystemPath matches admin-only system surfaces: settings, server/data stats,
// plugin management, and user administration (the latter added in a later phase).
func isSystemPath(path string) bool {
	switch path {
	case "/admin/overview", "/admin/settings", "/plugins/manage", "/admin/users":
		return true
	case "/v1/plugin/enable", "/v1/plugin/disable", "/v1/plugin/settings", "/v1/plugin/purge-data", "/v1/plugins/manage":
		return true
	}
	switch {
	case strings.HasPrefix(path, "/v1/admin/server-stats"),
		strings.HasPrefix(path, "/v1/admin/data-stats"),
		strings.HasPrefix(path, "/v1/admin/settings"):
		return true
	case strings.HasPrefix(path, "/v1/user"): // /v1/user, /v1/users, /v1/user/delete (admin user management)
		return true
	}
	return false
}

// isTaxonomyPath matches Category and Resource Category endpoints.
func isTaxonomyPath(path string) bool {
	return strings.HasPrefix(path, "/v1/category") ||
		strings.HasPrefix(path, "/v1/resourceCategory")
}

// isEditorPath matches editor-level operations. Reads of these surfaces remain
// open (handled by the caller's `safe` check); only writes require capEditor.
//
// Note sharing, group import/export, and plugin-action execution are
// deliberately NOT here: per product decision, plain users may also perform them
// (subject to group-subtree scoping), so they fall through to capWrite.
func isEditorPath(path string) bool {
	switch {
	// Admin shares dashboard (bulk management view), distinct from per-note sharing.
	case path == "/admin/shares", path == "/v1/admin/shares/bulk-revoke":
		return true
	// Relations and relation types.
	case strings.HasPrefix(path, "/v1/relation"):
		return true
	// Note types.
	case strings.HasPrefix(path, "/v1/noteType"), strings.HasPrefix(path, "/v1/note/noteType"):
		return true
	// Series.
	case strings.HasPrefix(path, "/v1/series"), path == "/v1/seriesList", path == "/v1/resource/removeSeries":
		return true
	// Saved queries (creating/editing/deleting). Running is read-via-POST above.
	case strings.HasPrefix(path, "/v1/query"), strings.HasPrefix(path, "/v1/mrql/saved"):
		return true
	default:
		return false
	}
}

// withAuthorization enforces role-based access using requiredCapability. It runs
// after withAuthentication, so the principal (if any) is already on the context.
// When auth is disabled it is a no-op (the super-user principal satisfies all).
func withAuthorization(appCtx *application_context.MahresourcesContext, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !appCtx.AuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		// Public, unauthenticated paths need no capability.
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		p := auth.PrincipalFromContext(r.Context())
		if principalSatisfies(p, requiredCapability(r.Method, r.URL.Path)) {
			next.ServeHTTP(w, r)
			return
		}
		denyAccess(w, r)
	})
}

func denyAccess(w http.ResponseWriter, r *http.Request) {
	if wantsJSONResponse(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "insufficient permissions"})
		return
	}
	http.Error(w, "Forbidden: insufficient permissions for your role", http.StatusForbidden)
}
