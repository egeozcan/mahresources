package shortcodes

import (
	"context"
	"errors"
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
		slots = parseMRQLSlotsM(meterFrom(reqCtx), sc.InnerContent)
		if meterFrom(reqCtx).Err() != nil {
			return ""
		}
	}

	m := meterFrom(reqCtx)
	result, err := executor(reqCtx, query, QueryOptions{
		SavedName:    saved,
		Params:       params,
		Limit:        limit,
		Buckets:      buckets,
		ScopeGroupID: scopeGroupID,
		WantTotal:    slots.mentionsTotal(),
	})
	if err != nil {
		if errors.Is(err, ErrQueryUnavailable) {
			return unavailableComment(err)
		}
		return mrqlErrorHTML(err, inline)
	}
	if !m.alive() {
		return ""
	}

	if result == nil {
		return ""
	}

	if inline {
		scalar := extractScalarFromResult(result, value)
		return html.EscapeString(formatJSONScalarObserved(ctx, scalar, sc.Attrs["format"], sc.Attrs["layout"]))
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
		if m.Err() != nil {
			return ""
		}
		// The wrapper copies the branch: reserve the copy before making it.
		if !m.chargeOut(int64(len(elseHTML) + len(`<div class="mrql-results"></div>`))) {
			return ""
		}
		return fmt.Sprintf(`<div class="mrql-results">%s</div>`, elseHTML)
	}

	var inner string

	switch result.Mode {
	case "aggregated":
		inner = renderAggregatedTable(m, result.Rows)
	case "bucketed":
		inner = renderBucketed(reqCtx, result.Groups, format, ctx, renderer, executor, depth)
	default: // "flat" or empty
		inner = renderFlat(reqCtx, result.Items, format, ctx, renderer, executor, depth)
	}

	// Prepend each distinct category's CustomCSS (once) so inline [mrql] custom cards are styled
	// the same way the /mrql page styles them.
	cssPrefix := renderResultCSS(reqCtx, result, ctx, renderer, executor, depth)
	if m.Err() != nil {
		return ""
	}

	// Assemble: header (once) wraps the results; a default "View all →" link
	// (link-all="true") sits after the results, before any custom footer.
	var b strings.Builder
	// Each part is reserved against the output budget before it is appended;
	// a stop leaves the part out and the render ends at the processor.
	write := func(s string) bool {
		if !m.chargeOut(int64(len(s))) {
			return false
		}
		b.WriteString(s)
		return true
	}
	if !write(`<div class="mrql-results">`) || !write(cssPrefix) {
		return ""
	}
	if !empty && slots.HasHeader {
		if !write(renderMRQLSlot(reqCtx, slots.Header, result, ctx, renderer, executor, depth)) {
			return ""
		}
	}
	if !write(inner) {
		return ""
	}
	if !empty && sc.Attrs["link-all"] == "true" {
		if !write(defaultViewAllLinkHTML(result)) {
			return ""
		}
	}
	if !empty && slots.HasFooter {
		if !write(renderMRQLSlot(reqCtx, slots.Footer, result, ctx, renderer, executor, depth)) {
			return ""
		}
	}
	if !write(`</div>`) {
		return ""
	}
	return b.String()
}

// renderMRQLSlot substitutes placeholders in a header/footer slot and processes
// it with the parent entity context (the slot renders once, around the results —
// not per item).
func renderMRQLSlot(reqCtx context.Context, slot string, result *QueryResult, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	substituted, ok := substitutePlaceholdersM(meterFrom(reqCtx), slot, result)
	if !ok {
		return ""
	}
	return processWithDepth(reqCtx, substituted, ctx, renderer, executor, depth+1)
}

