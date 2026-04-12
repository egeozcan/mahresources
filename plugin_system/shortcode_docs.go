package plugin_system

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"

	"mahresources/shortcodes"
)

// docItem is the unified representation used by the docs renderer.
// Both PluginShortcode and PluginDoc convert to this before rendering.
type docItem struct {
	Name        string // URL slug
	Label       string
	Description string
	Category    string // "Shortcode", "Action", etc. Empty = no badge.
	PluginName  string // owning plugin, used for shortcode syntax
	Attrs       []ShortcodeDocAttr
	Examples    []ShortcodeDocExample
	Notes       []string
}

func shortcodeToDocItem(sc *PluginShortcode) docItem {
	return docItem{
		Name:        shortcodeName(sc),
		Label:       sc.Label,
		Description: sc.Description,
		Category:    "Shortcode",
		PluginName:  sc.PluginName,
		Attrs:       sc.Attrs,
		Examples:    sc.Examples,
		Notes:       sc.Notes,
	}
}

func pluginDocToDocItem(d *PluginDoc) docItem {
	return docItem{
		Name:        d.Name,
		Label:       d.Label,
		Description: d.Description,
		Category:    d.Category,
		PluginName:  d.PluginName,
		Attrs:       d.Attrs,
		Examples:    d.Examples,
		Notes:       d.Notes,
	}
}

// collectDocItems merges documented shortcodes and general docs into a single list.
// Caller must hold pm.mu.RLock.
func (pm *PluginManager) collectDocItems(pluginName string) []docItem {
	var items []docItem
	for _, sc := range pm.shortcodes[pluginName] {
		if sc.Description != "" {
			items = append(items, shortcodeToDocItem(sc))
		}
	}
	for _, d := range pm.docs[pluginName] {
		if d.Description != "" {
			items = append(items, pluginDocToDocItem(d))
		}
	}
	return items
}

// shortcodeName extracts the short name from a TypeName like "plugin:foo:badge".
func shortcodeName(sc *PluginShortcode) string {
	parts := strings.SplitN(sc.TypeName, ":", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return sc.TypeName
}

// PluginHasDocs returns true if the named plugin has any documented items.
func (pm *PluginManager) PluginHasDocs(pluginName string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.collectDocItems(pluginName)) > 0
}

// HasDocsPage returns true if the given path is a valid auto-generated docs page.
func (pm *PluginManager) HasDocsPage(pluginName, path string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	items := pm.collectDocItems(pluginName)
	if len(items) == 0 {
		return false
	}

	if path == "docs" {
		return true
	}

	if strings.HasPrefix(path, "docs/") {
		name := strings.TrimPrefix(path, "docs/")
		for _, item := range items {
			if item.Name == name {
				return true
			}
		}
	}

	return false
}

// HandleDocsPage generates the HTML for a docs index or detail page.
func (pm *PluginManager) HandleDocsPage(pluginName, path string) (string, error) {
	pm.mu.RLock()
	items := pm.collectDocItems(pluginName)
	pm.mu.RUnlock()

	if len(items) == 0 {
		return "", fmt.Errorf("no documentation for plugin %q", pluginName)
	}

	if path == "docs" {
		return renderDocsIndex(pluginName, items), nil
	}

	if strings.HasPrefix(path, "docs/") {
		name := strings.TrimPrefix(path, "docs/")
		for i, item := range items {
			if item.Name == name {
				var prev, next *docItem
				if i > 0 {
					prev = &items[i-1]
				}
				if i < len(items)-1 {
					next = &items[i+1]
				}
				return renderDocsDetail(pm, pluginName, item, prev, next), nil
			}
		}
	}

	return "", fmt.Errorf("docs page %q not found for plugin %q", path, pluginName)
}

// ---------------------------------------------------------------------------
// HTML generation
// ---------------------------------------------------------------------------

