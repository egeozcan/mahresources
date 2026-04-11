package shortcodes

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
)

const (
	defaultMRQLShortcodeLimit   = 20
	defaultMRQLShortcodeBuckets = 5
)

// RenderMRQLShortcode expands an [mrql] shortcode into rendered query results.
// The depth parameter tracks recursion level for custom templates that may
// contain nested [mrql] shortcodes.
func RenderMRQLShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	query := sc.Attrs["query"]
	saved := sc.Attrs["saved"]
	if query == "" && saved == "" {
		return ""
	}

	limit := parseIntAttr(sc.Attrs["limit"], defaultMRQLShortcodeLimit)
	buckets := parseIntAttr(sc.Attrs["buckets"], defaultMRQLShortcodeBuckets)
	format := sc.Attrs["format"] // "" means auto-resolve
	scopeGroupID := resolveScopeKeyword(sc.Attrs["scope"], ctx)

	result, err := executor(reqCtx, query, saved, limit, buckets, scopeGroupID)
	if err != nil {
		return fmt.Sprintf(
			`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
			html.EscapeString(err.Error()),
		)
	}

	if result == nil {
		return ""
	}

	var inner string

	switch result.Mode {
	case "aggregated":
		inner = renderAggregatedTable(result.Rows)
	case "bucketed":
		inner = renderBucketed(reqCtx, result.Groups, format, ctx, renderer, executor, depth)
	default: // "flat" or empty
		inner = renderFlat(reqCtx, result.Items, format, ctx, renderer, executor, depth)
	}

	return fmt.Sprintf(`<div class="mrql-results">%s</div>`, inner)
}

// renderFlat renders flat result items using the resolved format.
func renderFlat(reqCtx context.Context, items []QueryResultItem, format string, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	switch format {
	case "table":
		return renderFlatTable(items)
	case "list":
		return renderFlatList(items)
	case "compact":
		return renderFlatCompact(items)
	case "custom":
		return renderFlatWithCustom(reqCtx, items, renderer, executor, depth, true)
	default:
		// Auto-resolve: try custom templates, fall back to default
		return renderFlatWithCustom(reqCtx, items, renderer, executor, depth, false)
	}
}

// renderFlatWithCustom renders items, using custom templates where available.
// If forceCustom is true (explicit format="custom"), items without templates use default rendering.
// If forceCustom is false (auto-resolve), items without templates also use default rendering.
func renderFlatWithCustom(reqCtx context.Context, items []QueryResultItem, renderer PluginRenderer, executor QueryExecutor, depth int, forceCustom bool) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	// Check if any item has a custom template
	hasAnyCustom := false
	for _, item := range items {
		if item.CustomMRQLResult != "" {
			hasAnyCustom = true
			break
		}
	}

	// If no custom templates and not forced, use default
	if !hasAnyCustom && !forceCustom {
		return renderFlatDefault(items)
	}

	var b strings.Builder
	for _, item := range items {
		if item.CustomMRQLResult != "" {
			childCtx := MetaShortcodeContext{
				EntityType:    item.EntityType,
				EntityID:      item.EntityID,
				Meta:          item.Meta,
				MetaSchema:    item.MetaSchema,
				Entity:        item.Entity,
				ScopeGroupID:  item.ScopeGroupID,
				ParentGroupID: item.ParentGroupID,
				RootGroupID:   item.RootGroupID,
			}
			rendered := processWithDepth(reqCtx, item.CustomMRQLResult, childCtx, renderer, executor, depth+1)
			b.WriteString(rendered)
		} else {
			// Fall back to default single-item rendering
			b.WriteString(renderFlatDefault([]QueryResultItem{item}))
		}
	}
	return b.String()
}

// renderBucketed renders bucketed GROUP BY results.
func renderBucketed(reqCtx context.Context, groups []QueryResultGroup, format string, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if len(groups) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="space-y-4">`)
	for _, group := range groups {
		b.WriteString(`<div class="border border-stone-200 rounded-md overflow-hidden">`)
		b.WriteString(renderBucketHeader(group.Key, len(group.Items)))
		b.WriteString(`<div class="p-3">`)
		b.WriteString(renderFlat(reqCtx, group.Items, format, parentCtx, renderer, executor, depth))
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func parseIntAttr(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}

// resolveScopeKeyword maps a scope attribute value to a concrete group ID
// using the precomputed scope fields in MetaShortcodeContext.
func resolveScopeKeyword(scope string, ctx MetaShortcodeContext) uint {
	switch scope {
	case "global":
		return 0
	case "parent":
		return ctx.ParentGroupID
	case "root":
		return ctx.RootGroupID
	case "":
		return ctx.ScopeGroupID
	default:
		if id, err := strconv.ParseUint(scope, 10, 64); err == nil {
			return uint(id)
		}
		return ctx.ScopeGroupID
	}
}
