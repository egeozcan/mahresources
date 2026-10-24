package application_context

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"

	"golang.org/x/net/html"
)

// templateSiblingStylesBudget bounds the sibling styles block, and
// templateWholeCustomCSSBytes is the largest CustomCSS it shows in full.
const (
	templateSiblingStylesBudget = 8 << 10
	templateWholeCustomCSSBytes = 6 << 10
	// The category's name and description are clipped to these, with a note
	// of their full length.
	templateIdentityNameBytes        = 200
	templateIdentityDescriptionBytes = 2 << 10
)

// identityLines names the category the template belongs to, fenced: both
// values are free text the user typed. Nothing when both are empty.
func identityLines(name, description string) []string {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if name == "" && description == "" {
		return nil
	}
	var body []string
	if name != "" {
		body = append(body, "Name: "+clipSampleString(name, templateIdentityNameBytes))
	}
	if description != "" {
		body = append(body, "Description: "+clipSampleString(description, templateIdentityDescriptionBytes))
	}
	return append([]string{"This template belongs to this category:"}, referenceBlock("category identity", strings.Join(body, "\n"))...)
}

// DescribeSectionConfig lists the built-in sections of the carrier's detail
// page that the Section Config shows and hides, resolved the way the page
// resolves it (invalid JSON means the defaults), and what each choice means
// for the template.
func DescribeSectionConfig(entityType, raw string, hasSchema bool) []string {
	data := types.JSON(raw)
	var resolved any
	switch entityType {
	case "group":
		resolved = models.ResolveGroupSectionConfig(&data)
	case "resource":
		resolved = models.ResolveResourceSectionConfig(&data)
	case "note":
		resolved = models.ResolveNoteSectionConfig(&data)
	default:
		return nil
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return nil
	}
	var sections map[string]any
	if err := json.Unmarshal(encoded, &sections); err != nil {
		return nil
	}

	shown := map[string]bool{}
	var shownNames, hiddenNames []string
	for _, key := range sortedAnyKeys(sections) {
		switch value := sections[key].(type) {
		case bool:
			shown[key] = value
			if value {
				shownNames = append(shownNames, key)
			} else {
				hiddenNames = append(hiddenNames, key)
			}
		case map[string]any:
			state, _ := value["state"].(string)
			if state == string(models.CollapsibleOff) {
				hiddenNames = append(hiddenNames, key)
				continue
			}
			shown[key] = true
			name := key
			if state == string(models.CollapsibleCollapsed) {
				name += " (collapsed)"
			}
			shownNames = append(shownNames, name)
			for _, sub := range sortedAnyKeys(value) {
				if on, ok := value[sub].(bool); ok && !on {
					hiddenNames = append(hiddenNames, key+"."+sub)
				}
			}
		}
	}

	body := []string{
		"shown: " + strings.Join(shownNames, ", "),
		"hidden: " + strings.Join(hiddenNames, ", "),
	}
	var consequences []string
	if entityType == "group" && !shown["ownEntities"] {
		consequences = append(consequences, "The Own Entities section is off, so CustomOwnEntities never renders: return an empty CustomOwnEntities and an empty CustomOwnEntitiesCSS.")
	}
	descriptionKey := "description"
	if entityType == "note" {
		descriptionKey = "content"
	}
	if shown[descriptionKey] {
		consequences = append(consequences, "The built-in description is shown below CustomHeader: do not repeat the description in CustomHeader.")
	}
	var duplicates []string
	for _, key := range []string{"tags", "owner", "breadcrumb", "timestamps"} {
		if shown[key] {
			duplicates = append(duplicates, key)
		}
	}
	if len(duplicates) > 0 {
		consequences = append(consequences, "These are already shown by built-in sections: "+strings.Join(duplicates, ", ")+". Do not duplicate them.")
	}
	// What each section renders, from the detail templates: the schema
	// display shows the schema's own fields, so only when it has some; the JSON panel
	// shows the raw Meta; the resource metadata grid shows built-in
	// properties, never Meta.
	if shown["metaSchemaDisplay"] && hasSchema {
		consequences = append(consequences, "The built-in schema display already shows the schema-defined Meta fields with their schema titles: no generic key/value dump of them; show only the fields the request is about. Meta fields outside the schema are not shown there.")
	}
	if shown["metaJson"] {
		consequences = append(consequences, "The sidebar already shows the raw Meta JSON.")
	}
	if entityType == "resource" && shown["metadataGrid"] {
		consequences = append(consequences, "The resource metadata grid already shows the name, original name, dimensions and, with timestamps on, the created and updated times: do not repeat them.")
	}
	if entityType == "resource" && !shown["previewImage"] {
		consequences = append(consequences, "The built-in preview image is off: CustomPreview is the only preview on the detail page.")
	}
	lines := append([]string{"Built-in sections of the " + entityType + " detail page, from the category's Section Config:"}, referenceBlock("section layout", strings.Join(body, "\n"))...)
	return append(lines, consequences...)
}

func sortedAnyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// templateClassSelector matches a class selector inside a selector list.
var templateClassSelector = regexp.MustCompile(`\.(-?[_a-zA-Z][_a-zA-Z0-9-]*)`)

