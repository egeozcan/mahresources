package api_handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
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
	"CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomEntityPickerResult", "CustomCSS",
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
	slots = append(slots, extra...)
	for _, slot := range append([]string(nil), slots...) {
		if !strings.HasSuffix(slot, "CSS") {
			slots = append(slots, slot+"CSS")
		}
	}
	return slots
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

// maxPluginContextPromptBytes independently caps the enabled-plugin and note
// block reference. Block schemas and defaults are plugin-authored and can be
// large, so each plugin's context is included whole or omitted whole.
const maxPluginContextPromptBytes = 128 << 10

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
		case application_context.TemplateTargetSlot, "cluster":
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

		pluginReferences := serializeTemplateGenerationPluginReferencesForPrompt(ctx)
		input := application_context.TemplateGenerationInput{
			Target:         target,
			Mode:           req.Mode,
			Slot:           req.Slot,
			EntityType:     entityType,
			CurrentContent: req.Content,
			MetaSchema:     metaSchema,
			SampleMeta:     loadSampleMeta(ctx, entityType, req.CategoryID, req.EntityID),
			DocsBlock:      pluginReferences.DocsBlock,
			PluginContext:  pluginReferences.PluginContext,
			PartialNames:   templatePartialNames(ctx),
			Known:          pluginReferences.Known,
			ValidateMRQL:   func(q string) error { _, e := mrql.Parse(q); return e },
		}
		if target == "cluster" || (target == application_context.TemplateTargetSlot && req.Slot != "CustomCSS") {
			base := strings.TrimSuffix(req.Slot, "CSS")
			if !templateGenerateSlotAllowed(entityType, base) || !templateGenerateSlotAllowed(entityType, base+"CSS") {
				http_utils.HandleError(errors.New("slot has no CSS companion"), writer, request, http.StatusBadRequest)
				return
			}
			input.Target = application_context.TemplateTargetBundle
			input.BundleSlots = []string{base, base + "CSS"}
			// Legacy single-slot requests still draft a pair. Preserve the saved partner
			// as context, while the requested field uses the caller's current value.
			if target == application_context.TemplateTargetSlot {
				current := map[string]string{base: "", base + "CSS": ""}
				if req.CategoryID != 0 {
					carrier, err := loadPreviewCarrier(ctx, entityType, req.CategoryID)
					if err != nil {
						http_utils.HandleError(errors.New("cannot load the existing template pair"), writer, request, http.StatusBadRequest)
						return
					}
					value := reflect.Indirect(reflect.ValueOf(carrier))
					for _, field := range input.BundleSlots {
						current[field] = value.FieldByName(field).String()
					}
				}
				current[req.Slot] = req.Content
				content, _ := json.Marshal(current)
				input.CurrentContent = string(content)
			}
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
	return serializeTemplateGenerationPluginReferencesForPrompt(ctx).DocsBlock
}

// serializeTemplateGenerationPluginReferencesForPrompt reads every
// plugin-derived prompt component from one AuthoringSnapshot. A generation
// request therefore sees shortcode contracts, plugins, and blocks from one
// real registry state even while an administrator enables or disables plugins.
type templateGenerationPluginReferences struct {
	DocsBlock     string
	PluginContext string
	Known         shortcodes.KnownShortcodes
}

