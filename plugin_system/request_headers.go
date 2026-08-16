package plugin_system

import (
	"net/http"
	"strings"
)

// credentialHeaders are the request headers that authenticate the caller to
// *this* application, and are therefore never handed to a plugin.
//
// The list is exactly this app's own credential surface, as described in
// CLAUDE.md's auth section: a Bearer token, the session cookie, and the
// per-session CSRF token. proxy-authorization is included because it
// authenticates to an intermediary the operator controls and a plugin has no
// business replaying it either.
//
// It is a denylist rather than an allowlist because plugins legitimately read
// arbitrary headers — content-type, accept, and whatever custom header their
// own callers send — and an allowlist would break every one of those. The cost
// is that a credential header added to this app later must be added here too;
// an architecture test pins the list against the middleware that reads them.
var credentialHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"x-csrf-token":        true,
}

// SafeRequestHeaders renders a request's headers for a plugin, omitting the
// ones that authenticate the caller.
//
// Handing a plugin the caller's credentials makes it a confused deputy against
// the application that hosts it. A plugin serving a mah.api endpoint or a
// mah.page receives whatever the browser or CLI sent; with the Authorization
// header or session cookie in hand it can call this server's own API as that
// user — POST /v1/download/submit or /v1/resource/remote with a URL of its
// choosing. Those are operator paths, so they carry no plugin egress policy and
// fetch unrestricted: the plugin reaches any internal address, including the
// cloud metadata endpoint, and has the response persisted as a resource it can
// then read back. That defeats the manifest's network allowlist completely, and
// it needs no db:write either, since the app does the writing.
//
// Redacting at the boundary is the fix rather than policing the outbound
// request, because the credential is the capability. A plugin that cannot see
// it cannot replay it, whatever it is aimed at.
func SafeRequestHeaders(header http.Header) map[string]any {
	out := make(map[string]any, len(header))
	for name, values := range header {
		lower := strings.ToLower(name)
		if credentialHeaders[lower] {
			continue
		}
		if len(values) == 1 {
			out[lower] = values[0]
			continue
		}
		items := make([]any, len(values))
		for i, v := range values {
			items[i] = v
		}
		out[lower] = items
	}
	return out
}

// credentialParams are the request PARAMETER names that carry a credential.
//
// The CSRF token is accepted three ways — the X-CSRF-Token header, a
// csrf_token query parameter (native multipart upload forms, which cannot set
// a header), and a csrf_token urlencoded form field (see server/csrf.go). A
// redaction that covers only the header spelling leaves the invariant it claims
// ("a plugin never sees the caller's credentials") false for the other two, and
// the query string is handed to plugins in full.
var credentialParams = map[string]bool{
	"csrf_token": true,
}

// StripCredentialParams removes credential-carrying entries from a query or
// form map built for a plugin. It mutates and returns the map it was given.
func StripCredentialParams(values map[string]any) map[string]any {
	for name := range values {
		if credentialParams[strings.ToLower(name)] {
			delete(values, name)
		}
	}
	return values
}
