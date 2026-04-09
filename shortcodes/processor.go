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
// limit caps the number of returned items, and buckets controls grouping bucket count.
type QueryExecutor func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error)

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
}

// QueryResultGroup is a bucket of QueryResultItems sharing a common key.
type QueryResultGroup struct {
	Key   map[string]any
	Items []QueryResultItem
}

// maxRecursionDepth limits how deeply [mrql] shortcodes may nest inside each
// other's output to prevent runaway recursive expansion.
const maxRecursionDepth = 2

// Process parses shortcodes in input and replaces them with rendered HTML.
// Built-in "meta" and "property" shortcodes are handled directly.
// "mrql" shortcodes use the provided executor callback (left as-is if nil).
// Plugin shortcodes (starting with "plugin:") use the provided renderer callback.
// If renderer is nil, plugin shortcodes are left as-is.
func Process(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string {
	return processWithDepth(reqCtx, input, ctx, renderer, executor, 0)
}

func processWithDepth(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	shortcodes := Parse(input)
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
		case strings.HasPrefix(sc.Name, "plugin:"):
			if renderer != nil {
				parts := strings.SplitN(sc.Name, ":", 3)
				if len(parts) == 3 {
					html, err := renderer(parts[1], sc, ctx)
					if err == nil {
						replacement = html
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