func serializeTemplateGenerationPluginReferencesForPrompt(ctx PluginManagerProvider) templateGenerationPluginReferences {
	var b strings.Builder
	for _, d := range shortcodes.BuiltinDocs() {
		writeBuiltinShortcodeDoc(&b, d)
	}
	references := templateGenerationPluginReferences{Known: shortcodes.KnownFromBuiltins()}
	if pm := ctx.PluginManager(); pm != nil {
		snapshot := pm.AuthoringSnapshot()
		if omitted := appendPluginShortcodeDocsForPrompt(&b, snapshot.Shortcodes); omitted > 0 {
			fmt.Fprintf(&b, "- %d enabled plugin shortcode reference(s) omitted because the plugin documentation exceeded the generation prompt safety limit.\n", omitted)
		}
		references.PluginContext = serializePluginContextFromSnapshot(snapshot)
		references.Known = buildKnownShortcodesFromPluginDocs(snapshot.Shortcodes)
	}
	references.DocsBlock = b.String()
	return references
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

// serializePluginContextForPrompt describes the plugins that are actually
// loaded now, including their registered note block types. Shortcode contracts
// remain in DocsBlock; this separate reference gives the model the surrounding
// plugin capabilities and the structured-block usage needed to design a
// complementary note template.
func serializePluginContextForPrompt(ctx PluginManagerProvider) string {
	pm := ctx.PluginManager()
	if pm == nil {
		return ""
	}
	return serializePluginContextFromSnapshot(pm.AuthoringSnapshot())
}

func serializePluginContextFromSnapshot(snapshot plugin_system.AuthoringSnapshot) string {
	plugins := snapshot.Plugins
	if len(plugins) == 0 {
		return ""
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })

	blocksByPlugin := make(map[string][]plugin_system.PluginBlockInfo)
	for _, block := range snapshot.Blocks {
		blocksByPlugin[block.PluginName] = append(blocksByPlugin[block.PluginName], block)
	}
	for _, blocks := range blocksByPlugin {
		sort.Slice(blocks, func(i, j int) bool { return blocks[i].TypeName < blocks[j].TypeName })
	}

	var b strings.Builder
	b.WriteString("Active plugins and their registered note blocks:\n")
	used, omitted := 0, 0
	for _, plugin := range plugins {
		var entry strings.Builder
		writePluginContextEntry(&entry, plugin, blocksByPlugin[plugin.Name])
		if entry.Len() > maxPluginContextPromptBytes-used {
			omitted++
			continue
		}
		b.WriteString(entry.String())
		used += entry.Len()
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "- %d enabled plugin reference(s) omitted because their complete metadata and block registration exceeded the generation prompt safety limit.\n", omitted)
	}
	return b.String()
}

func writePluginContextEntry(b *strings.Builder, plugin plugin_system.AuthoringPluginInfo, blocks []plugin_system.PluginBlockInfo) {
	b.WriteString("- Plugin: ")
	b.WriteString(plugin.Name)
	if plugin.Version != "" {
		b.WriteString(" (version ")
		b.WriteString(plugin.Version)
		b.WriteByte(')')
	}
	if description := oneLine(plugin.Description); description != "" {
		b.WriteString("\n  Description: ")
		b.WriteString(description)
	}
	if len(blocks) == 0 {
		b.WriteString("\n  Note blocks: none registered.\n")
		return
	}
	b.WriteString("\n  Note blocks:\n")
	for _, block := range blocks {
		writePluginBlockContext(b, block)
	}
}

func writePluginBlockContext(b *strings.Builder, block plugin_system.PluginBlockInfo) {
	b.WriteString("  - ")
	b.WriteString(block.TypeName)
	if block.Label != "" {
		b.WriteString(" (label: ")
		b.WriteString(oneLine(block.Label))
		b.WriteByte(')')
	}
	if description := oneLine(block.Description); description != "" {
		b.WriteString("\n    Description: ")
		b.WriteString(description)
	}
	b.WriteString("\n    Usage: Add this structured block to a Note with the note block editor; its type is ")
	b.WriteString(block.TypeName)
	b.WriteString(". It is not template shortcode markup.")
	writePluginBlockJSON(b, "Default content", block.DefaultContent)
	writePluginBlockJSON(b, "Default state", block.DefaultState)
	writePluginBlockJSON(b, "Content validation schema", block.ContentSchema)
	writePluginBlockJSON(b, "State validation schema", block.StateSchema)
	if len(block.Filters.NoteTypeIDs) > 0 || len(block.Filters.CategoryIDs) > 0 {
		b.WriteString("\n    Availability: requires")
		if len(block.Filters.NoteTypeIDs) > 0 {
			b.WriteString(" note type IDs ")
			b.WriteString(formatPluginBlockFilterIDs(block.Filters.NoteTypeIDs))
		}
		if len(block.Filters.NoteTypeIDs) > 0 && len(block.Filters.CategoryIDs) > 0 {
			b.WriteString(" and")
		}
		if len(block.Filters.CategoryIDs) > 0 {
			b.WriteString(" owning-group category IDs ")
			b.WriteString(formatPluginBlockFilterIDs(block.Filters.CategoryIDs))
		}
		b.WriteByte('.')
	}
	b.WriteByte('\n')
}

func writePluginBlockJSON(b *strings.Builder, label string, value json.RawMessage) {
	if len(value) == 0 {
		return
	}
	b.WriteString("\n    ")
	b.WriteString(label)
	b.WriteString(": ")
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err == nil {
		b.WriteString(compact.String())
		return
	}
	// Plugin block registration has already validated these values as JSON. The
	// fallback keeps malformed data visible without rewriting its string values.
	b.Write(value)
}

func formatPluginBlockFilterIDs(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ", ")
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
