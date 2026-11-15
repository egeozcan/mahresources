package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mahresources/application_context"
	"mahresources/models"
)

// The template tools give an agent what a template generation run gives its
// model: the run's reference (template_guide), each of its read Tools
// (template_lint, template_preview, ...) served by the run's own executor
// through the templateTool operations, and template_edit, the counterpart of
// the run's submit, which saves and answers with lint issues and a preview.

// templateCarriers maps a carrier type to its operations.
var templateCarriers = map[string]struct{ tool, guide, save string }{
	"category":         {"categoryTemplateTool", "categoryTemplateGuide", "createOrUpdateCategory"},
	"resourceCategory": {"resourceCategoryTemplateTool", "resourceCategoryTemplateGuide", "createOrUpdateResourceCategory"},
	"noteType":         {"noteTypeTemplateTool", "noteTypeTemplateGuide", "editNoteType"},
}

var carrierTypes = []any{"category", "resourceCategory", "noteType"}

// editingSchema is the context a template tool call is made in.
func editingSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "The carrier being edited. Lint and preview check fields of this carrier type and read its saved " +
			"value when content is omitted; previews render against entityId, else one of the carrier's members; " +
			"revisions default to this carrier.",
		"properties": map[string]any{
			"carrierType":    map[string]any{"type": "string", "enum": carrierTypes},
			"carrierId":      map[string]any{"type": "integer", "minimum": 1},
			"entityId":       map[string]any{"type": "integer", "minimum": 1, "description": "Entity a preview renders against"},
			"previewMRQL":    map[string]any{"type": "boolean", "description": "Run [mrql] queries in previews"},
			"previewPlugins": map[string]any{"type": "boolean", "description": "Run plugin shortcodes in previews, read-only"},
		},
		"required": []any{"carrierType"},
	}
}

// templateEditing is a decoded editing context.
type templateEditing struct {
	carrierType    string
	carrierID      any
	entityID       any
	previewMRQL    any
	previewPlugins any
}

func decodeEditing(v any) (templateEditing, error) {
	e := templateEditing{carrierType: "category"}
	m, ok := v.(map[string]any)
	if v != nil && !ok {
		return e, fmt.Errorf(`"editing" must be an object`)
	}
	if ct, ok := m["carrierType"].(string); ok && ct != "" {
		e.carrierType = ct
	}
	if _, ok := templateCarriers[e.carrierType]; !ok {
		return e, fmt.Errorf("editing.carrierType must be one of category, resourceCategory, noteType")
	}
	e.carrierID, e.entityID = m["carrierId"], m["entityId"]
	e.previewMRQL, e.previewPlugins = m["previewMRQL"], m["previewPlugins"]
	return e, nil
}

func (e templateEditing) requestArgs(tool string, toolArgs map[string]any) map[string]any {
	out := map[string]any{"tool": tool, "arguments": toolArgs}
	for k, v := range map[string]any{"categoryId": e.carrierID, "entityId": e.entityID,
		"previewMRQL": e.previewMRQL, "previewPlugins": e.previewPlugins} {
		if v != nil {
			out[k] = v
		}
	}
	return out
}

