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

// QueryExecutor is a callback that executes an MRQL query and returns results.
// query is the raw MRQL expression, savedName is an optional saved query name,
// params binds $name placeholders (nil/empty when none), limit caps the number
// of returned items, buckets controls grouping bucket count, and scopeGroupID
// restricts results to a group subtree (0 means global/no filter).
type QueryExecutor func(ctx context.Context, query string, savedName string, params map[string]string, limit int, buckets int, scopeGroupID uint) (*QueryResult, error)

// QueryResult holds the output of a QueryExecutor call.
type QueryResult struct {
	EntityType string
	Mode       string
	Items      []QueryResultItem
	Rows       []map[string]any
	Groups     []QueryResultGroup
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
				replacement = sc.Raw
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
						replacement = sc.Raw
					}
				} else {
					replacement = sc.Raw
				}
			} else {
				replacement = sc.Raw
			}
		}

		b.WriteString(replacement)
		lastEnd = sc.End
	}

	b.WriteString(input[lastEnd:])

	return b.String()
}
