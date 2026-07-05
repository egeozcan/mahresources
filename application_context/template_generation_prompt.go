package application_context

import "strings"

// System prompts pin the JSON envelope shape (mirrors deepSeekMRQLSystemPrompt).
const (
	templateSlotSystemPrompt = `You author Mahresources category template sections (HTML with shortcodes, or CSS). Return JSON only: one object with exactly the keys content and explanation, like {"content":"<div>[property path=\"Name\"]</div>","explanation":"Shows the name."}. content is the template markup for the one requested slot. Do not wrap the content in markdown code fences and do not add extra keys.`

	templateMetaSchemaSystemPrompt = `You author JSON Schema documents describing a Mahresources entity's metadata. Return JSON only: one object with exactly the keys content and explanation, like {"content":"{\"type\":\"object\",\"properties\":{}}","explanation":"..."}. content is the JSON Schema document itself, encoded as a JSON string. Do not wrap it in markdown code fences and do not add extra keys.`

	templateBundleSystemPrompt = `You design complete Mahresources category templates. Return JSON only: one object with exactly the keys slots and explanation. slots maps each requested slot field name to its template string, like {"slots":{"CustomHeader":"<h1>[property path=\"Name\"]</h1>","CustomCSS":".card{padding:1rem}"},"explanation":"..."}. Only include the requested slot names. Do not wrap values in markdown code fences and do not add extra keys.`
)

// buildTemplateGenerationPrompt returns the system prompt, the user message
// (guidance + grounding + request), and the token budget for one call.
func buildTemplateGenerationPrompt(in TemplateGenerationInput, userPrompt string) (systemPrompt, userMessage string, maxTokens int) {
	switch in.Target {
	case TemplateTargetBundle:
		return templateBundleSystemPrompt, buildBundleUserMessage(in, userPrompt), DefaultTemplateBundleMaxTokens
	case TemplateTargetMetaSchema:
		return templateMetaSchemaSystemPrompt, buildMetaSchemaUserMessage(in, userPrompt), DefaultTemplateGenerationMaxTokens
	default:
		return templateSlotSystemPrompt, buildSlotUserMessage(in, userPrompt), DefaultTemplateGenerationMaxTokens
	}
}

func buildSlotUserMessage(in TemplateGenerationInput, userPrompt string) string {
	lines := []string{
		"Generate the markup for one Mahresources category template section.",
		"The section is the " + in.Slot + " slot of a " + in.EntityType + " template.",
		slotRoleLine(in.Slot, in.EntityType),
		modeRuleLine(in.Mode),
		"Use only the shortcodes and attributes documented below; never invent shortcode names or attributes.",
		"Shortcode reference:",
		strings.TrimSpace(in.DocsBlock),
	}
	lines = append(lines, partialLine(in.PartialNames))
	lines = append(lines, schemaLines(in.MetaSchema)...)
	lines = append(lines, sampleLines(in.SampleMeta)...)
	lines = append(lines, currentContentLines(in.CurrentContent, "slot")...)
	lines = append(lines,
		"Return only the markup for this one slot as the content value.",
		"User request: "+userPrompt,
	)
	return strings.Join(lines, "\n")
}

func buildMetaSchemaUserMessage(in TemplateGenerationInput, userPrompt string) string {
	lines := []string{
		"Generate a JSON Schema describing the metadata (Meta) fields for a Mahresources " + in.EntityType + ".",
		"The schema must be a JSON Schema object with top-level \"type\": \"object\" and a \"properties\" map keyed by field name.",
		"Prefer simple field types (string, number, integer, boolean, array, object) and add \"title\" and \"description\" to each property.",
		"You may use \"enum\" for closed choice sets and \"format\" (date, date-time, email, uri) where appropriate.",
	}
	lines = append(lines, schemaLines(in.MetaSchema)...)
	lines = append(lines, sampleLines(in.SampleMeta)...)
	lines = append(lines,
		"Return the JSON Schema document as the content value (a JSON string).",
		"User request: "+userPrompt,
	)
	return strings.Join(lines, "\n")
}

