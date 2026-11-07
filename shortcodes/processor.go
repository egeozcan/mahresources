package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
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
	EntityType         string
	EntityID           uint
	Entity             any
	Meta               json.RawMessage
	MetaSchema         string
	CustomMRQLResult   string
	CustomCSS          string // category-level CustomCSS, injected once per category as a <style> block
	CategoryID         uint   // category/type ID, used to dedupe CustomCSS emission
	ScopeGroupID       uint   // precomputed: owning group ID (or sentinel for ownerless)
	ParentGroupID      uint   // precomputed: owner's owner ID
	RootGroupID        uint   // precomputed: root of ownership chain
	ScopeGroupName     string
	ScopeCategoryName  string
	ParentGroupName    string
	ParentCategoryName string
	RootGroupName      string
	RootCategoryName   string
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

// DeferredSigner signs the (entityType, entityID, body) triple of a [lazy] or
// [details] block into an opaque token the deferred-render endpoint can later
// verify and render. It is injected per-render via WithDeferredSigner, mirroring
// how the partial resolver is threaded, so the shortcodes package stays free of
// crypto/DB imports. When absent, [lazy]/[details] render their body inline
// (the graceful fallback used by the share server, live preview, and JSON API —
// none of which can serve a deferred round-trip).
type DeferredSigner func(entityType string, entityID uint, body string) string

type deferredSignerKey struct{}

// WithDeferredSigner returns a context carrying signer, consulted by [lazy] and
// [details] expansion during Process. Only the main HTML display-page render
// path sets it; every other render surface omits it and gets the inline fallback.
func WithDeferredSigner(ctx context.Context, signer DeferredSigner) context.Context {
	return context.WithValue(ctx, deferredSignerKey{}, signer)
}

func deferredSignerFrom(ctx context.Context) DeferredSigner {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(deferredSignerKey{}).(DeferredSigner)
	return s
}

type refreshSignerKey struct{}

// WithRefreshSigner returns a context carrying the signer [autorefresh] seals its
// body with. It is separate from WithDeferredSigner because the two answer
// different questions: the deferred signer decides whether [lazy]/[details]
// defer their body, while this one only decides whether an eagerly rendered
// [autorefresh] body can be asked for again. The lightbox's JSON render installs
// this one alone, so [lazy] stays inline there while [autorefresh] still works.
func WithRefreshSigner(ctx context.Context, signer DeferredSigner) context.Context {
	return context.WithValue(ctx, refreshSignerKey{}, signer)
}

func refreshSignerFrom(ctx context.Context) DeferredSigner {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(refreshSignerKey{}).(DeferredSigner)
	return s
}

// Process parses shortcodes in input and replaces them with rendered HTML.
// Built-in "meta" and "property" shortcodes are handled directly.
// "mrql" shortcodes use the provided executor callback (left as-is if nil).
// Plugin shortcodes (starting with "plugin:") use the provided renderer callback.
// If renderer is nil, plugin shortcodes are left as-is.
// ProcessResult preserves the legacy HTML while exposing execution failures.
// Diagnostics are local to this synchronous render, including nested expansion.
type ProcessResult struct {
	HTML   string
	Errors []error
}

func ProcessWithDiagnostics(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) ProcessResult {
	result := ProcessResult{}
	record := func(err error) {
		if err != nil && !errors.Is(err, ErrPluginUnavailable) && !errors.Is(err, ErrQueryUnavailable) && len(result.Errors) < 64 {
			result.Errors = append(result.Errors, err)
		}
	}
	if renderer != nil {
		original := renderer
		renderer = func(name string, sc Shortcode, meta MetaShortcodeContext) (string, error) {
			html, err := original(name, sc, meta)
			record(err)
			return html, err
		}
	}
	if executor != nil {
		original := executor
		executor = func(c context.Context, query string, opts QueryOptions) (*QueryResult, error) {
			value, err := original(c, query, opts)
			record(err)
			return value, err
		}
	}
	html, err := ProcessChecked(reqCtx, input, ctx, renderer, executor)
	if err != nil && len(result.Errors) < 64 {
		result.Errors = append(result.Errors, err)
	}
	result.HTML = html
	return result
}

