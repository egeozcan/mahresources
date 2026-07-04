package shortcodes

import (
	"context"
	"encoding/json"
	"strings"
)

// PluginRenderer is a callback that renders a plugin shortcode.
// It receives the plugin name (e.g., "test" from "plugin:test:widget"),
// the parsed shortcode, and the entity context.
// Returns rendered HTML or an error (in which case the original text is preserved).
type PluginRenderer func(pluginName string, sc Shortcode, ctx MetaShortcodeContext) (string, error)

// QueryOptions carries the non-query parameters of a QueryExecutor call.
// Grouping the tail into a struct keeps the callback signature stable as new
// knobs (e.g. WantTotal) are added.
type QueryOptions struct {
	// SavedName names a saved MRQL query to resolve when query is empty.
	SavedName string
	// Params binds $name placeholders (nil/empty when none).
	Params map[string]string
	// Limit caps the number of returned items.
	Limit int
	// Buckets controls the grouping bucket count.
	Buckets int
	// ScopeGroupID restricts results to a group subtree (0 means global/no filter).
	ScopeGroupID uint
	// WantTotal asks the executor to also compute QueryResult.Total, the true
	// row count ignoring Limit. Off by default because it runs a second query.
	WantTotal bool
}

// QueryExecutor is a callback that executes an MRQL query and returns results.
// query is the raw MRQL expression; opts carries the saved-query name, param
// bindings, limit/bucket caps, scope, and the total-count flag.
type QueryExecutor func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error)

// QueryResult holds the output of a QueryExecutor call.
type QueryResult struct {
	EntityType string
	Mode       string
	Items      []QueryResultItem
	Rows       []map[string]any
	Groups     []QueryResultGroup

	// EffectiveQuery is the MRQL text actually executed (resolved from a saved
	// query when SavedName was used). Empty for a bare/failed execution.
	EffectiveQuery string
	// SavedID is the resolved saved-query ID when SavedName was used (0 otherwise).
	SavedID uint
	// Total is the true row count ignoring Limit, populated only when
	// QueryOptions.WantTotal was set; nil otherwise.
	Total *int64
}

// QueryResultItem represents a single entity returned by a query.
type QueryResultItem struct {
	EntityType       string
	EntityID         uint
	Entity           any
	Meta             json.RawMessage
	MetaSchema       string
	CustomMRQLResult string
	CustomCSS        string // category-level CustomCSS, injected once per category as a <style> block
	CategoryID       uint   // category/type ID, used to dedupe CustomCSS emission
	ScopeGroupID     uint   // precomputed: owning group ID (or sentinel for ownerless)
	ParentGroupID    uint   // precomputed: owner's owner ID
	RootGroupID      uint   // precomputed: root of ownership chain
}

// QueryResultGroup is a bucket of QueryResultItems sharing a common key.
type QueryResultGroup struct {
	Key   map[string]any
	Items []QueryResultItem
}

// maxRecursionDepth limits how deeply shortcodes may nest inside each
// other's output to prevent runaway recursive expansion.
const maxRecursionDepth = 10

// PartialResolver resolves a [partial name="…"] reference to its raw template
// content. found is false when no partial with that name exists. It is injected
// per-request via WithPartialResolver, mirroring how the MRQL render cache is
// threaded on the request context, so the shortcodes package stays free of DB
// imports. A nil/absent resolver makes every [partial] render its not-found
// comment.
type PartialResolver func(name string) (content string, found bool)

type partialResolverKey struct{}

// WithPartialResolver returns a context carrying r, consulted by [partial]
// expansion during Process. Callers build the resolver once per page render
// (with a request-scoped cache) and attach it here.
func WithPartialResolver(ctx context.Context, r PartialResolver) context.Context {
	return context.WithValue(ctx, partialResolverKey{}, r)
}

func partialResolverFrom(ctx context.Context) PartialResolver {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(partialResolverKey{}).(PartialResolver)
	return r
}

// Process parses shortcodes in input and replaces them with rendered HTML.
// Built-in "meta" and "property" shortcodes are handled directly.
// "mrql" shortcodes use the provided executor callback (left as-is if nil).
// Plugin shortcodes (starting with "plugin:") use the provided renderer callback.
// If renderer is nil, plugin shortcodes are left as-is.
func Process(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string {
	return processWithDepth(reqCtx, input, ctx, renderer, executor, 0)
}

func processWithDepth(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if depth >= maxRecursionDepth {
		// Emit the content as-is (it may be meaningful text), but when it still
		// contains unexpanded shortcodes, append a comment so an author reading
		// the page source sees why expansion stopped here.
		if len(ParseWithBlocks(input)) > 0 {
			return input + shortcodeComment("shortcode depth limit reached")
		}
		return input
	}

	shortcodes := ParseWithBlocks(input)
	if len(shortcodes) == 0 {
		return input
	}

	var b strings.Builder
	b.Grow(len(input) * 2)
	lastEnd := 0

	for _, sc := range shortcodes {
		b.WriteString(input[lastEnd:sc.Start])

		var replacement string

		switch {
		case sc.Name == "conditional":
			replacement = RenderConditionalShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "meta":
			replacement = RenderMetaShortcode(sc, ctx)
		case sc.Name == "property":
			replacement = RenderPropertyShortcode(sc, ctx)
		case sc.Name == "mrql":
			if executor != nil && depth < maxRecursionDepth {
				replacement = RenderMRQLShortcode(reqCtx, sc, ctx, renderer, executor, depth)
			} else {
				// No executor wired (a context that deliberately omits one, e.g.
				// share-page rendering) — leave a comment rather than leaking the
				// raw shortcode text.
				replacement = shortcodeComment("mrql unavailable in this context")
			}
		case sc.Name == "link":
			replacement = RenderLinkShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "partial":
			replacement = RenderPartialShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "each":
			replacement = RenderEachShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "item":
			// [item] only has meaning inside an [each] block, where the each
			// handler substitutes it before this dispatch runs. A bare [item]
			// (outside any [each]) renders empty.
			replacement = ""
		case strings.HasPrefix(sc.Name, "plugin:"):
			if renderer != nil {
				parts := strings.SplitN(sc.Name, ":", 3)
				if len(parts) == 3 {
					html, err := renderer(parts[1], sc, ctx)
					if err == nil {
						replacement = html
						// Post-plugin expansion for block shortcodes
						if sc.IsBlock && depth+1 < maxRecursionDepth {
							replacement = processWithDepth(reqCtx, replacement, ctx, renderer, executor, depth+1)
						}
					} else {
						// Plugin rendering failed — an actionable, author-facing
						// error. Surface the shortcode name inline with the error
						// in the title attribute.
						replacement = shortcodeErrorMarker(sc.Name, err.Error())
					}
				} else {
					replacement = shortcodeErrorMarker(sc.Name, "malformed plugin shortcode name (expected plugin:<name>:<shortcode>)")
				}
			} else {
				// No plugin renderer wired in this context — comment, not a leak.
				replacement = shortcodeComment("plugin unavailable in this context")
			}
		}

		b.WriteString(replacement)
		lastEnd = sc.End
	}

	b.WriteString(input[lastEnd:])

	return b.String()
}
