package application_context

import "encoding/json"

// The Tool catalogue of a harness run (plan section 2): names, descriptions and
// JSON Schema parameters only. Validation, caps and execution are lane B's.
// Schemas list optional properties and set no strict mode; the Responses
// adapter sends strict:false explicitly.

// Tool names.
const (
	ToolGetEntity        = "get_entity"
	ToolListRelated      = "list_related"
	ToolGetTemplate      = "get_template"
	ToolListCarriers     = "list_carriers"
	ToolListMembers      = "list_members"
	ToolListRevisions    = "list_revisions"
	ToolGetRevision      = "get_revision"
	ToolDiffRevisions    = "diff_revisions"
	ToolLint             = "lint"
	ToolCheckMRQL        = "check_mrql"
	ToolPreview          = "preview"
	ToolGetPartial       = "get_partial"
	ToolGetShortcodeDocs = "get_shortcode_docs"
	ToolSubmit           = "submit"
)

// Per-Tool result caps in bytes, and entry caps for list Tools.
const (
	ToolCapGetEntityBytes        = 8 << 10
	ToolCapListRelatedBytes      = 16 << 10
	ToolCapGetTemplateBytes      = 32 << 10
	ToolCapListCarriersBytes     = 16 << 10
	ToolCapListMembersBytes      = 16 << 10
	ToolCapListRevisionsBytes    = 16 << 10
	ToolCapGetRevisionBytes      = 32 << 10
	ToolCapDiffRevisionsBytes    = 32 << 10
	ToolCapLintBytes             = 8 << 10
	ToolCapCheckMRQLBytes        = 4 << 10
	ToolCapPreviewBytes          = 16 << 10
	ToolCapGetPartialBytes       = 32 << 10
	ToolCapGetShortcodeDocsBytes = 16 << 10
	ToolCapSubmitBytes           = 8 << 10

	// ToolListEntryCap bounds every list Tool's page.
	ToolListEntryCap = 50
	// TemplateRunMaxCallsPerTurn is how many calls of one Turn run.
	TemplateRunMaxCallsPerTurn = 6
)

func toolSchema(properties string, required ...string) json.RawMessage {
	req := ""
	if len(required) > 0 {
		b, _ := json.Marshal(required)
		req = `,"required":` + string(b)
	}
	return json.RawMessage(`{"type":"object","properties":{` + properties + `}` + req + `}`)
}

const (
	schemaEntityType  = `"type":{"type":"string","enum":["group","resource","note"]}`
	schemaEntityID    = `"id":{"type":"integer","minimum":1}`
	schemaCarrierType = `"carrierType":{"type":"string","enum":["category","resourceCategory","noteType"]}`
	schemaOffset      = `"offset":{"type":"integer","minimum":0}`
	schemaField       = `"field":{"type":"string","description":"A template field name, e.g. CustomHeader, CustomCSS or MetaSchema."}`
)