// htmlClassTokens lists the class tokens an HTML slot uses, sorted. A token
// that comes from a shortcode is not the template's own, so shortcodes are
// blanked out first; then the markup is parsed as a browser would, foreign
// SVG and MathML content included, and only element class attributes count,
// never text, comments, or script and style bodies. :class and x-bind:class
// are Alpine expressions, not class lists.
func htmlClassTokens(markup string) []string {
	for _, sc := range shortcodes.Parse(markup) {
		markup = strings.Replace(markup, sc.Raw, strings.Repeat(" ", len(sc.Raw)), 1)
	}
	tokens := map[string]bool{}
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return nil
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Namespace == "" {
					for _, token := range strings.Fields(attr.Val) {
						tokens[token] = true
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return sortedKeys(tokens)
}

// cssClassSelectors lists the classes a stylesheet's selectors name, sorted.
// Comments and strings are dropped first; declaration blocks are skipped, so
// a value like .5em is not read as a class; @media and @supports blocks are
// read as nested rules, and other at-rules (keyframes, font-face) skipped.
// Unsaved CSS is often half typed, so an unclosed block runs to the end and a
// stray closing brace is ignored.
func cssClassSelectors(css string) []string {
	css = stripCSSCommentsAndStrings(css)
	const (
		blockGroup = iota // @media, @supports: holds rules
		blockSkip         // a declaration block or another at-rule
	)
	classes := map[string]bool{}
	var stack []int
	var prelude strings.Builder
	for i := 0; i < len(css); i++ {
		c := css[i]
		if len(stack) > 0 && stack[len(stack)-1] == blockSkip {
			switch c {
			case '{':
				stack = append(stack, blockSkip)
			case '}':
				stack = stack[:len(stack)-1]
			}
			continue
		}
		switch c {
		case '{':
			selector := strings.TrimSpace(prelude.String())
			prelude.Reset()
			switch {
			case strings.HasPrefix(selector, "@media"), strings.HasPrefix(selector, "@supports"):
				stack = append(stack, blockGroup)
			case strings.HasPrefix(selector, "@"):
				stack = append(stack, blockSkip)
			default:
				for _, m := range templateClassSelector.FindAllStringSubmatch(selector, -1) {
					classes[m[1]] = true
				}
				stack = append(stack, blockSkip)
			}
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			prelude.Reset()
		case ';':
			prelude.Reset()
		default:
			prelude.WriteByte(c)
		}
	}
	return sortedKeys(classes)
}

func stripCSSCommentsAndStrings(css string) string {
	var b strings.Builder
	for i := 0; i < len(css); i++ {
		switch {
		case strings.HasPrefix(css[i:], "/*"):
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 3
		case css[i] == '"' || css[i] == '\'':
			quote := css[i]
			for i++; i < len(css) && css[i] != quote; i++ {
				if css[i] == '\\' {
					i++
				}
			}
			b.WriteString(`""`)
		default:
			b.WriteByte(css[i])
		}
	}
	return b.String()
}

// DescribeSiblingStyles tells the model which classes the category's other
// slots already use, so a new slot can share their look: the class tokens of
// each HTML slot, the class selectors of each CSS slot, and CustomCSS whole
// when it is small, its selectors otherwise. Slots are listed in name order
// and whole lines are kept within budget.
func DescribeSiblingStyles(siblings map[string]string, customCSS string, budget int) []string {
	var entries []string
	names := make([]string, 0, len(siblings))
	for name := range siblings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := siblings[name]
		if strings.TrimSpace(content) == "" {
			continue
		}
		if strings.HasSuffix(name, "CSS") {
			if selectors := cssClassSelectors(content); len(selectors) > 0 {
				entries = append(entries, name+" selectors: ."+strings.Join(selectors, ", ."))
			}
		} else if classes := htmlClassTokens(content); len(classes) > 0 {
			entries = append(entries, name+" classes: "+strings.Join(classes, ", "))
		}
	}
	if css := strings.TrimSpace(customCSS); css != "" {
		if len(css) <= templateWholeCustomCSSBytes {
			entries = append(entries, "CustomCSS (whole):\n"+css)
		} else if selectors := cssClassSelectors(css); len(selectors) > 0 {
			entries = append(entries, "CustomCSS selectors: ."+strings.Join(selectors, ", ."))
		}
	}
	if len(entries) == 0 {
		return nil
	}
	lines, _ := fitSection(entries, budget, func(kept []string, omitted int) []string {
		if omitted > 0 {
			kept = append(kept[:len(kept):len(kept)], fmt.Sprintf("%d more slots are not listed.", omitted))
		}
		return append([]string{"The category's other slots already use these classes; reuse them where they fit so the template looks consistent:"}, referenceBlock("sibling styles", strings.Join(kept, "\n"))...)
	})
	return lines
}

// siblingStyleLines is the sibling block for one call: every slot except the
// ones this call writes. A whole-template call gets none, because its current
// content already holds every slot.
func siblingStyleLines(in TemplateGenerationInput) []string {
	if len(in.SiblingSlots) == 0 || templateCallKind(in) == "bundle" {
		return nil
	}
	own := map[string]bool{in.Slot: true}
	for _, slot := range in.BundleSlots {
		own[slot] = true
	}
	siblings := map[string]string{}
	customCSS := ""
	for name, content := range in.SiblingSlots {
		switch {
		case own[name]:
		case name == "CustomCSS":
			customCSS = content
		default:
			siblings[name] = content
		}
	}
	return DescribeSiblingStyles(siblings, customCSS, templateSiblingStylesBudget)
}

// schemaShowsFields reports whether the schema display has fields to show: it
// iterates the schema's properties (src/schema-editor/modes/display-mode.ts),
// after resolving $ref and composition, so a schema with neither shows
// nothing.
func schemaShowsFields(schema string) bool {
	var root map[string]any
	if err := json.Unmarshal([]byte(schema), &root); err != nil {
		return false
	}
	if properties, ok := root["properties"].(map[string]any); ok && len(properties) > 0 {
		return true
	}
	for _, key := range []string{"$ref", "allOf", "anyOf", "oneOf", "if"} {
		if _, ok := root[key]; ok {
			return true
		}
	}
	return false
}
