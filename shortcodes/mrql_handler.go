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

// shortcodeParamPrefix marks shortcode attributes that bind MRQL $name
// placeholders, e.g. [mrql saved="report" param-tag="x" param-since="-7d"].
const shortcodeParamPrefix = "param-"

// collectShortcodeParams extracts `param-<name>` attributes into a params map
// (keyed by <name>). Returns nil when none are present.
func collectShortcodeParams(attrs map[string]string) map[string]string {
	var params map[string]string
	for k, v := range attrs {
		if name, ok := strings.CutPrefix(k, shortcodeParamPrefix); ok && name != "" {
			if params == nil {
				params = map[string]string{}
			}
			params[name] = v
		}
	}
	return params
}

// mrqlErrorHTML renders an executor error. The block/inline distinction keeps an
// inline [mrql value=] error from injecting a block-level <div> mid-sentence:
// inline errors are a <span>, block errors keep the original results-styled div.
func mrqlErrorHTML(err error, inline bool) string {
	if inline {
		return fmt.Sprintf(
			`<span class="mrql-error text-red-700 font-mono">%s</span>`,
			html.EscapeString(err.Error()),
		)
	}
	return fmt.Sprintf(
		`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
		html.EscapeString(err.Error()),
	)
}

// extractScalarFromResult draws a single scalar value from a query result.
//
//	key "count" (or "")  → item count (flat), group count (bucketed), row count
//	                       (aggregated).
//	key "<column>"       → Rows[0][column] for aggregated results; nil for other
//	                       modes (a column has no meaning outside aggregation).
//
// It is the shared value-extraction logic behind [conditional]'s mrql source and
// inline [mrql value=], so their semantics stay identical. Returns nil for a nil
// result or an empty aggregated set.
func extractScalarFromResult(result *QueryResult, key string) any {
	if result == nil {
		return nil
	}
	if key == "" || key == "count" {
		switch result.Mode {
		case "aggregated":
			return float64(len(result.Rows))
		case "bucketed":
			return float64(len(result.Groups))
		default:
			return float64(len(result.Items))
		}
	}
	if result.Mode == "aggregated" {
		if len(result.Rows) == 0 {
			return nil
		}
		return result.Rows[0][key]
	}
	return nil
}

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
	params := collectShortcodeParams(sc.Attrs)

	// Inline scalar mode: [mrql value="…"] renders a single escaped text value
	// with no wrapper. Its block body (if any) is ignored (a lint error). Errors
	// render as an inline span so they don't break the surrounding sentence.
	value := sc.Attrs["value"]
	inline := value != ""

	// Decompose the block body into header/footer/else slots + per-item template
	// up front: a {total} placeholder in any slot must set WantTotal before the
	// query runs (otherwise the extra COUNT query never happens).
	var slots mrqlSlots
	if sc.IsBlock && !inline {
		slots = parseMRQLSlots(sc.InnerContent)
	}

	result, err := executor(reqCtx, query, QueryOptions{
		SavedName:    saved,
		Params:       params,
		Limit:        limit,
		Buckets:      buckets,
		ScopeGroupID: scopeGroupID,
		WantTotal:    slots.mentionsTotal(),
	})
	if err != nil {
		return mrqlErrorHTML(err, inline)
	}

	if result == nil {
		return ""
	}

	if inline {
		scalar := extractScalarFromResult(result, value)
		return html.EscapeString(formatScalarValue(scalar, sc.Attrs["format"], sc.Attrs["layout"]))
	}

	// Per-item template (block body minus slots). Non-empty content overrides
	// CustomMRQLResult on every item and forces custom rendering.
	itemTemplate := strings.TrimSpace(slots.Item)
	if itemTemplate != "" {
		applyBlockTemplate(result, itemTemplate)
		format = "custom"
	}

	empty := resultIsEmpty(result)

	// Empty state: header, footer, and the view-all link are chrome around
	// results and are suppressed when there are none. An [else] branch, when
	// present, is the entire empty-state output; otherwise the renderer's own
	// "No results." placeholder shows.
	if empty && slots.HasElse {
		elseHTML := renderMRQLSlot(reqCtx, slots.Else, result, ctx, renderer, executor, depth)
		return fmt.Sprintf(`<div class="mrql-results">%s</div>`, elseHTML)
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

	// Prepend each distinct category's CustomCSS (once) so inline [mrql] custom cards are styled
	// the same way the /mrql page styles them.
	cssPrefix := renderResultCSS(reqCtx, result, renderer, executor, depth)

	// Assemble: header (once) wraps the results; a default "View all →" link
	// (link-all="true") sits after the results, before any custom footer.
	var b strings.Builder
	b.WriteString(`<div class="mrql-results">`)
	b.WriteString(cssPrefix)
	if !empty && slots.HasHeader {
		b.WriteString(renderMRQLSlot(reqCtx, slots.Header, result, ctx, renderer, executor, depth))
	}
	b.WriteString(inner)
	if !empty && sc.Attrs["link-all"] == "true" {
		b.WriteString(defaultViewAllLinkHTML(result))
	}
	if !empty && slots.HasFooter {
		b.WriteString(renderMRQLSlot(reqCtx, slots.Footer, result, ctx, renderer, executor, depth))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderMRQLSlot substitutes placeholders in a header/footer slot and processes
// it with the parent entity context (the slot renders once, around the results —
// not per item).
func renderMRQLSlot(reqCtx context.Context, slot string, result *QueryResult, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	substituted := substitutePlaceholders(slot, result)
	return processWithDepth(reqCtx, substituted, ctx, renderer, executor, depth+1)
}

// renderResultCSS emits a deduped <style> block for each distinct category among the result items
// that render a custom card (CustomMRQLResult set), so the inline [mrql] shortcode styles its custom
// cards the same way the /mrql API path does (see mrqlCategoryCSS in mrql_api_handlers.go). Categories
// without a custom card are skipped — a default card carries no per-category hook to target. The CSS is
// emitted unescaped per the KAN-6 trust model, and shortcodes inside it are processed with the first
// matching item's context.
func renderResultCSS(reqCtx context.Context, result *QueryResult, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if result == nil {
		return ""
	}
	seen := map[string]bool{}
	var b strings.Builder
	emit := func(items []QueryResultItem) {
		for i := range items {
			it := &items[i]
			if it.CustomMRQLResult == "" || strings.TrimSpace(it.CustomCSS) == "" {
				continue
			}
			key := it.EntityType + ":" + strconv.FormatUint(uint64(it.CategoryID), 10)
			if seen[key] {
				continue
			}
			seen[key] = true
			childCtx := MetaShortcodeContext{
				EntityType:    it.EntityType,
				EntityID:      it.EntityID,
				Meta:          it.Meta,
				MetaSchema:    it.MetaSchema,
				Entity:        it.Entity,
				ScopeGroupID:  it.ScopeGroupID,
				ParentGroupID: it.ParentGroupID,
				RootGroupID:   it.RootGroupID,
			}
			css := processWithDepth(reqCtx, it.CustomCSS, childCtx, renderer, executor, depth+1)
			b.WriteString(`<style data-mr-custom-css="` + key + `">` + css + `</style>`)
		}
	}
	emit(result.Items)
	for i := range result.Groups {
		emit(result.Groups[i].Items)
	}
	return b.String()
}

// applyBlockTemplate stamps every entity item in the result with the block
// template, overriding any category-level CustomMRQLResult. Aggregated results
// have no items so they are unaffected.
func applyBlockTemplate(result *QueryResult, tpl string) {
	for i := range result.Items {
		result.Items[i].CustomMRQLResult = tpl
	}
	for i := range result.Groups {
		for j := range result.Groups[i].Items {
			result.Groups[i].Items[j].CustomMRQLResult = tpl
		}
	}
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
