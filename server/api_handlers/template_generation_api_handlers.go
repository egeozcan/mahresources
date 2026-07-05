package api_handlers

import (
	"encoding/json"
	"errors"
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

// templateGenerateBundleSlots are the slot fields a whole-template ("bundle")
// generation fills, in a stable order.
var templateGenerateBundleSlots = []string{
	"CustomHeader", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomListHeader", "CustomMRQLResult", "CustomCSS",
}

// templateGenerateAllowedSlots is the set a single-slot generation may target.
var templateGenerateAllowedSlots = map[string]bool{
	"CustomHeader": true, "CustomSidebar": true, "CustomSummary": true, "CustomAvatar": true,
	"CustomListHeader": true, "CustomMRQLResult": true, "CustomCSS": true,
}

// maxTemplatePartialsForPrompt caps how many partial names are injected into the
// generation prompt.
const maxTemplatePartialsForPrompt = 200

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
func GetGenerateTemplateHandler(ctx *application_context.MahresourcesContext, entityType string) func(http.ResponseWriter, *http.Request) {
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
			if !templateGenerateAllowedSlots[req.Slot] {
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
			input.BundleSlots = templateGenerateBundleSlots
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
func loadSampleMeta(ctx *application_context.MahresourcesContext, entityType string, categoryID, entityID uint) string {
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
func firstCategoryMember(ctx *application_context.MahresourcesContext, entityType string, categoryID uint) any {
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
func templatePartialNames(ctx *application_context.MahresourcesContext) []string {
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

// serializeShortcodeDocsForPrompt renders a compact, token-efficient catalogue of
// the built-in and enabled-plugin shortcodes for the generation prompt — one line
// per shortcode (syntax, block capability, description, attribute summary, one
// example). It is not the full /v1/shortcodes/docs JSON (that is too large).
func serializeShortcodeDocsForPrompt(ctx *application_context.MahresourcesContext) string {
	var b strings.Builder
	for _, d := range shortcodes.BuiltinDocs() {
		writeShortcodeDocLine(&b, d.Syntax, string(d.IsBlock), d.Description, builtinAttrSummary(d.Attrs), firstBuiltinExampleCode(d.Examples))
	}
	if pm := ctx.PluginManager(); pm != nil {
		for _, sc := range pm.AllShortcodeDocs() {
			writeShortcodeDocLine(&b, pluginShortcodeSyntax(sc), "optional", sc.Description, pluginAttrSummary(sc.Attrs), firstPluginExampleCode(sc.Examples))
		}
	}
	return b.String()
}

func writeShortcodeDocLine(b *strings.Builder, syntax, block, description, attrs, example string) {
	b.WriteString("- ")
	b.WriteString(syntax)
	b.WriteString(" (block:")
	b.WriteString(block)
	b.WriteString(") — ")
	b.WriteString(oneLine(description))
	if attrs != "" {
		b.WriteString(" | attrs: ")
		b.WriteString(attrs)
	}
	if example != "" {
		b.WriteString(" | e.g. ")
		b.WriteString(oneLine(example))
	}
	b.WriteByte('\n')
}

func builtinAttrSummary(attrs []shortcodes.DocAttr) string {
	parts := make([]string, 0, len(attrs))
	for _, a := range attrs {
		name := a.Name
		if a.Wildcard {
			name += "*(prefix)"
		} else if a.Required {
			name += "*"
		}
		if len(a.Enum) > 0 {
			name += "=" + strings.Join(a.Enum, "|")
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

func pluginAttrSummary(attrs []plugin_system.ShortcodeDocAttr) string {
	parts := make([]string, 0, len(attrs))
	for _, a := range attrs {
		name := a.Name
		if a.Required {
			name += "*"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

func firstBuiltinExampleCode(examples []shortcodes.DocExample) string {
	if len(examples) == 0 {
		return ""
	}
	return examples[0].Code
}

func firstPluginExampleCode(examples []plugin_system.ShortcodeDocExample) string {
	if len(examples) == 0 {
		return ""
	}
	return examples[0].Code
}

// oneLine collapses newlines/tabs so a multi-line description or example stays on
// one prompt line.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}
