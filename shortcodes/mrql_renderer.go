package shortcodes

import (
	"fmt"
	"html"
	"strings"
)

// renderFlatDefault renders flat result items using the default card layout.
func renderFlatDefault(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">`)
	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<a href="/%s?id=%d" class="block p-3 bg-white border border-stone-200 rounded-md hover:border-amber-400 hover:shadow-sm transition-colors"><div class="min-w-0"><p class="text-sm font-medium text-stone-900 truncate">%s</p>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		))
		if desc != "" {
			b.WriteString(fmt.Sprintf(
				`<p class="text-xs text-stone-500 mt-0.5 line-clamp-2">%s</p>`,
				html.EscapeString(desc),
			))
		}
		b.WriteString(`</div></a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderFlatTable renders flat result items as an HTML table.
func renderFlatTable(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Name</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Type</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Description</th>`)
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)

	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<tr class="hover:bg-stone-50"><td class="px-3 py-2"><a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a></td><td class="px-3 py-2 text-stone-500">%s</td><td class="px-3 py-2 text-stone-500 truncate max-w-xs">%s</td></tr>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
			html.EscapeString(item.EntityType),
			html.EscapeString(desc),
		))
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderFlatList renders flat result items as a vertical list.
func renderFlatList(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<ul class="divide-y divide-stone-200 border border-stone-200 rounded-md bg-white">`)
	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<li class="px-3 py-2 hover:bg-stone-50"><a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		))
		if desc != "" {
			b.WriteString(fmt.Sprintf(
				` <span class="text-xs text-stone-500">— %s</span>`,
				html.EscapeString(desc),
			))
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renderFlatCompact renders flat result items as inline comma-separated links.
func renderFlatCompact(items []QueryResultItem) string {
	if len(items) == 0 {
		return ""
	}

	parts := make([]string, len(items))
	for i, item := range items {
		name := extractEntityName(item)
		parts[i] = fmt.Sprintf(
			`<a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		)
	}
	return strings.Join(parts, ", ")
}

// renderAggregatedTable renders aggregated GROUP BY rows as an HTML table.
func renderAggregatedTable(rows []map[string]any) string {
	if len(rows) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	// Collect column keys from the first row
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}

	var b strings.Builder
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm font-mono border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(
			`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">%s</th>`,
			html.EscapeString(k),
		))
	}
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)

	for _, row := range rows {
		b.WriteString(`<tr class="hover:bg-stone-50">`)
		for _, k := range keys {
			val := row[k]
			b.WriteString(fmt.Sprintf(
				`<td class="px-3 py-2 text-stone-800 whitespace-nowrap">%s</td>`,
				html.EscapeString(fmt.Sprintf("%v", val)),
			))
		}
		b.WriteString(`</tr>`)
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderBucketHeader renders the header bar for a bucketed group.
func renderBucketHeader(key map[string]any, itemCount int) string {
	var parts []string
	for k, v := range key {
		parts = append(parts, fmt.Sprintf(
			`<span class="text-stone-500">%s:</span> <span class="font-semibold text-stone-700">%v</span>`,
			html.EscapeString(k),
			html.EscapeString(fmt.Sprintf("%v", v)),
		))
	}
	return fmt.Sprintf(
		`<div class="bg-stone-100 px-3 py-2 flex items-center gap-2 text-xs font-mono">%s<span class="ml-auto text-stone-400">%d items</span></div>`,
		strings.Join(parts, " "),
		itemCount,
	)
}

// extractEntityName gets the Name field from the entity via reflection, falling back to the entity type + ID.
func extractEntityName(item QueryResultItem) string {
	if item.Entity == nil {
		return fmt.Sprintf("%s #%d", item.EntityType, item.EntityID)
	}
	ctx := MetaShortcodeContext{Entity: item.Entity}
	sc := Shortcode{Attrs: map[string]string{"path": "Name", "raw": "true"}}
	name := RenderPropertyShortcode(sc, ctx)
	if name == "" {
		return fmt.Sprintf("%s #%d", item.EntityType, item.EntityID)
	}
	return name
}

// extractEntityDescription gets the Description field from the entity via reflection.
func extractEntityDescription(item QueryResultItem) string {
	if item.Entity == nil {
		return ""
	}
	ctx := MetaShortcodeContext{Entity: item.Entity}
	sc := Shortcode{Attrs: map[string]string{"path": "Description", "raw": "true"}}
	return RenderPropertyShortcode(sc, ctx)
}
