package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
)

// AdjacentQuery asks for the previous and next siblings of one entity, in
// created_at, id order. Series switches a resource's siblings from "same owner
// and same category" to "same Series".
type AdjacentQuery struct {
	EntityType string // "group" | "resource" | "note"
	EntityID   uint
	Series     bool
}

// AdjacentLink is one neighbour: just what the link shows.
type AdjacentLink struct {
	ID   uint
	Name string
}

// AdjacentResult holds the neighbours; a nil side has none (or none the viewer
// may see).
type AdjacentResult struct {
	Prev, Next *AdjacentLink
}

// AdjacentResolver looks up an entity's neighbours as the request's principal.
// It is injected per render with WithAdjacentResolver, like PartialResolver, so
// this package stays free of DB imports. Only surfaces whose handle carries the
// ORM subtree scope install one; every other surface renders [adjacent] as an
// "unavailable" comment.
type AdjacentResolver func(ctx context.Context, q AdjacentQuery) (AdjacentResult, error)

// ErrAdjacentBudget is returned, wrapped with a message naming the limit, by a
// resolver whose page query budget refused the lookup.
var ErrAdjacentBudget = errors.New("adjacent: budget")

type adjacentResolverKey struct{}

// WithAdjacentResolver returns a context carrying r for [adjacent].
func WithAdjacentResolver(ctx context.Context, r AdjacentResolver) context.Context {
	return context.WithValue(ctx, adjacentResolverKey{}, r)
}

func adjacentResolverFrom(ctx context.Context) AdjacentResolver {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(adjacentResolverKey{}).(AdjacentResolver)
	return r
}

// adjacentPaths are the detail-page paths [link] builds for each entity type;
// anything else is a carrier context, which has no siblings.
var adjacentPaths = map[string]string{"group": "/group", "resource": "/resource", "note": "/note"}

// adjacentTypeNames name a sibling whose own name is blank, so no link is left
// without an accessible name.
var adjacentTypeNames = map[string]string{"group": "Group", "resource": "Resource", "note": "Note"}

const defaultAdjacentLabel = "Previous and next"

// RenderAdjacentShortcode renders [adjacent]: a <nav> with a Previous and a
// Next link to the entity's siblings. A side with no sibling is left out, and
// with neither the output is empty rather than an empty landmark. An entity the
// viewer may not see and one that does not exist both render nothing.
func RenderAdjacentShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string {
	resolver := adjacentResolverFrom(reqCtx)
	if resolver == nil {
		return shortcodeComment("adjacent unavailable in this context")
	}

	// A <button> takes phrasing content only; a <nav> of links is neither.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("adjacent", "[adjacent] cannot be used inside a [reload] button face")
	}

	series := false
	if raw, ok := sc.Attrs["series"]; ok {
		switch strings.TrimSpace(raw) {
		case "true":
			series = true
		case "false":
		default:
			return shortcodeErrorMarker("adjacent", `series must be "true" or "false"`)
		}
	}
	path, isEntity := adjacentPaths[ctx.EntityType]
	if series && isEntity && ctx.EntityType != "resource" {
		return shortcodeErrorMarker("adjacent", `series="true" works only on resources`)
	}
	if !isEntity || ctx.EntityID == 0 {
		return ""
	}

	result, err := resolver(reqCtx, AdjacentQuery{EntityType: ctx.EntityType, EntityID: ctx.EntityID, Series: series})
	if err != nil {
		switch {
		case errors.Is(err, ErrAdjacentBudget):
			return shortcodeErrorMarker("adjacent", strings.TrimPrefix(err.Error(), ErrAdjacentBudget.Error()+": "))
		case errors.Is(err, context.DeadlineExceeded):
			return shortcodeErrorMarker("adjacent", "lookup timed out")
		default:
			// The resolver logged the full error; SQL text stays off the page.
			return shortcodeErrorMarker("adjacent", "lookup failed")
		}
	}
	if !meterFrom(reqCtx).alive() {
		return ""
	}
	if result.Prev == nil && result.Next == nil {
		return ""
	}

	label := sc.Attrs["label"]
	if strings.TrimSpace(label) == "" {
		label = defaultAdjacentLabel
	}
	name := func(l *AdjacentLink) string {
		if strings.TrimSpace(l.Name) == "" {
			return fmt.Sprintf("%s %d", adjacentTypeNames[ctx.EntityType], l.ID)
		}
		return l.Name
	}

	var b strings.Builder
	b.WriteString(`<nav class="shortcode-adjacent flex flex-wrap justify-between gap-2" aria-label="`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`">`)
	if l := result.Prev; l != nil {
		fmt.Fprintf(&b, `<a href="%s?id=%d" rel="prev" class="text-amber-700 hover:text-amber-900 underline"><span aria-hidden="true">←</span> Previous: %s</a>`,
			path, l.ID, html.EscapeString(name(l)))
	}
	if l := result.Next; l != nil {
		fmt.Fprintf(&b, `<a href="%s?id=%d" rel="next" class="ml-auto text-amber-700 hover:text-amber-900 underline">Next: %s <span aria-hidden="true">→</span></a>`,
			path, l.ID, html.EscapeString(name(l)))
	}
	b.WriteString(`</nav>`)
	return b.String()
}
