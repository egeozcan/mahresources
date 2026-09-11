package api_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// templateGenerateSharedSlots are the slot fields every carrier has, in a stable
// order. templateGenerateCarrierSlots adds the ones whose render surface exists
// for a single carrier only: there is no group table view and no note lightbox,
// so those fields are not on the other two models and generating one would
// produce markup with nowhere to go.
var templateGenerateSharedSlots = []string{
	"CustomHeader", "CustomDetailFooter", "CustomSidebar", "CustomSummary", "CustomAvatar",
	"CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomCSS",
}

var templateGenerateCarrierSlots = map[string][]string{
	"group":    {"CustomOwnEntities"},
	"resource": {"CustomPreview", "CustomLightbox", "CustomCell"},
	"note":     nil,
}

// templateGenerateBundleSlots is the ordered slot list a whole-template
// ("bundle") generation fills for one carrier.
func templateGenerateBundleSlots(entityType string) []string {
	extra := templateGenerateCarrierSlots[entityType]
	slots := make([]string, 0, len(templateGenerateSharedSlots)+len(extra))
	slots = append(slots, templateGenerateSharedSlots...)
	return append(slots, extra...)
}

// templateGenerateSlotAllowed reports whether a single-slot generation may
// target this slot on this carrier.
func templateGenerateSlotAllowed(entityType, slot string) bool {
	for _, s := range templateGenerateBundleSlots(entityType) {
		if s == slot {
			return true
		}
	}
	return false
}

// maxTemplatePartialsForPrompt caps how many partial names are injected into the
// generation prompt.
const maxTemplatePartialsForPrompt = 200

// Plugin documentation is authored outside the application binary. Keep a
// malicious or accidentally enormous enabled-plugin catalogue from exhausting
// the provider context. Entries are included whole or omitted whole, so the
// model never receives a misleading partial attribute contract. Built-in docs
// are trusted source and sit outside this budget.
const maxPluginShortcodePromptBytes = 128 << 10

type templateGenerateRequest struct {
	Target     string `json:"target" schema:"target"`
	Mode       string `json:"mode" schema:"mode"`
	Slot       string `json:"slot" schema:"slot"`
	Content    string `json:"content" schema:"content"`
	MetaSchema string `json:"metaSchema" schema:"metaSchema"` // current, possibly unsaved, schema editor value
	Prompt     string `json:"prompt" schema:"prompt"`
	CategoryID uint   `json:"categoryId" schema:"categoryId"`
	EntityID   uint   `json:"entityId" schema:"entityId"`
}