func Process(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string {
	out, _ := ProcessChecked(reqCtx, input, ctx, renderer, executor)
	return out
}

func processWithDepth(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	// Admit the source before anything copies or parses it. Every partial,
	// plugin output and re-processed branch passes through here, so repetition
	// is what the parsed-source budget measures.
	m := meterFrom(reqCtx)
	if !m.chargeParsed(int64(len(input))) {
		return ""
	}

	// Template comments go first, at every depth, so nothing inside one is
	// parsed or run, whether it sits in a slot, a partial body or an [each].
	input = stripTemplateComments(input)

	if depth >= maxRecursionDepth {
		// Emit the content as-is (it may be meaningful text), but when it still
		// contains unexpanded shortcodes, append a comment so an author reading
		// the page source sees why expansion stopped here.
		if len(parseWithBlocksM(m, input)) > 0 {
			input += shortcodeComment("shortcode depth limit reached")
		}
		if !m.chargeOut(int64(len(input))) {
			return ""
		}
		return input
	}

	shortcodes := parseWithBlocksM(m, input)
	if m.Err() != nil {
		return ""
	}
	if len(shortcodes) == 0 {
		if !m.chargeOut(int64(len(input))) {
			return ""
		}
		return input
	}

	// One decode of this entity's Meta for the whole render, shared with every
	// nested processWithDepth through the by-value copies of ctx. A context
	// built fresh per MRQL result item arrives with this nil and gets its own.
	if ctx.decodedMeta == nil && len(ctx.Meta) > 0 {
		ctx.decodedMeta = &decodedMetaCache{}
	}

	var b strings.Builder
	b.Grow(len(input) * 2)
	lastEnd := 0

	for _, sc := range shortcodes {
		// Reserve each append before making it; after a failure every nested
		// call returns "", so the loop unwinds at the next check.
		if !m.tick() || !m.chargeOut(int64(sc.Start-lastEnd)) {
			return ""
		}
		b.WriteString(input[lastEnd:sc.Start])

		var replacement string

		switch {
		case sc.Name == "conditional":
			replacement = RenderConditionalShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "datetime":
			replacement = RenderDateTimeShortcode(sc, ctx)
		case sc.Name == "pills":
			replacement = RenderPillsShortcode(sc, ctx)
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
		case sc.Name == "using":
			if executor != nil {
				replacement = RenderUsingShortcode(reqCtx, sc, ctx, renderer, executor, depth)
			} else {
				// Same as [mrql]: the target is loaded through the executor, so a
				// context without one cannot switch to it.
				replacement = shortcodeComment("using unavailable in this context")
			}
		case sc.Name == "link":
			replacement = RenderLinkShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "partial":
			replacement = RenderPartialShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "each":
			replacement = RenderEachShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "lazy":
			replacement = RenderLazyShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "details":
			replacement = RenderDetailsShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "reload":
			replacement = RenderReloadShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "autorefresh":
			replacement = RenderAutorefreshShortcode(reqCtx, sc, ctx, renderer, executor, depth)
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
						// A plugin's output is template source. Whether the
						// author wrapped a body says nothing about what the
						// plugin emits, so the inline form expands exactly as
						// the block form does; gating this on sc.IsBlock shipped
						// literal bracket text to the page instead. That makes a
						// plugin emitting its own shortcode a loop, and the depth
						// bound is the only thing that stops it. Only this branch
						// is source: the two below carry text the plugin did not
						// author, and expanding either would put it through the
						// template engine.
						if depth+1 < maxRecursionDepth {
							replacement = processWithDepth(reqCtx, replacement, ctx, renderer, executor, depth+1)
						} else {
							replacement = stripTemplateComments(replacement)
						}
					} else if errors.Is(err, ErrPluginUnavailable) {
						// Not a failure: this caller may not reach this plugin.
						// It renders exactly what a context with no plugin
						// renderer renders, so a page cannot be read for which
						// plugins exist by comparing the two.
						replacement = shortcodeComment("plugin unavailable in this context")
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

		if !m.chargeOut(int64(len(replacement))) {
			return ""
		}
		b.WriteString(replacement)
		lastEnd = sc.End
	}

	if !m.chargeOut(int64(len(input) - lastEnd)) {
		return ""
	}
	b.WriteString(input[lastEnd:])

	return b.String()
}
