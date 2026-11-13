package shortcodes

import "context"

// credentialFields are struct fields that carry a bearer credential. [property]
// and [conditional field=] read entity fields by reflection, so json:"-" does
// not keep them out of a template; they resolve these only in a render whose
// request context says the reader may hold them (WithCredentialFieldsVisible).
// The one reachable today is Note.ShareToken: anonymous read of the note and
// its resources plus todo-state writes, outliving the account that read it
// (#107). The rest match plugin_system's credentialEntityFields, so a model that
// gains one is covered here too. Matched on the field name, at any path segment.
var credentialFields = map[string]bool{
	"ShareToken":   true,
	"TokenHash":    true,
	"CsrfToken":    true,
	"PasswordHash": true,
}

type credentialFieldsVisibleKey struct{}

// WithCredentialFieldsVisible returns a context whose renders resolve
// credential fields when visible is true. The server sets it from the request
// principal: a reader that may share a note (CanWrite) sees its share token, as
// GET /v1/note gives it. A context without it withholds them, so a render
// nobody marked fails closed.
func WithCredentialFieldsVisible(ctx context.Context, visible bool) context.Context {
	return context.WithValue(ctx, credentialFieldsVisibleKey{}, visible)
}

func credentialFieldsVisibleFrom(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	visible, _ := ctx.Value(credentialFieldsVisibleKey{}).(bool)
	return visible
}
