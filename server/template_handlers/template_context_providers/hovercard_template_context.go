package template_context_providers

import (
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

// HoverCardContextProvider backs GET /hovercard?type=<group|resource|note>&id=<n>,
// returning a compact-card HTML fragment for the hover-preview popover. The
// entity is loaded through the *scoped* context passed in, so a group-limited
// principal that hovers a link to an out-of-subtree entity gets a fail-closed
// "unavailable" fragment (GetGroup/GetResource/GetNote return ErrRecordNotFound
// for anything outside the allowed subtree) rather than a preview.
//
// The Category/ResourceCategory/NoteType relation is preloaded by each getter,
// so the fragment's CustomAvatar / CustomSummary shortcodes resolve.
func HoverCardContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		q := request.URL.Query()
		entityType := q.Get("type")
		id64, err := strconv.ParseUint(q.Get("id"), 10, 64)

		base := pongo2.Context{
			"hoverType":   entityType,
			"hoverEntity": nil,
		}
		if err != nil || id64 == 0 {
			return base
		}
		id := uint(id64)

		switch entityType {
		case "group":
			if g, err := context.GetGroup(id); err == nil {
				base["hoverEntity"] = g
			}
		case "resource":
			if r, err := context.GetResource(id); err == nil {
				base["hoverEntity"] = r
			}
		case "note":
			if n, err := context.GetNote(id); err == nil {
				base["hoverEntity"] = n
			}
		}
		return base
	}
}
