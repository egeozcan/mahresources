package server

import (
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/constants"
)

// withJSONBodyLimit bounds application/json request bodies when a positive limit
// is configured (MaxJSONBodySize). It is keyed on Content-Type so multipart
// uploads (governed by -max-upload-size) and urlencoded forms are untouched.
// Default 0 disables it, preserving the historical unlimited behaviour.
//
// The wrap is lazy: http.MaxBytesReader only enforces the cap when a handler
// actually reads the body, so it does not change behaviour for endpoints that
// ignore the body, and it does not interfere with the CSRF middleware (which
// never reads JSON bodies) or with the per-upload size limits applied on the
// multipart upload paths.
func withJSONBodyLimit(appCtx *application_context.MahresourcesContext, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limit := appCtx.Config.MaxJSONBodySize; limit > 0 && r.Body != nil &&
			strings.HasPrefix(r.Header.Get("Content-Type"), constants.JSON) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