// renderResultCSS emits a deduped <style> block for each distinct category among the result items
// that render a custom card (CustomMRQLResult set), so the inline [mrql] shortcode styles its custom
// cards the same way the /mrql API path does (see mrqlCategoryCSS in mrql_api_handlers.go). Categories
// without a custom card are skipped — a default card carries no per-category hook to target. The CSS is
// emitted unescaped per the KAN-6 trust model, and shortcodes inside it are processed with the first
// matching item's context, which inherits UntrustedSource from parent.
func renderResultCSS(reqCtx context.Context, result *QueryResult, parent MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if result == nil {
		return ""
	}
	seen := pageCSSSeen(reqCtx)
	if seen == nil {
		seen = map[string]bool{}
	}
	m := meterFrom(reqCtx)
	var b strings.Builder
	emit := func(items []QueryResultItem) {
		for i := range items {
			if !m.tick() {
				return
			}
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
				EntityType:     it.EntityType,
				EntityID:       it.EntityID,
				Meta:           it.Meta,
				MetaSchema:     it.MetaSchema,
				Entity:         it.Entity,
				ScopeGroupID:   it.ScopeGroupID,
				ParentGroupID:  it.ParentGroupID,
				RootGroupID:    it.RootGroupID,
				ScopeGroupName: it.ScopeGroupName, ScopeCategoryName: it.ScopeCategoryName,
				ParentGroupName: it.ParentGroupName, ParentCategoryName: it.ParentCategoryName,
				RootGroupName: it.RootGroupName, RootCategoryName: it.RootCategoryName,
				UntrustedSource: parent.UntrustedSource,
			}
			css := processWithDepth(reqCtx, it.CustomCSS, childCtx, renderer, executor, depth+1)
			wrapped := `<style data-mr-custom-css="` + key + `">` + css + `</style>`
			if !m.chargeOut(int64(len(wrapped))) {
				return
			}
			b.WriteString(wrapped)
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
		return renderFlatTable(meterFrom(reqCtx), items)
	case "list":
		return renderFlatList(meterFrom(reqCtx), items)
	case "compact":
		return renderFlatCompact(meterFrom(reqCtx), items)
	case "custom":
		return renderFlatWithCustom(reqCtx, items, parentCtx, renderer, executor, depth, true)
	default:
		// Auto-resolve: try custom templates, fall back to default
		return renderFlatWithCustom(reqCtx, items, parentCtx, renderer, executor, depth, false)
	}
}

// renderFlatWithCustom renders items, using custom templates where available.
// If forceCustom is true (explicit format="custom"), items without templates use default rendering.
// If forceCustom is false (auto-resolve), items without templates also use default rendering.
func renderFlatWithCustom(reqCtx context.Context, items []QueryResultItem, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int, forceCustom bool) string {
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
	m := meterFrom(reqCtx)
	if !hasAnyCustom && !forceCustom {
		return renderFlatDefault(m, items)
	}

	var b strings.Builder
	for _, item := range items {
		if !m.tick() {
			return ""
		}
		var piece string
		if item.CustomMRQLResult != "" {
			piece = processWithDepth(reqCtx, item.CustomMRQLResult, contextFromItem(item, parentCtx), renderer, executor, depth+1)
		} else {
			// Fall back to default single-item rendering
			piece = renderFlatDefault(m, []QueryResultItem{item})
		}
		if !m.chargeOut(int64(len(piece))) {
			return ""
		}
		b.WriteString(piece)
	}
	return b.String()
}

// contextFromItem builds the shortcode context a query result renders in: the
// result's own entity, Meta and scope chain. It inherits ForceReadOnly from the
// context that ran the query, because a read-only render must not grow an edit
// affordance by rendering some other entity inside itself, and UntrustedSource,
// because the item template may be the user content's own [mrql] body.
func contextFromItem(item QueryResultItem, parent MetaShortcodeContext) MetaShortcodeContext {
	return MetaShortcodeContext{
		EntityType:     item.EntityType,
		EntityID:       item.EntityID,
		Meta:           item.Meta,
		MetaSchema:     item.MetaSchema,
		Entity:         item.Entity,
		ScopeGroupID:   item.ScopeGroupID,
		ParentGroupID:  item.ParentGroupID,
		RootGroupID:    item.RootGroupID,
		ScopeGroupName: item.ScopeGroupName, ScopeCategoryName: item.ScopeCategoryName,
		ParentGroupName: item.ParentGroupName, ParentCategoryName: item.ParentCategoryName,
		RootGroupName: item.RootGroupName, RootCategoryName: item.RootCategoryName,
		ForceReadOnly:   parent.ForceReadOnly,
		UntrustedSource: parent.UntrustedSource,
	}
}

// renderBucketed renders bucketed GROUP BY results.
func renderBucketed(reqCtx context.Context, groups []QueryResultGroup, format string, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if len(groups) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	m := meterFrom(reqCtx)
	var b strings.Builder
	c := charged{m: m}
	b.WriteString(`<div class="space-y-4">`)
	for _, group := range groups {
		if !m.tick() {
			return ""
		}
		b.WriteString(`<div class="border border-stone-200 rounded-md overflow-hidden">`)
		// The header and every wrapper are reserved as they are written.
		if !writeBucketHeader(m, &b, &c, group.Key, len(group.Items)) {
			return ""
		}
		b.WriteString(`<div class="p-3">`)
		inner := renderFlat(reqCtx, group.Items, format, parentCtx, renderer, executor, depth)
		if !c.flush(&b) || !m.chargeOut(int64(len(inner))) {
			return ""
		}
		b.WriteString(inner)
		c.n = b.Len() // inner is already paid
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div>`)
	if !c.flush(&b) {
		return ""
	}
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