// buildTemplateTools returns the template tools, resolved against c.
func buildTemplateTools(c *Catalog) ([]namedTool, error) {
	ops := map[string]*Operation{}
	for _, carrier := range templateCarriers {
		for _, id := range []string{carrier.tool, carrier.guide, carrier.save} {
			op, ok := c.Get(id)
			if !ok {
				return nil, fmt.Errorf("template tools: operation %s is not exposed", id)
			}
			ops[id] = op
		}
	}

	var out []namedTool
	f := false
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &f, IdempotentHint: true, OpenWorldHint: &f}

	out = append(out, namedTool{
		tool: &mcp.Tool{
			Name:  "template_guide",
			Title: "Template authoring guide",
			Description: "Read what a template generation run is given to work from: the shortcode, plugin and MRQL " +
				"references, template partials, the carrier's identity, detail-page layout, metadata schema, sample values " +
				"and, with slot, that slot's current content. Call it before editing a carrier's templates.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"carrierType": map[string]any{"type": "string", "enum": carrierTypes},
					"carrierId":   map[string]any{"type": "integer", "minimum": 1},
					"slot":        map[string]any{"type": "string", "description": "A template field, e.g. CustomHeader, or MetaSchema"},
					"entityId":    map[string]any{"type": "integer", "minimum": 1, "description": "Sample entity to describe"},
				},
				"required": []any{"carrierType"},
			},
			Annotations: readOnly,
		},
		handle: func(_ context.Context, c *caller, _ *mcp.CallToolRequest, args map[string]any) *mcp.CallToolResult {
			ct, _ := args["carrierType"].(string)
			carrier, ok := templateCarriers[ct]
			if !ok {
				return toolError("carrierType must be one of category, resourceCategory, noteType")
			}
			req := map[string]any{}
			for from, to := range map[string]string{"carrierId": "categoryId", "slot": "slot", "entityId": "entityId"} {
				if v, ok := args[from]; ok {
					req[to] = v
				}
			}
			return c.templateGuide(ops[carrier.guide], req)
		},
	})

	for _, spec := range application_context.TemplateRunToolSpecs() {
		if spec.Name == application_context.ToolSubmit {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(spec.Parameters, &schema); err != nil {
			return nil, fmt.Errorf("template tool %s: %w", spec.Name, err)
		}
		props, _ := schema["properties"].(map[string]any)
		if props == nil {
			props = map[string]any{}
			schema["properties"] = props
		}
		props["editing"] = editingSchema()
		name := spec.Name
		out = append(out, namedTool{
			tool: &mcp.Tool{
				Name:        "template_" + name,
				Title:       "Template tool: " + name,
				Description: spec.Description + " (the same Tool a template generation run uses)",
				InputSchema: schema,
				Annotations: readOnly,
			},
			handle: func(_ context.Context, c *caller, _ *mcp.CallToolRequest, args map[string]any) *mcp.CallToolResult {
				editing, err := decodeEditing(args["editing"])
				if err != nil {
					return toolError(err.Error())
				}
				toolArgs := make(map[string]any, len(args))
				for k, v := range args {
					if k != "editing" {
						toolArgs[k] = v
					}
				}
				// The Tool's own text, not the JSON envelope around it: a model
				// reads a preview or lint result better unescaped.
				text, isErr := c.templateToolCall(ops[templateCarriers[editing.carrierType].tool], editing, name, toolArgs)
				if isErr {
					return toolError(text)
				}
				return textResult(text)
			},
		})
	}

	out = append(out, namedTool{
		tool: &mcp.Tool{
			Name:  "template_edit",
			Title: "Edit templates",
			Description: "Save template fields of a category, resource category or note type, like a generation run's " +
				"submit: fields maps field names (CustomHeader, CustomHeaderCSS, CustomSidebar, MetaSchema, ...) to their " +
				"new content; other fields keep their values. Answers with each field's lint issues and a preview of the " +
				"first HTML field, both read after the save. Every save is a template revision, which template_diff_revisions and " +
				"restore*Revision can undo. Note type edits ask the user to confirm first.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"carrierType": map[string]any{"type": "string", "enum": carrierTypes},
					"carrierId":   map[string]any{"type": "integer", "minimum": 1},
					"fields": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": []any{"string", "boolean"}},
						"description": "Field name to new content (true or false for a boolean field such as ApplyTemplatesToShares)"},
					"entityId": map[string]any{"type": "integer", "minimum": 1, "description": "Entity the preview renders against"},
				},
				"required": []any{"carrierType", "carrierId", "fields"},
			},
			Annotations: annotationsFor(Mutating, false, "POST"),
		},
		handle: func(ctx context.Context, c *caller, req *mcp.CallToolRequest, args map[string]any) *mcp.CallToolResult {
			return c.templateEdit(ctx, req, ops, args)
		},
	})
	return out, nil
}

