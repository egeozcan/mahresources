package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
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

// adjacentEnv is what WithAdjacentResolver installs: the resolver and the
// landmark names the render has used so far.
type adjacentEnv struct {
	resolve AdjacentResolver
	labels  *adjacentLabels
}

// adjacentLabels keeps every <nav> one render emits uniquely named. A page
// with [adjacent] in a per-card slot, or twice in one template, would
// otherwise hold several navigation landmarks with one name.
type adjacentLabels struct {
	mu   sync.Mutex
	used map[string]bool
}

// unique returns label when the render has not emitted it yet, and otherwise
// "label (n)" with the smallest n no emitted name holds, an author's own
// "Chapters (2)" included.
func (l *adjacentLabels) unique(label string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	name := label
	for n := 2; l.used[name]; n++ {
		name = label + " (" + strconv.Itoa(n) + ")"
	}
	l.used[name] = true
	return name
}

// WithAdjacentResolver returns a context carrying r for [adjacent]. Install it
// once per render: the landmark names are unique within it.
func WithAdjacentResolver(ctx context.Context, r AdjacentResolver) context.Context {
	return context.WithValue(ctx, adjacentResolverKey{}, adjacentEnv{resolve: r, labels: &adjacentLabels{used: map[string]bool{}}})
}

type tooltipKey struct{}

// WithinTooltip marks a render whose output the keyboard cannot enter, such as
// a hover card: the shortcodes this package adds controls for leave them out
// there. [adjacent] renders its unavailable comment and [calendar] renders
// without its Prev/Next buttons, even when a resolver or signer is installed.
func WithinTooltip(ctx context.Context) context.Context {
	return context.WithValue(ctx, tooltipKey{}, true)
}

// IsWithinTooltip reports whether this render must omit tooltip controls.
func IsWithinTooltip(ctx context.Context) bool { return withinTooltip(ctx) }

func withinTooltip(ctx context.Context) bool {
	return ctx != nil && ctx.Value(tooltipKey{}) != nil
}

func adjacentEnvFrom(ctx context.Context) (adjacentEnv, bool) {
	if ctx == nil || withinTooltip(ctx) {
		return adjacentEnv{}, false
	}
	env, ok := ctx.Value(adjacentResolverKey{}).(adjacentEnv)
	return env, ok && env.resolve != nil
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
	env, ok := adjacentEnvFrom(reqCtx)
	if !ok {
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

	result, err := env.resolve(reqCtx, AdjacentQuery{EntityType: ctx.EntityType, EntityID: ctx.EntityID, Series: series})
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

	// Whitespace collapsed as the accessible name computation does, so
	// " Meetings " and "Meetings" count as one name.
	label := strings.Join(strings.Fields(sc.Attrs["label"]), " ")
	if label == "" {
		label = defaultAdjacentLabel
	}
	label = env.labels.unique(label)
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
