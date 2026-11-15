package application_context

import "strings"

// templateGuideSkipped are the Seed sections that belong to a generation run
// and not to an agent editing templates through other tools: the run's own
// system prompt (it speaks of Turns and submit), how a run works, the user's
// request and the run's task.
var templateGuideSkipped = map[string]bool{
	"system":  true,
	"harness": true,
	"request": true,
	"task":    true,
}

// TemplateAuthoringGuide returns the reference a generation run's model is
// given, for an agent that edits the same templates by other means (the MCP
// template tools): the shortcode, plugin and MRQL references, the partials,
// the carrier's identity, layout and metadata schema, sample values, the
// starting points and the current content. It is built by the run's own Seed
// builder, so the two cannot drift; only the sections about the run itself
// are left out.
func TemplateAuthoringGuide(in TemplateGenerationInput) string {
	if in.Target == "" {
		in.Target = TemplateTargetSlot
	}
	in = normalizeSlotMode(in)
	fields, required, cssAdditions := templateRunFieldsOf(in)
	sections := buildTemplateRunSections(in, "", fields, required, cssAdditions, TemplateHarnessSettings{}.withDefaults())
	var parts []string
	for _, section := range sections {
		if section.Role != promptRoleUser || templateGuideSkipped[section.Key] {
			continue
		}
		parts = append(parts, section.Text)
	}
	return strings.Join(parts, "\n")
}