// GetGenerateTemplateHandler handles POST /v1/{category|resourceCategory|noteType}/generateTemplate.
// entityType selects the carrier ("group", "resource", or "note"). It assembles
// grounding (MetaSchema, a sample entity's Meta, the shortcode docs, partial
// names) and asks the template generator to draft a slot / metaschema / whole
// template. Mounted under the taxonomy/editor path prefixes, so it inherits the
// same capability gate as saving the corresponding template (admin for
// category/resourceCategory, editor for noteType).
func GetGenerateTemplateHandler(ctx TemplateGenerationContext, entityType string) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req templateGenerateRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			http_utils.HandleError(errors.New("prompt is required"), writer, request, http.StatusBadRequest)
			return
		}

		target := strings.TrimSpace(req.Target)
		if target == "" {
			target = application_context.TemplateTargetSlot
		}
		switch target {
		case application_context.TemplateTargetSlot:
			if !templateGenerateSlotAllowed(entityType, req.Slot) {
				http_utils.HandleError(errors.New("unknown template slot"), writer, request, http.StatusBadRequest)
				return
			}
		case application_context.TemplateTargetMetaSchema, application_context.TemplateTargetBundle:
			// no slot required
		default:
			http_utils.HandleError(errors.New("unknown generation target"), writer, request, http.StatusBadRequest)
			return
		}

		generator := ctx.TemplateGenerator()
		if generator == nil {
			http_utils.HandleError(errors.New("template generation is not configured"), writer, request, http.StatusServiceUnavailable)
			return
		}
		key := application_context.ClientIP(request)
		if !ctx.TemplateGenerationRateLimiter().Allow(key, time.Now()) {
			http_utils.HandleError(errors.New("template generation rate limit exceeded"), writer, request, http.StatusTooManyRequests)
			return
		}

		// MetaSchema: prefer the client's (possibly unsaved) value; else the saved carrier's.
		metaSchema := req.MetaSchema
		if strings.TrimSpace(metaSchema) == "" && req.CategoryID != 0 {
			if carrier, err := loadPreviewCarrier(ctx, entityType, req.CategoryID); err == nil {
				metaSchema = carrierMetaSchema(carrier)
			}
		}

		input := application_context.TemplateGenerationInput{
			Target:         target,
			Mode:           req.Mode,
			Slot:           req.Slot,
			EntityType:     entityType,
			CurrentContent: req.Content,
			MetaSchema:     metaSchema,
			SampleMeta:     loadSampleMeta(ctx, entityType, req.CategoryID, req.EntityID),
			DocsBlock:      serializeShortcodeDocsForPrompt(ctx),
			PartialNames:   templatePartialNames(ctx),
			Known:          buildKnownShortcodes(ctx),
			ValidateMRQL:   func(q string) error { _, e := mrql.Parse(q); return e },
		}
		if target == application_context.TemplateTargetBundle {
			input.BundleSlots = templateGenerateBundleSlots(entityType)
		}

		result, err := generator.GenerateTemplate(request.Context(), input, req.Prompt)
		if err != nil {
			switch {
			case errors.Is(err, application_context.ErrTemplateGenerationNotConfigured):
				http_utils.HandleError(errors.New("template generation is not configured"), writer, request, http.StatusServiceUnavailable)
			case errors.Is(err, application_context.ErrTemplateGenerationBadRequest):
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			case errors.Is(err, application_context.ErrTemplateGenerationTimeout):
				http_utils.HandleError(errors.New("template generation timed out"), writer, request, http.StatusGatewayTimeout)
			default:
				http_utils.HandleError(errors.New("template generation provider error"), writer, request, http.StatusBadGateway)
			}
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

// carrierMetaSchema pulls the MetaSchema string off a loaded carrier.
func carrierMetaSchema(carrier any) string {
	switch c := carrier.(type) {
	case *models.Category:
		return c.MetaSchema
	case *models.ResourceCategory:
		return c.MetaSchema
	case *models.NoteType:
		return c.MetaSchema
	}
	return ""
}

// loadSampleMeta returns the Meta JSON of the chosen sample entity (the client's
// entityId, else the first member of the category), or "" when none is available
// (e.g. the create form). Failures degrade to schema-only.
func loadSampleMeta(ctx TemplateGenerationContext, entityType string, categoryID, entityID uint) string {
	var entity any
	if entityID != 0 {
		if e, _, err := loadPreviewEntity(ctx, entityType, entityID); err == nil {
			entity = e
		}
	}
	if entity == nil && categoryID != 0 {
		entity = firstCategoryMember(ctx, entityType, categoryID)
	}
	if entity == nil {
		return ""
	}
	metaCtx := template_filters.BuildMetaContextForEntity(entity, ctx)
	if metaCtx == nil || len(metaCtx.Meta) == 0 {
		return ""
	}
	return string(metaCtx.Meta)
}

// firstCategoryMember loads the first member entity of a category, fully
// preloaded via the preview loader.
func firstCategoryMember(ctx TemplateGenerationContext, entityType string, categoryID uint) any {
	var id uint
	switch entityType {
	case "group":
		if list, err := ctx.GetGroups(0, 1, &query_models.GroupQuery{CategoryId: categoryID}); err == nil && len(list) > 0 {
			id = list[0].ID
		}
	case "resource":
		if list, err := ctx.GetResources(0, 1, &query_models.ResourceSearchQuery{ResourceCategoryId: categoryID}); err == nil && len(list) > 0 {
			id = list[0].ID
		}
	case "note":
		if list, err := ctx.GetNotes(0, 1, &query_models.NoteQuery{NoteTypeId: categoryID}); err == nil && len(list) > 0 {
			id = list[0].ID
		}
	}
	if id == 0 {
		return nil
	}
	entity, _, err := loadPreviewEntity(ctx, entityType, id)
	if err != nil {
		return nil
	}
	return entity
}

// templatePartialNames lists existing partial names so the model won't invent
// [partial name=…] references that render as empty comments.
func templatePartialNames(ctx TemplateGenerationContext) []string {
	partials, err := ctx.GetTemplatePartials(&query_models.TemplatePartialQuery{}, 0, maxTemplatePartialsForPrompt)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(partials))
	for i := range partials {
		names = append(names, partials[i].Name)
	}
	return names
}