// renderExamplePreview attempts to render a shortcode example with its example data.
// Returns the rendered HTML or empty string if rendering fails or the code doesn't match.
func renderExamplePreview(pm *PluginManager, pluginName, fullTypeName string, ex ShortcodeDocExample) string {
	// Parse the shortcode from the example code to extract attrs (use ParseWithBlocks to support block shortcodes)
	parsed := shortcodes.ParseWithBlocks(ex.Code)
	if len(parsed) != 1 || parsed[0].Name != fullTypeName {
		return ""
	}

	metaJSON, err := json.Marshal(ex.ExampleData)
	if err != nil {
		log.Printf("[plugin] docs preview: failed to marshal example data for %s: %v", fullTypeName, err)
		return ""
	}

	result, err := pm.renderShortcodeForDocs(pluginName, fullTypeName, metaJSON, parsed[0].Attrs, parsed[0].InnerContent, parsed[0].IsBlock)
	if err != nil {
		log.Printf("[plugin] docs preview: render failed for %s: %v", fullTypeName, err)
		return ""
	}

	return result
}

func renderDocsIndex(pluginName string, items []docItem) string {
	var b strings.Builder

	b.WriteString(`<div class="max-w-4xl mx-auto px-4 py-8">`)

	// Header
	b.WriteString(`<h1 class="text-2xl font-bold text-stone-800 mb-2">`)
	b.WriteString(html.EscapeString(pluginName))
	b.WriteString(` Documentation</h1>`)
	b.WriteString(`<p class="text-stone-500 mb-6">`)
	fmt.Fprintf(&b, "%d items", len(items))
	b.WriteString(`</p>`)

	// Quick reference for shortcodes only
	var shortcodeItems []docItem
	for _, item := range items {
		if item.Category == "Shortcode" {
			shortcodeItems = append(shortcodeItems, item)
		}
	}
	if len(shortcodeItems) > 0 {
		b.WriteString(`<div class="bg-stone-100 rounded-lg p-4 mb-8">`)
		b.WriteString(`<h2 class="text-sm font-semibold text-stone-600 uppercase tracking-wider mb-3">Shortcode Reference</h2>`)
		b.WriteString(`<div class="space-y-1 font-mono text-xs text-stone-600">`)
		for _, sc := range shortcodeItems {
			b.WriteString(`<div>`)
			fmt.Fprintf(&b, `<span class="text-amber-800">[plugin:%s:%s]</span>`, html.EscapeString(pluginName), html.EscapeString(sc.Name))
			b.WriteString(` — `)
			desc := sc.Description
			if idx := strings.Index(desc, "."); idx > 0 && idx < len(desc)-1 {
				desc = desc[:idx+1]
			}
			b.WriteString(html.EscapeString(desc))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Item cards
	b.WriteString(`<div class="grid gap-3">`)
	for _, item := range items {
		b.WriteString(`<a href="/plugins/`)
		b.WriteString(html.EscapeString(pluginName))
		b.WriteString(`/docs/`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`" class="block p-4 border border-stone-200 rounded-lg hover:border-amber-300 hover:bg-amber-50/30 transition-colors">`)

		b.WriteString(`<div class="flex items-center justify-between">`)
		b.WriteString(`<div class="flex items-center gap-2">`)
		b.WriteString(`<h3 class="font-semibold text-stone-800">`)
		b.WriteString(html.EscapeString(item.Label))
		b.WriteString(`</h3>`)
		if item.Category != "" {
			b.WriteString(`<span class="text-[10px] font-medium px-1.5 py-0.5 rounded bg-stone-100 text-stone-500 uppercase tracking-wider">`)
			b.WriteString(html.EscapeString(item.Category))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`<span class="text-xs text-stone-400 font-mono">`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`</span>`)
		b.WriteString(`</div>`)

		b.WriteString(`<p class="text-sm text-stone-500 mt-1">`)
		b.WriteString(html.EscapeString(item.Description))
		b.WriteString(`</p>`)

		b.WriteString(`<div class="flex gap-3 mt-2 text-xs text-stone-400">`)
		if len(item.Attrs) > 0 {
			fmt.Fprintf(&b, `<span>%d attributes</span>`, len(item.Attrs))
		}
		if len(item.Examples) > 0 {
			fmt.Fprintf(&b, `<span>%d examples</span>`, len(item.Examples))
		}
		b.WriteString(`</div>`)

		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	return b.String()
}

func renderDocsDetail(pm *PluginManager, pluginName string, item docItem, prev, next *docItem) string {
	var b strings.Builder

	b.WriteString(`<div class="max-w-4xl mx-auto px-4 py-8">`)

	// Breadcrumb
	b.WriteString(`<nav class="text-sm mb-4" aria-label="Breadcrumb">`)
	fmt.Fprintf(&b, `<a href="/plugins/%s/docs" class="text-amber-700 hover:underline">%s Docs</a>`,
		html.EscapeString(pluginName), html.EscapeString(pluginName))
	b.WriteString(`<span class="text-stone-400 mx-1">/</span>`)
	b.WriteString(`<span class="text-stone-600">`)
	b.WriteString(html.EscapeString(item.Label))
	b.WriteString(`</span></nav>`)

	// Header
	b.WriteString(`<h1 class="text-2xl font-bold text-stone-800 mb-1">`)
	b.WriteString(html.EscapeString(item.Label))
	b.WriteString(`</h1>`)
	if item.Category != "" {
		b.WriteString(`<span class="text-[10px] font-medium px-1.5 py-0.5 rounded bg-stone-100 text-stone-500 uppercase tracking-wider">`)
		b.WriteString(html.EscapeString(item.Category))
		b.WriteString(`</span> `)
	}
	b.WriteString(`<p class="text-stone-500 mb-3 mt-2">`)
	b.WriteString(html.EscapeString(item.Description))
	b.WriteString(`</p>`)

	// Syntax snippet — only for shortcodes
	if item.Category == "Shortcode" {
		b.WriteString(`<code class="text-xs bg-stone-100 px-2 py-1 rounded font-mono text-stone-600">`)
		fmt.Fprintf(&b, `[plugin:%s:%s`, html.EscapeString(pluginName), html.EscapeString(item.Name))
		for _, attr := range item.Attrs {
			if attr.Required {
				fmt.Fprintf(&b, ` %s="…"`, html.EscapeString(attr.Name))
			}
		}
		b.WriteString(`]</code>`)
	}

	// Attributes table
	if len(item.Attrs) > 0 {
		attrLabel := "Attributes"
		if item.Category != "Shortcode" && item.Category != "" {
			attrLabel = "Parameters"
		}
		b.WriteString(`<div class="mt-8"><h2 class="text-lg font-semibold text-stone-800 mb-3">`)
		b.WriteString(attrLabel)
		b.WriteString(`</h2>`)
		b.WriteString(`<div class="overflow-x-auto"><table class="w-full text-sm border-collapse">`)
		b.WriteString(`<thead><tr class="border-b-2 border-stone-200">`)
		b.WriteString(`<th class="text-left py-2 px-3 text-stone-600 font-semibold">Name</th>`)
		b.WriteString(`<th class="text-left py-2 px-3 text-stone-600 font-semibold">Type</th>`)
		b.WriteString(`<th class="text-left py-2 px-3 text-stone-600 font-semibold">Default</th>`)
		b.WriteString(`<th class="text-left py-2 px-3 text-stone-600 font-semibold">Description</th>`)
		b.WriteString(`</tr></thead><tbody>`)

		for _, attr := range item.Attrs {
			b.WriteString(`<tr class="border-b border-stone-100">`)

			b.WriteString(`<td class="py-2 px-3 font-mono text-xs whitespace-nowrap">`)
			if attr.Required {
				b.WriteString(`<span class="text-amber-800 font-semibold">`)
			} else {
				b.WriteString(`<span class="text-stone-700">`)
			}
			b.WriteString(html.EscapeString(attr.Name))
			b.WriteString(`</span>`)
			if attr.Required {
				b.WriteString(`<span class="text-amber-600 ml-1" title="Required">*</span>`)
			}
			b.WriteString(`</td>`)

			b.WriteString(`<td class="py-2 px-3 text-stone-500 text-xs">`)
			b.WriteString(html.EscapeString(attr.Type))
			b.WriteString(`</td>`)

			b.WriteString(`<td class="py-2 px-3 font-mono text-xs text-stone-400">`)
			if attr.Default != "" {
				b.WriteString(html.EscapeString(attr.Default))
			} else {
				b.WriteString(`—`)
			}
			b.WriteString(`</td>`)

			b.WriteString(`<td class="py-2 px-3 text-stone-600">`)
			b.WriteString(html.EscapeString(attr.Description))
			b.WriteString(`</td>`)

			b.WriteString(`</tr>`)
		}

		b.WriteString(`</tbody></table></div></div>`)
	}

	// Examples
	if len(item.Examples) > 0 {
		fullTypeName := "plugin:" + pluginName + ":" + item.Name

		b.WriteString(`<div class="mt-8"><h2 class="text-lg font-semibold text-stone-800 mb-3">Examples</h2>`)
		b.WriteString(`<div class="space-y-4">`)

		for _, ex := range item.Examples {
			b.WriteString(`<div class="border border-stone-200 rounded-lg overflow-hidden">`)

			if ex.Title != "" {
				b.WriteString(`<div class="bg-stone-50 px-4 py-2 border-b border-stone-200">`)
				b.WriteString(`<h3 class="text-sm font-medium text-stone-700">`)
				b.WriteString(html.EscapeString(ex.Title))
				b.WriteString(`</h3></div>`)
			}

			// Render live preview if example_data is provided and this is a shortcode
			if item.Category == "Shortcode" && ex.ExampleData != nil && pm != nil {
				if preview := renderExamplePreview(pm, pluginName, fullTypeName, ex); preview != "" {
					b.WriteString(`<div class="px-4 py-3 border-b border-stone-200 bg-stone-50/50">`)
					b.WriteString(`<div class="text-[10px] font-medium text-stone-400 uppercase tracking-wider mb-2">Preview</div>`)
					b.WriteString(`<div>`)
					b.WriteString(preview)
					b.WriteString(`</div></div>`)
				}
			}

			b.WriteString(`<pre class="p-4 text-xs font-mono text-stone-700 bg-white overflow-x-auto whitespace-pre-wrap">`)
			b.WriteString(html.EscapeString(ex.Code))
			b.WriteString(`</pre>`)

			if ex.Notes != "" {
				b.WriteString(`<div class="px-4 py-2 bg-amber-50 border-t border-amber-100 text-xs text-amber-800">`)
				b.WriteString(html.EscapeString(ex.Notes))
				b.WriteString(`</div>`)
			}

			b.WriteString(`</div>`)
		}

		b.WriteString(`</div></div>`)
	}

	// Notes
	if len(item.Notes) > 0 {
		b.WriteString(`<div class="mt-8"><h2 class="text-lg font-semibold text-stone-800 mb-3">Notes</h2>`)
		b.WriteString(`<ul class="list-disc list-inside space-y-1 text-sm text-stone-600">`)
		for _, note := range item.Notes {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(note))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul></div>`)
	}

	// Prev / Next navigation
	if prev != nil || next != nil {
		b.WriteString(`<div class="flex justify-between mt-12 pt-4 border-t border-stone-200 text-sm">`)
		if prev != nil {
			fmt.Fprintf(&b, `<a href="/plugins/%s/docs/%s" class="text-amber-700 hover:underline">&larr; %s</a>`,
				html.EscapeString(pluginName), html.EscapeString(prev.Name), html.EscapeString(prev.Label))
		} else {
			b.WriteString(`<span></span>`)
		}
		if next != nil {
			fmt.Fprintf(&b, `<a href="/plugins/%s/docs/%s" class="text-amber-700 hover:underline">%s &rarr;</a>`,
				html.EscapeString(pluginName), html.EscapeString(next.Name), html.EscapeString(next.Label))
		} else {
			b.WriteString(`<span></span>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}
