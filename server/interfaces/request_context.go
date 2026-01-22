package interfaces

import "net/http"

// RequestContextSetter is an optional interface that contexts can implement
// to support HTTP request metadata capture in log entries.
// Handlers can type-assert to this interface to enable request-aware logging.
//
// Example usage in a handler:
//
//	func GetAddTagHandler(ctx interfaces.TagsWriter) http.HandlerFunc {
//	    return func(w http.ResponseWriter, r *http.Request) {
//	        // Enable request-aware logging if supported
//	        effectiveCtx := withRequestContext(ctx, request).(interfaces.TagsWriter)
//	        // ... use effectiveCtx for operations
//	    }
//	}
type RequestContextSetter interface {
	// WithRequest returns a copy of the context with the HTTP request set.
	// The returned value implements all the same interfaces as the original.
	WithRequest(r *http.Request) any
}