func buildBundleUserMessage(in TemplateGenerationInput, userPrompt string) string {
	lines := []string{
		"Design a cohesive Mahresources category template for a " + in.EntityType + ".",
		"Fill these slot fields, each as a value in the slots object: " + strings.Join(in.BundleSlots, ", ") + ".",
		"Each slot has a distinct role:",
	}
	for _, slot := range in.BundleSlots {
		lines = append(lines, "- "+slot+": "+slotRoleLine(slot, in.EntityType))
	}
	lines = append(lines,
		"CustomCSS is CSS (no <style> wrapper); the other slots are HTML with shortcodes. Style them cohesively — use the same CSS class names in the HTML slots and CustomCSS.",
		"Use only the shortcodes and attributes documented below; never invent shortcode names or attributes.",
		"Shortcode reference:",
		strings.TrimSpace(in.DocsBlock),
	)
	lines = append(lines, partialLine(in.PartialNames))
	lines = append(lines, schemaLines(in.MetaSchema)...)
	lines = append(lines, sampleLines(in.SampleMeta)...)
	lines = append(lines, "User request: "+userPrompt)
	return strings.Join(lines, "\n")
}

// slotRoleLine describes where a slot renders and its constraints. Wording
// mirrors the reference panels on the create forms.
func slotRoleLine(slot, entityType string) string {
	switch slot {
	case "CustomHeader":
		return "CustomHeader renders at the top of the entity's detail page, against the entity itself."
	case "CustomSidebar":
		return "CustomSidebar renders in the sidebar of the entity's detail page, against the entity itself."
	case "CustomSummary":
		return "CustomSummary is a compact summary block for the entity, shown on the detail page and reused where a short overview fits."
	case "CustomAvatar":
		if entityType == "resource" {
			return "CustomAvatar replaces the small avatar/thumbnail area for a resource; keep it compact. A resource usually already has a preview image."
		}
		return "CustomAvatar replaces the small avatar/thumbnail area for the entity; keep it compact."
	case "CustomListHeader":
		return "CustomListHeader renders once at the top of the category's LIST page, against the category itself: [meta] and [property] have no entity here, and [mrql] runs at global scope."
	case "CustomMRQLResult":
		return "CustomMRQLResult renders once per item in MRQL result cards; Alpine directives are unavailable, so keep it self-contained HTML plus shortcodes."
	case "CustomCSS":
		return "CustomCSS is CSS that styles this category's custom slots; scope selectors so they do not affect the rest of the page. No <style> wrapper."
	default:
		return "This slot renders against the entity."
	}
}

func modeRuleLine(mode string) string {
	switch mode {
	case "css":
		return "Output CSS only (it is injected inside a <style> block by the app). Do not include a <style> wrapper. Shortcodes may be used to inject values."
	case "json":
		return "Output valid JSON only."
	default:
		return "Output HTML with Mahresources shortcodes. Do not include <script> or <style> tags unless the user explicitly asks for them."
	}
}

func partialLine(names []string) string {
	if len(names) == 0 {
		return "No template partials exist; do not use [partial]."
	}
	return "Only these template partials exist (use [partial name=\"...\"] only with these names): " + strings.Join(names, ", ") + "."
}

func schemaLines(metaSchema string) []string {
	if strings.TrimSpace(metaSchema) == "" {
		return []string{"No metadata JSON Schema is defined for this category; avoid [meta] paths unless the user names them."}
	}
	return []string{"The entity metadata follows this JSON Schema (use [meta path=\"...\"] for its fields):", strings.TrimSpace(metaSchema)}
}

func sampleLines(sampleMeta string) []string {
	if strings.TrimSpace(sampleMeta) == "" {
		return nil
	}
	return []string{"Here is one example entity's metadata (Meta JSON) for reference only — do not hard-code its values:", strings.TrimSpace(sampleMeta)}
}

func currentContentLines(current, kind string) []string {
	if strings.TrimSpace(current) == "" {
		return []string{"This " + kind + " is currently empty."}
	}
	return []string{"Current content of this " + kind + " (extend or refine it unless the user asks to replace it):", strings.TrimSpace(current)}
}