// templateRunToolSpecs is the fixed-order catalogue; it never changes during a run.
var templateRunToolSpecs = []ToolSpec{
	{
		Name:        ToolGetEntity,
		Description: "Read one group, resource or note as the sample projection with its full Meta, plus the id and name of its carrier (category, resource category or note type).",
		Parameters:  toolSchema(schemaEntityType+","+schemaEntityID, "type", "id"),
	},
	{
		Name:        ToolListRelated,
		Description: "List entities related to one entity: up (owners), down (owned) or across (links). Returns at most 50 entries per page.",
		Parameters: toolSchema(schemaEntityType+","+schemaEntityID+
			`,"direction":{"type":"string","enum":["up","down","across"]}`+
			`,"kind":{"type":"string","enum":["group","resource","note"],"description":"Only entries of this entity type."}`+
			","+schemaOffset, "type", "id", "direction"),
	},
	{
		Name:        ToolGetTemplate,
		Description: "Read a carrier's saved template slots, MetaSchema and SectionConfig. Name fields to read only those.",
		Parameters: toolSchema(schemaCarrierType+","+`"id":{"type":"integer","minimum":1}`+
			`,"fields":{"type":"array","items":{"type":"string"}}`, "carrierType", "id"),
	},
	{
		Name:        ToolListCarriers,
		Description: "Find carriers (categories, resource categories or note types) by name. Returns id, name and description, at most 50 per page.",
		Parameters: toolSchema(schemaCarrierType+
			`,"nameQuery":{"type":"string"}`+","+schemaOffset, "carrierType"),
	},
	{
		Name:        ToolListMembers,
		Description: "List the entities that use a carrier, newest first, as type, id and name. At most 50 per page.",
		Parameters:  toolSchema(schemaCarrierType+`,"id":{"type":"integer","minimum":1},`+schemaOffset, "carrierType", "id"),
	},
	{
		Name:        ToolListRevisions,
		Description: "List a carrier's template revisions, newest first, with more=true when another page exists. Defaults to the carrier being edited. Needs edit rights to that carrier.",
		Parameters:  toolSchema(schemaCarrierType + `,"id":{"type":"integer","minimum":1},` + schemaOffset),
	},
	{
		Name:        ToolGetRevision,
		Description: "Read one template revision by number, optionally one field of it. Defaults to the carrier being edited. Needs edit rights to that carrier.",
		Parameters: toolSchema(schemaCarrierType+`,"id":{"type":"integer","minimum":1},`+
			`"number":{"type":"integer","minimum":1},`+schemaField, "number"),
	},
	{
		Name:        ToolDiffRevisions,
		Description: "Diff two template revisions, grouped by slot. 'to' defaults to the newest. Defaults to the carrier being edited. Needs edit rights to that carrier.",
		Parameters: toolSchema(schemaCarrierType+`,"id":{"type":"integer","minimum":1},`+
			`"from":{"type":"integer","minimum":1},"to":{"type":"integer","minimum":1}`, "from"),
	},
	{
		Name:        ToolLint,
		Description: "Lint a template field (or a MetaSchema) and return its issues. Omit content to lint your latest submission, else the editor value.",
		Parameters:  toolSchema(schemaField+`,"content":{"type":"string"}`, "field"),
	},
	{
		Name:        ToolCheckMRQL,
		Description: "Check an MRQL query statically without running it, optionally with parameters, a row limit and bucket count.",
		Parameters: toolSchema(`"query":{"type":"string"},"params":{"type":"object"},`+
			`"limit":{"type":"integer","minimum":1},"buckets":{"type":"integer","minimum":1}`, "query"),
	},
	{
		Name:        ToolPreview,
		Description: "Render a template field against a real entity and return the HTML text and CSS text. Omit content or css to use your latest submission, else the editor value.",
		Parameters: toolSchema(schemaField+`,"content":{"type":"string"},"css":{"type":"string"},`+
			`"entityType":{"type":"string","enum":["group","resource","note"]},`+`"entityId":{"type":"integer","minimum":1}`, "field"),
	},
	{
		Name:        ToolGetPartial,
		Description: "Read a template partial's description and source by name.",
		Parameters:  toolSchema(`"name":{"type":"string"}`, "name"),
	},
	{
		Name:        ToolGetShortcodeDocs,
		Description: "Read the full documentation entry of a built-in or plugin shortcode by name.",
		Parameters:  toolSchema(`"name":{"type":"string"}`, "name"),
	},
	{
		Name: ToolSubmit,
		Description: "Submit a draft: a map of field name to content, with an optional explanation. Each field is linted and the result returned. " +
			"The last submission of each field is kept. Submit early and in parts. Set done=true when finished.",
		Parameters: toolSchema(`"fields":{"type":"object","additionalProperties":{"type":"string"}},`+
			`"explanation":{"type":"string"},"done":{"type":"boolean"}`, "fields"),
	},
}

// TemplateRunToolSpecs returns a copy of the catalogue, in its fixed order.
func TemplateRunToolSpecs() []ToolSpec {
	out := make([]ToolSpec, len(templateRunToolSpecs))
	copy(out, templateRunToolSpecs)
	return out
}
