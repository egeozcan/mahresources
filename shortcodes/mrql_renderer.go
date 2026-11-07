package shortcodes

import (
	"fmt"
	"html"
	"strings"
)

// charged tracks how much of a builder has been reserved against the output
// budget, so a layout reserves each row's bytes right after formatting it and
// stops before formatting the next, holding at most the budget plus one row.
type charged struct {
	m *renderMeter
	n int
}

func (c *charged) flush(b *strings.Builder) bool {
	d := b.Len() - c.n
	c.n = b.Len()
	return c.m.chargeOut(int64(d))
}

// renderFlatDefault renders flat result items using the default card layout.
func renderFlatDefault(m *renderMeter, items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	c := charged{m: m}
	b.WriteString(`<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">`)
	for _, item := range items {
		if !m.tick() {
			return ""
		}
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
		if !c.flush(&b) {
			return ""
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderFlatTable renders flat result items as an HTML table.
func renderFlatTable(m *renderMeter, items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	c := charged{m: m}
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Name</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Type</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Description</th>`)
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)

	for _, item := range items {
		if !m.tick() {
			return ""
		}
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
		if !c.flush(&b) {
			return ""
		}
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderFlatList renders flat result items as a vertical list.
func renderFlatList(m *renderMeter, items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	c := charged{m: m}
	b.WriteString(`<ul class="divide-y divide-stone-200 border border-stone-200 rounded-md bg-white">`)
	for _, item := range items {
		if !m.tick() {
			return ""
		}
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
		if !c.flush(&b) {
			return ""
		}
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renderFlatCompact renders flat result items as inline comma-separated links.
func renderFlatCompact(m *renderMeter, items []QueryResultItem) string {
	if len(items) == 0 {
		return ""
	}

	// Streamed into one builder, reserved link by link: no slice of every
	// formatted link is retained.
	var b strings.Builder
	c := charged{m: m}
	for i, item := range items {
		if !m.tick() {
			return ""
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b,
			`<a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(extractEntityName(item)),
		)
		if !c.flush(&b) {
			return ""
		}
	}
	return b.String()
}

// renderAggregatedTable renders aggregated GROUP BY rows as an HTML table.
func renderAggregatedTable(m *renderMeter, rows []map[string]any) string {
	if len(rows) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	// Collect column keys from the first row
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}

	var b strings.Builder
	c := charged{m: m}
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm font-mono border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(
			`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">%s</th>`,
			html.EscapeString(k),
		))
	}
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)
	if !c.flush(&b) {
		return ""
	}

	for _, row := range rows {
		if !m.tick() {
			return ""
		}
		b.WriteString(`<tr class="hover:bg-stone-50">`)
		for _, k := range keys {
			if !m.tick() {
				return ""
			}
			val := row[k]
			b.WriteString(fmt.Sprintf(
				`<td class="px-3 py-2 text-stone-800 whitespace-nowrap">%s</td>`,
				html.EscapeString(fmt.Sprintf("%v", val)),
			))
			if !c.flush(&b) {
				return ""
			}
		}
		b.WriteString(`</tr>`)
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// writeBucketHeader writes the header bar for a bucketed group straight into
// the caller's builder, reserving each key and value before it is formatted
// (a key value can be large) and the rest of the bar before it is written. It
// reports false when the meter stopped the render.
func writeBucketHeader(m *renderMeter, b *strings.Builder, c *charged, key map[string]any, itemCount int) bool {
	const open = `<div class="bg-stone-100 px-3 py-2 flex items-center gap-2 text-xs font-mono">`
	b.WriteString(open)
	first := true
	for k, v := range key {
		if !m.tick() {
			return false
		}
		text, isString := v.(string)
		if !isString {
			text = fmt.Sprint(v)
		}
		// Reserve the raw sizes before escaping copies them.
		if !m.chargeOut(int64(len(k) + len(text))) {
			return false
		}
		c.n += len(k) + len(text) // already paid; flush below pays the markup and escaping growth
		if !first {
			b.WriteString(" ")
		}
		first = false
		fmt.Fprintf(b,
			`<span class="text-stone-500">%s:</span> <span class="font-semibold text-stone-700">%s</span>`,
			html.EscapeString(k),
			html.EscapeString(text),
		)
		if !c.flush(b) {
			return false
		}
	}
	fmt.Fprintf(b, `<span class="ml-auto text-stone-400">%d items</span></div>`, itemCount)
	return c.flush(b)
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
