package api_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

// The template tools: a generation run's read Tools (lint, preview, entity and
// template reads, revisions, partials, shortcode documentation, MRQL checks)
// and its reference, served one call at a time to any client that edits
// templates itself, such as an agent over MCP. They run through the run's own
// grounding (groundTemplateInput) and executor, so a call here answers exactly
// what the same call answers inside a run.

// templateToolRequest is one template tool call.
type templateToolRequest struct {
	// Tool is a run Tool name other than submit.
	Tool string `json:"tool"`
	// Arguments are that Tool's arguments, as a run's model would send them.
	Arguments json.RawMessage `json:"arguments"`
	// CategoryID is the carrier being edited (category, resource category or
	// note type). Lint and preview read its saved fields when content is
	// omitted, and revisions default to it. 0 means none.
	CategoryID uint `json:"categoryId"`
	// EntityID is the entity a preview renders against when the call names
	// none; 0 uses the first of the carrier's grounded samples.
	EntityID uint `json:"entityId"`
	// PreviewMRQL and PreviewPlugins are a run's opt-ins: previews execute
	// [mrql] queries, previews run plugin shortcodes read-only.
	PreviewMRQL    bool `json:"previewMRQL"`
	PreviewPlugins bool `json:"previewPlugins"`
}

// templateToolResponse is a Tool's result: its text, and whether it is an
// error result the caller should read and correct.
type templateToolResponse struct {
	Tool    string `json:"tool"`
	Result  string `json:"result"`
	IsError bool   `json:"isError"`
}

// templateGuideRequest selects what the guide is grounded in.
type templateGuideRequest struct {
	CategoryID uint `json:"categoryId"`
	EntityID   uint `json:"entityId"`
	// Slot names one template field whose current content the guide carries;
	// empty carries none.
	Slot string `json:"slot"`
}

type templateGuideResponse struct {
	Guide string `json:"guide"`
	// Tools lists the template tools and their argument schemas.
	Tools []application_context.ToolSpec `json:"tools"`
}

// templateToolSpecs is the run's Tool list without submit: what a template
// tool call may name.
func templateToolSpecs() []application_context.ToolSpec {
	var out []application_context.ToolSpec
	for _, spec := range application_context.TemplateRunToolSpecs() {
		if spec.Name != application_context.ToolSubmit {
			out = append(out, spec)
		}
	}
	return out
}

// refuseScopedTemplateTools refuses a group-limited account, as a generation
// run does: the Tools read across the library and assume an unscoped caller.
func refuseScopedTemplateTools(writer http.ResponseWriter, request *http.Request) bool {
	principal := auth.PrincipalFromContext(request.Context())
	if principal != nil && (principal.IsScoped() || principal.RequiresScope()) {
		http_utils.HandleError(errors.New("template tools are not available to group-limited accounts"), writer, request, http.StatusForbidden)
		return true
	}
	return false
}

// GetTemplateToolHandler handles POST /v1/{category|resourceCategory|noteType}/templateTool.
func GetTemplateToolHandler(ctx TemplateGenerationContext, entityType string, access TemplateRunAccessFactory) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if refuseScopedTemplateTools(writer, request) {
			return
		}
		var req templateToolRequest
		if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
			http_utils.HandleError(fmt.Errorf("the request body must be a JSON object: %w", err), writer, request, http.StatusBadRequest)
			return
		}
		req.Tool = strings.TrimSpace(req.Tool)
		known := false
		var names []string
		for _, spec := range templateToolSpecs() {
			names = append(names, spec.Name)
			known = known || spec.Name == req.Tool
		}
		if !known {
			http_utils.HandleError(fmt.Errorf("unknown template tool %q; tools: %s", req.Tool, strings.Join(names, ", ")), writer, request, http.StatusBadRequest)
			return
		}
		if len(req.Arguments) == 0 || string(req.Arguments) == "null" {
			req.Arguments = json.RawMessage(`{}`)
		}
		// A preview with no entity of its own renders against entityId, even
		// one outside the carrier: grounding only samples members, and the
		// caller named this entity on purpose.
		if req.Tool == application_context.ToolPreview && req.EntityID != 0 {
			var args map[string]any
			if err := json.Unmarshal(req.Arguments, &args); err != nil {
				http_utils.HandleError(fmt.Errorf("arguments must be a JSON object: %w", err), writer, request, http.StatusBadRequest)
				return
			}
			if _, named := args["entityId"]; !named {
				args["entityType"], args["entityId"] = entityType, req.EntityID
				b, _ := json.Marshal(args)
				req.Arguments = b
			}
		}

		runCtx := ctx.WithParentContext(request.Context())
		gen := templateGenerateRequest{
			Target: application_context.TemplateTargetSlot, CategoryID: req.CategoryID, EntityID: req.EntityID,
			PreviewMRQL: req.PreviewMRQL, PreviewPlugins: req.PreviewPlugins, PrivacyOff: true,
		}
		g, err := groundTemplateInput(runCtx, request, entityType, application_context.TemplateTargetSlot, gen, false, false)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		input := g.input
		prepared := harnessPreparation{
			ctx: runCtx, r: request, entityType: entityType, carrier: g.carrier, carrierID: req.CategoryID,
			carrierName: g.name, sampleRefs: g.sampleRefs, request: gen, principal: auth.PrincipalFromContext(request.Context()),
			access: access, siblings: g.siblings, metaSchema: g.metaSchema, sectionConfig: g.sectionConfig,
		}
		env := prepared.env(&input)
		out, err := env.Executor.Execute(request.Context(), application_context.TemplateToolInvocation{
			Name: req.Tool, Arguments: string(req.Arguments),
		})
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(templateToolResponse{Tool: req.Tool, Result: out.Text, IsError: out.IsError})
	}
}

// GetTemplateGuideHandler handles POST /v1/{category|resourceCategory|noteType}/templateGuide:
// the reference a generation run's model is given, without the parts about the
// run itself, plus the template tools' argument schemas.
func GetTemplateGuideHandler(ctx TemplateGenerationContext, entityType string, access TemplateRunAccessFactory) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if refuseScopedTemplateTools(writer, request) {
			return
		}
		var req templateGuideRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		target := application_context.TemplateTargetSlot
		slot := strings.TrimSpace(req.Slot)
		switch {
		case slot == "MetaSchema":
			target, slot = application_context.TemplateTargetMetaSchema, ""
		case slot != "" && !templateGenerateSlotAllowed(entityType, slot):
			http_utils.HandleError(fmt.Errorf("unknown template slot %q", slot), writer, request, http.StatusBadRequest)
			return
		}
		runCtx := ctx.WithParentContext(request.Context())
		gen := templateGenerateRequest{Target: target, Slot: slot, CategoryID: req.CategoryID, EntityID: req.EntityID, PrivacyOff: true}
		g, err := groundTemplateInput(runCtx, request, entityType, target, gen, false, false)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		input := g.input
		if slot != "" {
			input.CurrentContent = g.siblings[slot]
		}
		prepared := harnessPreparation{
			ctx: runCtx, r: request, entityType: entityType, carrier: g.carrier, carrierID: req.CategoryID,
			carrierName: g.name, sampleRefs: g.sampleRefs, request: gen, principal: auth.PrincipalFromContext(request.Context()),
			access: access, siblings: g.siblings, metaSchema: g.metaSchema, sectionConfig: g.sectionConfig,
		}
		_ = prepared.env(&input) // fills the partial lookup and the starting points
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(templateGuideResponse{
			Guide: application_context.TemplateAuthoringGuide(input),
			Tools: templateToolSpecs(),
		})
	}
}