// serializeShortcodeDocsForPrompt renders the complete authoring catalogue for
// the generation prompt. In particular, built-ins retain every attribute's
// type, required/default/enum contract and description, plus every example and
// its notes. A names-only summary made the model aware that an attribute
// existed without teaching it what the attribute did, and keeping only the
// first example hid most block/slot forms.
func serializeShortcodeDocsForPrompt(ctx PluginManagerProvider) string {
	var b strings.Builder
	for _, d := range shortcodes.BuiltinDocs() {
		writeBuiltinShortcodeDoc(&b, d)
	}
	if pm := ctx.PluginManager(); pm != nil {
		if omitted := appendPluginShortcodeDocsForPrompt(&b, pm.AllShortcodeDocs()); omitted > 0 {
			fmt.Fprintf(&b, "- %d enabled plugin shortcode reference(s) omitted because the plugin documentation exceeded the generation prompt safety limit.\n", omitted)
		}
	}
	return b.String()
}

func appendPluginShortcodeDocsForPrompt(b *strings.Builder, docs []plugin_system.PluginShortcodeInfo) int {
	used := 0
	omitted := 0
	for _, sc := range docs {
		var entry strings.Builder
		writePluginShortcodeDoc(&entry, sc)
		if entry.Len() > maxPluginShortcodePromptBytes-used {
			omitted++
			continue
		}
		b.WriteString(entry.String())
		used += entry.Len()
	}
	return omitted
}

func writeBuiltinShortcodeDoc(b *strings.Builder, d shortcodes.BuiltinDoc) {
	b.WriteString("- ")
	b.WriteString(d.Syntax)
	b.WriteString(" (block form: ")
	b.WriteString(string(d.IsBlock))
	b.WriteString(")\n  Description: ")
	b.WriteString(oneLine(d.Description))
	writeBuiltinAttrs(b, d.Attrs)
	for _, example := range d.Examples {
		writePromptExample(b, example.Title, example.Code, example.Notes)
	}
	b.WriteByte('\n')
}

func writeBuiltinAttrs(b *strings.Builder, attrs []shortcodes.DocAttr) {
	b.WriteString("\n  Attributes:")
	if len(attrs) == 0 {
		b.WriteString(" none")
		return
	}
	for _, attr := range attrs {
		b.WriteString("\n  - ")
		b.WriteString(attr.Name)
		b.WriteString(" (")
		b.WriteString(attr.Type)
		if attr.Wildcard {
			b.WriteString(", wildcard prefix")
		}
		if attr.Required {
			b.WriteString(", required")
		} else {
			b.WriteString(", optional")
		}
		if attr.Default != "" {
			b.WriteString(", default=")
			b.WriteString(attr.Default)
		}
		if len(attr.Enum) > 0 {
			b.WriteString(", values=")
			b.WriteString(strings.Join(attr.Enum, "|"))
		}
		b.WriteString("): ")
		b.WriteString(oneLine(attr.Description))
	}
}

func writePluginShortcodeDoc(b *strings.Builder, sc plugin_system.PluginShortcodeInfo) {
	b.WriteString("- ")
	b.WriteString(pluginShortcodeSyntax(sc))
	b.WriteString(" (plugin shortcode; block form: optional)\n  Description: ")
	b.WriteString(oneLine(sc.Description))
	b.WriteString("\n  Attributes:")
	if len(sc.Attrs) == 0 {
		b.WriteString(" none")
	} else {
		for _, attr := range sc.Attrs {
			b.WriteString("\n  - ")
			b.WriteString(attr.Name)
			b.WriteString(" (")
			b.WriteString(attr.Type)
			if attr.Required {
				b.WriteString(", required")
			} else {
				b.WriteString(", optional")
			}
			if attr.Default != "" {
				b.WriteString(", default=")
				b.WriteString(attr.Default)
			}
			b.WriteString("): ")
			b.WriteString(oneLine(attr.Description))
		}
	}
	for _, example := range sc.Examples {
		writePromptExample(b, example.Title, example.Code, example.Notes)
	}
	for _, note := range sc.Notes {
		b.WriteString("\n  Note: ")
		b.WriteString(oneLine(note))
	}
	b.WriteByte('\n')
}

func writePromptExample(b *strings.Builder, title, code, notes string) {
	b.WriteString("\n  Example")
	if title != "" {
		b.WriteString(" (")
		b.WriteString(oneLine(title))
		b.WriteString(")")
	}
	b.WriteString(": ")
	b.WriteString(oneLine(code))
	if notes != "" {
		b.WriteString(" | Notes: ")
		b.WriteString(oneLine(notes))
	}
}

// oneLine collapses newlines/tabs so a multi-line description or example stays on
// one prompt line.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}
