package application_context

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// Privacy mode in the Seed (classic and harness): which sections are readable,
// and the section that explains Placeholders to the model.

// privacyReadableSections are the section keys whose text carries no Entity
// data (template source, schemas, the user's request and fixed instructions),
// so the whole text joins the guard's readable corpus. A section that mixes in
// entity data (identity, samples, starting-points, partials) is not on this
// list: it registers only its readable parts, where it is built.
var privacyReadableSections = map[string]bool{
	"system": true, "harness": true, "rules": true, "shortcodes": true, "plugins": true, "mrql": true,
	"reference-end": true, "carrier": true, "extensions": true, "display-types": true, "schema": true,
	"layout": true, "request": true, "task": true, "siblings": true, "current": true, "included-partials": true,
	"privacy": true, "output": true, "plan-task": true, "plan": true, "css": true, "repair": true,
}

// privacySectionLines tell the model what a Placeholder is and what never to do
// with one. The Seed is the only place it learns the stand-in date.
func privacySectionLines() []string {
	return []string{
		"Privacy mode is on. Values from the library (names, descriptions, meta values, tags, owners, URLs, file names, locations and any other text people typed into their entities) are replaced by Placeholders such as ⟦ab·text 3 · 1 line · ~50 chars⟧. A Placeholder shows only the kind of value (text, url, email or date), its rough length and whether it spans lines; equal values share one Placeholder.",
		"Never copy a Placeholder into a draft, and never use one as a path, key or query value: a draft that contains one is refused. Reference a field with [property] or [meta] instead. Template source, field names, the metadata schema, category and type names and the user's request are shown as they are.",
		"A date in a rendered preview is the fixed stand-in " + TemplatePrivacyStandInDate + ", whatever the real date is. Dates in sample values appear as date Placeholders.",
	}
}

// identityLinesFor is identityLines for a request that may run under Privacy
// mode: the category name is vocabulary and stays readable, its description is
// an Entity value.
func identityLinesFor(in TemplateGenerationInput) []string {
	return identityLines(in.Privacy.Keep(in.CategoryName), in.Privacy.Text(in.CategoryDescription))
}

// insertPrivacySection puts the privacy section before the request, once.
func insertPrivacySection(in TemplateGenerationInput, p promptSections) promptSections {
	if in.Privacy == nil {
		return p
	}
	for _, s := range p {
		if s.Key == "privacy" {
			return p
		}
	}
	var one promptSections
	one.add(promptRoleUser, "privacy", "Privacy mode", true, privacySectionLines()...)
	at := len(p)
	for i, s := range p {
		if s.Key == "request" {
			at = i
			break
		}
	}
	out := append(promptSections{}, p[:at]...)
	out = append(out, one...)
	return append(out, p[at:]...)
}

// privacyMixedSections carry entity data between fixed prose. Only the prose
// outside their fences (and the field names inside) joins the corpus, so the
// words around a Placeholder never make a short value look like a leak.
var privacyMixedSections = map[string]bool{"identity": true, "samples": true, "starting-points": true, "partials": true}

// registerReadableSections adds the readable text of a built Seed to the
// guard's corpus: whole readable sections, and the frame of the mixed ones.
func registerReadableSections(in TemplateGenerationInput, p promptSections) {
	privacy := in.Privacy
	if privacy == nil {
		return
	}
	for _, s := range p {
		switch {
		case privacyReadableSections[s.Key]:
			privacy.Keep(s.Text)
		case privacyMixedSections[s.Key]:
			keepFrame(privacy, s.Text)
		}
	}
	for _, sample := range in.Samples {
		for key, value := range sample {
			privacy.Keep(key)
			if meta, ok := value.(map[string]any); ok && key == "Meta" {
				for metaKey := range meta {
					privacy.Keep(metaKey)
					privacy.Keep("meta." + metaKey)
				}
			}
		}
	}
	for _, k := range []string{"carrierType", "type", "id", "name", "Name", "Description", "Edited carrier", "Sample", "Raw samples", "Summary of", "sample(s)", "values seen", "string", "number", "boolean", "array", "object", "null"} {
		privacy.Keep(k)
	}
}