// templateEdit saves fields through the carrier's own update operation (so a
// note type edit is confirmed like any other) and then reports lint issues
// and a preview, as a run's submit does.
func (c *caller) templateEdit(ctx context.Context, req *mcp.CallToolRequest, ops map[string]*Operation, args map[string]any) *mcp.CallToolResult {
	ct, _ := args["carrierType"].(string)
	carrier, ok := templateCarriers[ct]
	if !ok {
		return toolError("carrierType must be one of category, resourceCategory, noteType")
	}
	id := args["carrierId"]
	if isZeroArg(id) {
		return toolError("carrierId is required")
	}
	fields, ok := args["fields"].(map[string]any)
	if !ok || len(fields) == 0 {
		return toolError(`"fields" must be a non-empty object of field name to content`)
	}
	kinds := map[string]models.TemplateFieldKind{}
	var known []string
	for _, f := range models.TemplateFields(ct) {
		kinds[f.Name] = f.Kind
		known = append(known, f.Name)
	}
	names := make([]string, 0, len(fields))
	saveArgs := map[string]any{"ID": id}
	for name, v := range fields {
		kind, ok := kinds[name]
		if !ok {
			return toolError(fmt.Sprintf("%s is not a template field of a %s; fields: %s\nNothing was saved.", name, ct, strings.Join(known, ", ")))
		}
		switch t := v.(type) {
		case string:
			if kind == models.TemplateFieldBool {
				b, err := strconv.ParseBool(t)
				if err != nil {
					return toolError(fmt.Sprintf("%s is true or false\nNothing was saved.", name))
				}
				saveArgs[name] = b
			} else {
				saveArgs[name] = t
			}
		case bool:
			if kind != models.TemplateFieldBool {
				return toolError(fmt.Sprintf("fields.%s must be a string\nNothing was saved.", name))
			}
			saveArgs[name] = t
		default:
			return toolError(fmt.Sprintf("fields.%s must be a string (true or false for %s fields)\nNothing was saved.", name, "boolean"))
		}
		names = append(names, name)
	}
	sort.Strings(names)
	editing := templateEditing{carrierType: ct, carrierID: id, entityID: args["entityId"]}
	tool := ops[carrier.tool]

	// Only an operation that ran gets the lint and preview report; a
	// confirmation request, a refusal and a declined confirmation are
	// returned as they are.
	res, ran := c.runOp(ctx, req, "template_edit", ops[carrier.save], saveArgs)
	if !ran || res.IsError {
		return res
	}

	// Lint what was saved, so a MetaSchema saved in the same call is the one
	// [meta] paths are checked against.
	var lint []string
	for _, name := range names {
		if kinds[name] == models.TemplateFieldBool {
			continue
		}
		text, _ := c.templateToolCall(tool, editing, "lint", map[string]any{"field": name})
		lint = append(lint, name+": "+text)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Saved %s to %s %v.\n", strings.Join(names, ", "), ct, id)
	if len(lint) > 0 {
		fmt.Fprintf(&b, "\nLint:\n%s\n", strings.Join(lint, "\n"))
	}
	for _, name := range names {
		if kinds[name] != models.TemplateFieldText || strings.HasSuffix(name, "CSS") {
			continue
		}
		text, isErr := c.templateToolCall(tool, editing, "preview", map[string]any{"field": name})
		label := "Preview of " + name
		if isErr {
			label += " (failed)"
		}
		fmt.Fprintf(&b, "\n%s:\n%s\n", label, text)
		break
	}
	return textResult(strings.TrimRight(b.String(), "\n"))
}

// templateToolCall runs one template tool read-only and returns its result
// text and whether it is an error result.
func (c *caller) templateToolCall(op *Operation, editing templateEditing, name string, toolArgs map[string]any) (string, bool) {
	resp, err := dispatch(c.h.opts.Dispatch, c.readOnly(), op, editing.requestArgs(name, toolArgs), c.h.opts.MaxResponse)
	if err != nil {
		return err.Error(), true
	}
	if resp.Truncated {
		return "the result was too large", true
	}
	if resp.Status >= 300 {
		return httpError(resp).Content[0].(*mcp.TextContent).Text, true
	}
	var out struct {
		Result  string `json:"result"`
		IsError bool   `json:"isError"`
	}
	if json.Unmarshal(resp.Body, &out) != nil {
		return string(resp.Body), true
	}
	return out.Result, out.IsError
}

// templateGuide returns the guide as plain text, followed by the template
// tools it refers to.
func (c *caller) templateGuide(op *Operation, args map[string]any) *mcp.CallToolResult {
	resp, err := dispatch(c.h.opts.Dispatch, c.readOnly(), op, args, c.h.opts.MaxResponse)
	if err != nil {
		return toolError(err.Error())
	}
	if resp.Truncated {
		return toolError("the guide was too large to return")
	}
	if resp.Status >= 300 {
		return httpError(resp)
	}
	var out struct {
		Guide string `json:"guide"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return toolError("unreadable guide: " + err.Error())
	}
	names := make([]string, 0, len(out.Tools))
	for _, t := range out.Tools {
		names = append(names, "template_"+t.Name)
	}
	return textResult(out.Guide + "\n\nTools for this work: " + strings.Join(names, ", ") +
		", and template_edit to save. Lint and preview before saving; template_edit reports both after it saves.")
}