// keepFrame registers every line of a section that is outside a
// <<<BEGIN>>>...<<<END>>> fence, and the fence markers themselves.
func keepFrame(privacy *TemplatePrivacy, text string) {
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<BEGIN "):
			privacy.Keep(line)
			inFence = true
		case strings.HasPrefix(line, "<<<END "):
			privacy.Keep(line)
			inFence = false
		case !inFence:
			privacy.Keep(line)
		}
	}
}

// finishPrivacySections applies Privacy mode to a built Seed.
func finishPrivacySections(in TemplateGenerationInput, p promptSections) promptSections {
	p = insertPrivacySection(in, p)
	registerReadableSections(in, p)
	return p
}

// privacyPartials masks the descriptions of the partial catalogue (an Entity
// value) and keeps the names and the reads summary (vocabulary).
func privacyPartials(privacy *TemplatePrivacy, in []TemplatePartialSummary) []TemplatePartialSummary {
	if privacy == nil {
		return in
	}
	out := append([]TemplatePartialSummary(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	for i := range out {
		out[i].Name = privacy.Keep(out[i].Name)
		out[i].Reads = privacy.Keep(out[i].Reads)
		out[i].Description = privacy.Text(strings.TrimSpace(out[i].Description))
	}
	return out
}

// registerRunSkeleton registers the trusted request skeleton of a run: the
// model identifier, every string of the Tool catalogue, the fixed server-written
// strings of the loop, and the adapter's protocol strings.
func registerRunSkeleton(privacy *TemplatePrivacy, conv any, model string, tools []ToolSpec) {
	if privacy == nil {
		return
	}
	privacy.Keep(model)
	for _, t := range tools {
		privacy.Keep(t.Name)
		privacy.Keep(t.Description)
		privacy.Keep(string(t.Parameters))
		keepJSONLeaves(privacy, t.Parameters)
	}
	for _, s := range []string{
		templateRunFinalInstruction, templateRunNudgeText, "Your reply was " + templateRunCutOffText + ".",
		templateRunCutOffText, templateRunMalformedText, templateRunTooManyText, templateRunNotRunFinalText,
		"unknown Tool; the Tools are listed in the request", "the Tool failed", "the Tool timed out",
	} {
		privacy.Keep(s)
	}
	registerSkeletonOf(privacy, conv)
}

// registerSkeletonOf registers the protocol strings an adapter declares.
func registerSkeletonOf(privacy *TemplatePrivacy, adapter any) {
	if privacy == nil {
		return
	}
	if s, ok := adapter.(interface{ SkeletonStrings() []string }); ok {
		for _, str := range s.SkeletonStrings() {
			privacy.Keep(str)
		}
	}
}

// keepJSONLeaves registers every decoded string leaf of a JSON document, and
// the text of its numbers and booleans. Text that is not JSON is registered whole.
func keepJSONLeaves(privacy *TemplatePrivacy, raw []byte) {
	if privacy == nil || len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		privacy.Keep(string(raw))
		return
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			privacy.Keep(x)
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		default:
			privacy.keepScalar(v)
		}
	}
	walk(v)
}

// KeepModelOutput registers what the model itself wrote: its text, every call's
// ID, name and arguments (raw and decoded), and the leaves of its Replay. The
// live Replay bytes are never changed.
func (p *TemplatePrivacy) KeepModelOutput(text string, calls []ToolCall, replay []byte) {
	if p == nil {
		return
	}
	p.Keep(text)
	for _, c := range calls {
		p.Keep(c.ID)
		p.Keep(c.Name)
		p.Keep(c.Arguments)
		keepJSONLeaves(p, []byte(c.Arguments))
	}
	keepJSONLeaves(p, replay)
}
